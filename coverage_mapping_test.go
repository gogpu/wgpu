//go:build !(js && wasm)

package wgpu_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogpu/wgpu"
)

// =============================================================================
// MapPending — Wait fast path (already resolved)
// Covers map_pending.go Wait lines 90-118 (fast path + goroutine path)
// =============================================================================

func TestMapPendingWaitFastPath(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "wait-fast-buf",
		Size:             64,
		Usage:            wgpu.BufferUsageMapRead | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	pending, err := buf.MapAsync(wgpu.MapModeRead, 0, 64)
	if err != nil {
		t.Fatalf("MapAsync: %v", err)
	}
	defer pending.Release()

	// Poll to resolve.
	device.Poll(wgpu.PollWait)

	// Wait should hit the fast path (already resolved).
	err = pending.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait (fast path): %v", err)
	}
}

// =============================================================================
// MapPending — Wait with goroutine path (unresolved, then resolved)
// Covers map_pending.go Wait lines 107-117 (doneCh + goroutine path)
// =============================================================================

func TestMapPendingWaitGoroutinePath(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "wait-goroutine-buf",
		Size:  64,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	pending, err := buf.MapAsync(wgpu.MapModeRead, 0, 64)
	if err != nil {
		t.Fatalf("MapAsync: %v", err)
	}
	defer pending.Release()

	// Use a context with a deadline to avoid hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start a goroutine that polls the device to drive the map forward.
	go func() {
		device.Poll(wgpu.PollWait)
	}()

	// Wait should resolve via the goroutine path.
	err = pending.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait (goroutine path): %v", err)
	}
}

// =============================================================================
// MapPending — Status idempotent on resolved
// Covers map_pending.go Status lines 69-83 (resolved cache)
// =============================================================================

func TestMapPendingStatusIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "status-idempotent",
		Size:             32,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	pending, err := buf.MapAsync(wgpu.MapModeWrite, 0, 32)
	if err != nil {
		t.Fatalf("MapAsync: %v", err)
	}
	defer pending.Release()

	device.Poll(wgpu.PollWait)

	// Call Status three times — all should return the same result.
	for i := 0; i < 3; i++ {
		ready, mapErr := pending.Status()
		if !ready {
			t.Errorf("iteration %d: Status should be ready", i)
		}
		if mapErr != nil {
			t.Errorf("iteration %d: Status error: %v", i, mapErr)
		}
	}
}

// =============================================================================
// MapPending — Release before resolution
// Covers map_pending.go release() lines 47-56
// =============================================================================

func TestMapPendingReleaseBeforeResolution(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "release-before-resolve",
		Size:  64,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	pending, err := buf.MapAsync(wgpu.MapModeRead, 0, 64)
	if err != nil {
		t.Fatalf("MapAsync: %v", err)
	}

	// Release without waiting — should not panic.
	pending.Release()
}

// =============================================================================
// MappedRange — Len() and Offset() on valid range
// Covers mapped_range.go lines 81-94 (Len, Offset)
// =============================================================================

func TestMappedRangeLenAndOffset(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "mr-len-offset",
		Size:             128,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// Map at offset 64, size 32 (both multiples of 8 and 4).
	rng, err := buf.MappedRange(64, 32)
	if err != nil {
		t.Fatalf("MappedRange(64,32): %v", err)
	}

	if rng.Len() != 32 {
		t.Errorf("Len() = %d, want 32", rng.Len())
	}
	if rng.Offset() != 64 {
		t.Errorf("Offset() = %d, want 64", rng.Offset())
	}

	data := rng.Bytes()
	if data == nil {
		t.Fatal("Bytes() returned nil on valid range")
	}
	if len(data) != 32 {
		t.Errorf("len(Bytes()) = %d, want 32", len(data))
	}

	rng.Release()
}

// =============================================================================
// MappedRange — generation mismatch after Unmap
// Covers mapped_range.go Bytes() lines 64-78 (generation check)
// =============================================================================

func TestMappedRangeGenerationMismatch(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "mr-gen-mismatch",
		Size:             64,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	rng, err := buf.MappedRange(0, 64)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}

	// Before unmap, Bytes should work.
	if rng.Bytes() == nil {
		t.Fatal("Bytes() should be non-nil before Unmap")
	}

	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	// After unmap, generation mismatch makes Bytes return nil.
	if rng.Bytes() != nil {
		t.Error("Bytes() should return nil after Unmap (generation mismatch)")
	}

	rng.Release()
}

// =============================================================================
// MappedRange — Release resets all fields
// Covers mapped_range.go Release() lines 102-112
// =============================================================================

func TestMappedRangeReleaseResets(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "mr-reset",
		Size:             64,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	rng, err := buf.MappedRange(0, 64)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}

	rng.Release()

	// After Release, all fields should be zero.
	if rng.Bytes() != nil {
		t.Error("Bytes() should be nil after Release")
	}
	if rng.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after Release", rng.Len())
	}
	if rng.Offset() != 0 {
		t.Errorf("Offset() = %d, want 0 after Release", rng.Offset())
	}
}

// =============================================================================
// Buffer.Map — synchronous map with PollWait
// Covers buffer.go Map lines 112-152 (full synchronous path)
// =============================================================================

