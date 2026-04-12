// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/wgpu"
)

// FEAT-WGPU-MAPPING-001 — public Buffer mapping API tests.
//
// These tests exercise the WebGPU-compliant Buffer.Map / MapAsync /
// MappedRange / Unmap path end-to-end. Every test uses the same test
// helper (createTestDevice) as the existing integration tests and
// skips when no GPU backend is available.

// createMapReadBuf is a small helper that returns a MAP_READ + COPY_DST
// buffer of the given size.
func createMapReadBuf(t *testing.T, device *wgpu.Device, size uint64) *wgpu.Buffer {
	t.Helper()
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "map-read-test",
		Size:  size,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	return buf
}

// TestBufferMapRoundTrip writes a pattern through Queue.WriteBuffer,
// maps it for read, and verifies the data round-trips.
func TestBufferMapRoundTrip(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 64
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	want := make([]byte, size)
	for i := range want {
		want[i] = byte(i*3 + 1)
	}
	if err := device.Queue().WriteBuffer(buf, 0, want); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		_ = buf.Unmap()
		t.Fatalf("MappedRange: %v", err)
	}
	got := rng.Bytes()
	if len(got) != size {
		t.Fatalf("Bytes len: got %d want %d", len(got), size)
	}
	for i := 0; i < size; i++ {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x want 0x%02x", i, got[i], want[i])
		}
	}
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	if s := buf.MapState(); s != wgpu.MapStateUnmapped {
		t.Errorf("MapState after Unmap: got %v want Unmapped", s)
	}
}

// TestBufferMapAlignment verifies that the WebGPU MAP_ALIGNMENT rules
// (offset%8==0, size%4==0) are enforced synchronously.
func TestBufferMapAlignment(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 64)
	defer buf.Release()

	cases := []struct {
		name          string
		offset, size  uint64
		expectFailure bool
	}{
		{"aligned", 0, 16, false},
		{"offset-misaligned-1", 1, 16, true},
		{"offset-misaligned-4", 4, 16, true},
		{"size-misaligned-1", 0, 17, true},
		{"size-misaligned-2", 0, 18, true},
		{"size-mul-4", 0, 12, false},
		{"offset-8", 8, 16, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := buf.Map(ctx, wgpu.MapModeRead, c.offset, c.size)
			switch {
			case c.expectFailure && err == nil:
				_ = buf.Unmap()
				t.Fatalf("expected alignment error, got nil")
			case c.expectFailure && !errors.Is(err, wgpu.ErrMapAlignment):
				_ = buf.Unmap()
				t.Fatalf("expected ErrMapAlignment, got %v", err)
			case !c.expectFailure && err != nil:
				t.Fatalf("unexpected err: %v", err)
			case !c.expectFailure:
				_ = buf.Unmap()
			}
		})
	}
}

// TestBufferMapDoubleMapFails — calling Map twice on an already-mapped
// buffer must return ErrMapAlreadyMapped synchronously.
func TestBufferMapDoubleMapFails(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 32)
	defer buf.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 32); err != nil {
		t.Fatalf("first Map: %v", err)
	}
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 32); !errors.Is(err, wgpu.ErrMapAlreadyMapped) {
		t.Fatalf("second Map: got %v, want ErrMapAlreadyMapped", err)
	}
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
}

// TestBufferMapWrongUsageFails — mapping a buffer that was not created
// with BufferUsageMapRead must fail with ErrMapInvalidMode.
func TestBufferMapWrongUsageFails(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	// Storage-only buffer (no MAP_READ).
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "storage-only",
		Size:  64,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 64); !errors.Is(err, wgpu.ErrMapInvalidMode) {
		t.Fatalf("expected ErrMapInvalidMode, got %v", err)
	}
}

// TestBufferMappedRangeOverlap verifies that WebGPU spec §5.3.4's ban
// on overlapping MappedRange calls is enforced.
func TestBufferMappedRangeOverlap(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 128)
	defer buf.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 128); err != nil {
		t.Fatalf("Map: %v", err)
	}
	defer func() { _ = buf.Unmap() }()

	if _, err := buf.MappedRange(0, 64); err != nil {
		t.Fatalf("first MappedRange: %v", err)
	}
	// Overlap at byte 32..64.
	if _, err := buf.MappedRange(32, 64); !errors.Is(err, wgpu.ErrMapRangeOverlap) {
		t.Fatalf("second MappedRange: got %v, want ErrMapRangeOverlap", err)
	}
	// Non-overlapping — 64..128 is fine.
	if _, err := buf.MappedRange(64, 64); err != nil {
		t.Fatalf("third MappedRange (non-overlapping): %v", err)
	}
}