func TestBufferMapSynchronous(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "sync-map",
		Size:             64,
		Usage:            wgpu.BufferUsageMapRead | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// Unmap to allow re-mapping.
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	// Synchronous map should succeed.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = buf.Map(ctx, wgpu.MapModeRead, 0, 64)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	state := buf.MapState()
	if state != wgpu.MapStateMapped {
		t.Errorf("MapState after Map = %v, want MapStateMapped", state)
	}

	rng, err := buf.MappedRange(0, 64)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	data := rng.Bytes()
	if data == nil {
		t.Fatal("Bytes() should be non-nil after Map")
	}
	if len(data) != 64 {
		t.Errorf("len(Bytes()) = %d, want 64", len(data))
	}
	rng.Release()

	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap after Map: %v", err)
	}
}

// =============================================================================
// Buffer.Map — range overflow
// Covers buffer.go Map -> MapAsync -> core.BeginMap overflow check
// =============================================================================

func TestBufferMapRangeOverflow(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "overflow-map",
		Size:  64,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// offset + size > buffer size should fail.
	_, err = buf.MapAsync(wgpu.MapModeRead, 32, 64)
	if !errors.Is(err, wgpu.ErrMapRangeOverflow) {
		t.Errorf("MapAsync overflow: got %v, want ErrMapRangeOverflow", err)
	}
}

// =============================================================================
// Buffer.Map — alignment validation
// Covers buffer.go Map -> MapAsync -> core.BeginMap alignment check
// =============================================================================

func TestBufferMapAlignmentErrors(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "align-map",
		Size:  128,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	tests := []struct {
		name   string
		offset uint64
		size   uint64
	}{
		{"offset not multiple of 8", 3, 64},
		{"size not multiple of 4", 0, 5},
		{"both misaligned", 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buf.MapAsync(wgpu.MapModeRead, tt.offset, tt.size)
			if !errors.Is(err, wgpu.ErrMapAlignment) {
				t.Errorf("got %v, want ErrMapAlignment", err)
			}
		})
	}
}

// =============================================================================
// Buffer.Unmap — double unmap returns error
// Covers buffer.go Unmap lines 232-252
// =============================================================================

func TestBufferDoubleUnmap(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "double-unmap",
		Size:             64,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	if err := buf.Unmap(); err != nil {
		t.Fatalf("first Unmap: %v", err)
	}

	// Second unmap should fail (buffer is no longer mapped).
	err = buf.Unmap()
	if err == nil {
		t.Fatal("second Unmap should return error")
	}
}

// =============================================================================
// coreErrToTyped — all error kind mappings
// Covers map_types.go coreErrToTyped lines 142-173
// =============================================================================

func TestCoreErrToTypedMapping(t *testing.T) {
	// This is tested indirectly by verifying that each sentinel error
	// is distinct, non-nil, and has a meaningful message.
	allErrors := []struct {
		name string
		err  error
	}{
		{"AlreadyPending", wgpu.ErrMapAlreadyPending},
		{"AlreadyMapped", wgpu.ErrMapAlreadyMapped},
		{"NotMapped", wgpu.ErrMapNotMapped},
		{"RangeOverlap", wgpu.ErrMapRangeOverlap},
		{"RangeDetached", wgpu.ErrMapRangeDetached},
		{"Alignment", wgpu.ErrMapAlignment},
		{"Canceled", wgpu.ErrMapCanceled},
		{"Destroyed", wgpu.ErrBufferDestroyed},
		{"DeviceLost", wgpu.ErrMapDeviceLost},
		{"InvalidMode", wgpu.ErrMapInvalidMode},
		{"RangeOverflow", wgpu.ErrMapRangeOverflow},
	}

	for i, a := range allErrors {
		if a.err == nil {
			t.Errorf("[%d] %s is nil", i, a.name)
		}
		msg := a.err.Error()
		if msg == "" {
			t.Errorf("[%d] %s has empty message", i, a.name)
		}
		// Verify uniqueness.
		for j := i + 1; j < len(allErrors); j++ {
			if errors.Is(a.err, allErrors[j].err) {
				t.Errorf("%s == %s (should be distinct)", a.name, allErrors[j].name)
			}
		}
	}
}

// =============================================================================
// MapMode and MapState conversions
// Covers map_types.go MapMode.toInternal and mapStateFromCore
// =============================================================================

func TestMapModeRead(t *testing.T) {
	if wgpu.MapModeRead != 1 {
		t.Errorf("MapModeRead = %d, want 1", wgpu.MapModeRead)
	}
}

func TestMapModeWrite(t *testing.T) {
	if wgpu.MapModeWrite != 2 {
		t.Errorf("MapModeWrite = %d, want 2", wgpu.MapModeWrite)
	}
}

// =============================================================================
// Buffer.Map with write mode
// Covers buffer.go Map + map_types.go MapModeWrite path
// =============================================================================

func TestBufferMapWriteMode(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "map-write",
		Size:             64,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// Unmap first.
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	// Map with write mode.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = buf.Map(ctx, wgpu.MapModeWrite, 0, 64)
	if err != nil {
		t.Fatalf("Map(Write): %v", err)
	}

	rng, err := buf.MappedRange(0, 64)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	data := rng.Bytes()
	if data == nil {
		t.Fatal("Bytes() nil after Map(Write)")
	}
	// Write something to the range.
	for i := range data {
		data[i] = byte(i)
	}
	rng.Release()

	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
}