// TestBufferMappedRangeDetachedAfterUnmap verifies the anti-UAF guard:
// calling Bytes on a MappedRange after Unmap returns nil rather than
// exposing freed memory.
func TestBufferMappedRangeDetachedAfterUnmap(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 32)
	defer buf.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 32); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := buf.MappedRange(0, 32)
	if err != nil {
		_ = buf.Unmap()
		t.Fatalf("MappedRange: %v", err)
	}
	if len(rng.Bytes()) != 32 {
		_ = buf.Unmap()
		t.Fatalf("Bytes() before Unmap: want 32 bytes")
	}
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	if b := rng.Bytes(); b != nil {
		t.Errorf("Bytes() after Unmap: got %d bytes, want nil", len(b))
	}
}

// TestBufferUnmapIdempotency — second Unmap returns ErrMapNotMapped.
func TestBufferUnmapIdempotency(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 16)
	defer buf.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 16); err != nil {
		t.Fatalf("Map: %v", err)
	}
	if err := buf.Unmap(); err != nil {
		t.Fatalf("first Unmap: %v", err)
	}
	if err := buf.Unmap(); !errors.Is(err, wgpu.ErrMapNotMapped) {
		t.Fatalf("second Unmap: got %v, want ErrMapNotMapped", err)
	}
}

// TestBufferMapStateTransitions checks the observable state transitions.
func TestBufferMapStateTransitions(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 16)
	defer buf.Release()

	if s := buf.MapState(); s != wgpu.MapStateUnmapped {
		t.Errorf("initial state: got %v want Unmapped", s)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, 16); err != nil {
		t.Fatalf("Map: %v", err)
	}
	if s := buf.MapState(); s != wgpu.MapStateMapped {
		t.Errorf("after Map: got %v want Mapped", s)
	}
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	if s := buf.MapState(); s != wgpu.MapStateUnmapped {
		t.Errorf("after Unmap: got %v want Unmapped", s)
	}
}

// TestBufferMapConcurrentPoll verifies that multiple goroutines can call
// Device.Poll concurrently without deadlocking.
func TestBufferMapConcurrentPoll(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 16; j++ {
				_ = device.Poll(wgpu.PollPoll)
			}
		}()
	}
	wg.Wait()
}

// TestBufferMapContextCancel — canceling the context before the Map
// resolves should leave the buffer in the Unmapped state (the Pending
// request is canceled by a follow-up Unmap).
func TestBufferMapContextCancel(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf := createMapReadBuf(t, device, 32)
	defer buf.Release()

	// Software / noop backends resolve Map synchronously inside the
	// first Poll, so a canceled context behaves like "immediate
	// success". For GPU backends the call may actually wait; in either
	// case we then Unmap and expect no state leak.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = buf.Map(ctx, wgpu.MapModeRead, 0, 32)
	// Unmap should succeed whether the map resolved or was canceled.
	_ = buf.Unmap()
	if s := buf.MapState(); s != wgpu.MapStateUnmapped {
		t.Errorf("post-cancel state: got %v want Unmapped", s)
	}
}

// TestBufferMapAsyncStatus tests the escape hatch path: MapAsync returns
// a MapPending whose Status resolves after Device.Poll.
func TestBufferMapAsyncStatus(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 32
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	src := make([]byte, size)
	for i := range src {
		src[i] = byte(i)
	}
	if err := device.Queue().WriteBuffer(buf, 0, src); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}

	pending, err := buf.MapAsync(wgpu.MapModeRead, 0, size)
	if err != nil {
		t.Fatalf("MapAsync: %v", err)
	}
	// Drive the device at least once.
	device.Poll(wgpu.PollWait)
	ready, statusErr := pending.Status()
	if !ready {
		t.Fatalf("Status after PollWait: not ready")
	}
	if statusErr != nil {
		t.Fatalf("Status: %v", statusErr)
	}
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		_ = buf.Unmap()
		t.Fatalf("MappedRange: %v", err)
	}
	if binary.LittleEndian.Uint32(rng.Bytes()[:4]) != binary.LittleEndian.Uint32(src[:4]) {
		t.Errorf("data mismatch")
	}
	_ = buf.Unmap()
}
