//go:build !(js && wasm)

package core

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
)

// =============================================================================
// BufferMapError
// =============================================================================

func TestBufferMapError_AllKinds(t *testing.T) {
	tests := []struct {
		name    string
		err     *BufferMapError
		wantSub string
	}{
		{
			name:    "AlreadyPending",
			err:     &BufferMapError{Kind: BufferMapErrKindAlreadyPending},
			wantSub: "already pending",
		},
		{
			name:    "AlreadyMapped",
			err:     &BufferMapError{Kind: BufferMapErrKindAlreadyMapped},
			wantSub: "already mapped",
		},
		{
			name:    "NotMapped",
			err:     &BufferMapError{Kind: BufferMapErrKindNotMapped},
			wantSub: "not mapped",
		},
		{
			name:    "Alignment",
			err:     &BufferMapError{Kind: BufferMapErrKindAlignment},
			wantSub: "not aligned",
		},
		{
			name:    "InvalidMode",
			err:     &BufferMapError{Kind: BufferMapErrKindInvalidMode},
			wantSub: "MAP_READ/MAP_WRITE",
		},
		{
			name:    "RangeOverflow",
			err:     &BufferMapError{Kind: BufferMapErrKindRangeOverflow},
			wantSub: "exceeds buffer size",
		},
		{
			name:    "Canceled",
			err:     &BufferMapError{Kind: BufferMapErrKindCancelled},
			wantSub: "canceled",
		},
		{
			name:    "Destroyed",
			err:     &BufferMapError{Kind: BufferMapErrKindDestroyed},
			wantSub: "destroyed",
		},
		{
			name:    "DeviceLost",
			err:     &BufferMapError{Kind: BufferMapErrKindDeviceLost},
			wantSub: "device lost",
		},
		{
			name:    "RangeOverlap",
			err:     &BufferMapError{Kind: BufferMapErrKindRangeOverlap},
			wantSub: "overlaps",
		},
		{
			name:    "RangeDetached",
			err:     &BufferMapError{Kind: BufferMapErrKindRangeDetached},
			wantSub: "detached",
		},
		{
			name:    "HAL",
			err:     &BufferMapError{Kind: BufferMapErrKindHAL},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &BufferMapError{Kind: BufferMapErrorKind(255)},
			wantSub: "unknown error",
		},
		{
			name:    "NilReceiver",
			err:     nil,
			wantSub: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if tt.wantSub == "" {
				if msg != "" {
					t.Errorf("Error() = %q, want empty", msg)
				}
				return
			}
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestBufferMapError_WithWrapped(t *testing.T) {
	inner := errors.New("underlying HAL failure")
	e := &BufferMapError{Kind: BufferMapErrKindHAL, Wrapped: inner}

	msg := e.Error()
	if !strings.Contains(msg, "underlying HAL failure") {
		t.Errorf("Error() = %q, want wrapped message", msg)
	}

	if !errors.Is(e.Unwrap(), inner) {
		t.Error("Unwrap() did not return wrapped error")
	}
}

func TestBufferMapError_UnwrapNil(t *testing.T) {
	e := &BufferMapError{Kind: BufferMapErrKindCancelled}
	if e.Unwrap() != nil {
		t.Error("Unwrap() should return nil when no wrapped error")
	}
}

// =============================================================================
// MapWaiter
// =============================================================================

func TestMapWaiter_SignalThenWait(t *testing.T) {
	w := newMapWaiter()

	// Signal success before Wait.
	w.Signal(nil)

	err := w.Wait()
	if err != nil {
		t.Errorf("Wait() returned error after successful signal: %v", err)
	}

	done, errStatus := w.Status()
	if !done {
		t.Error("Status() should report done after Signal")
	}
	if errStatus != nil {
		t.Errorf("Status() error = %v, want nil", errStatus)
	}
}

func TestMapWaiter_SignalErrorThenWait(t *testing.T) {
	w := newMapWaiter()

	mapErr := &BufferMapError{Kind: BufferMapErrKindDestroyed}
	w.Signal(mapErr)

	err := w.Wait()
	if err == nil {
		t.Fatal("Wait() should return error")
	}
	if err.Kind != BufferMapErrKindDestroyed {
		t.Errorf("Wait() error Kind = %v, want Destroyed", err.Kind)
	}
}

func TestMapWaiter_ConcurrentWait(t *testing.T) {
	w := newMapWaiter()

	var wg sync.WaitGroup
	errs := make([]*BufferMapError, 5)

	// Launch 5 goroutines all waiting.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = w.Wait()
		}(i)
	}

	// Give goroutines time to start waiting.
	time.Sleep(10 * time.Millisecond)

	// Signal -- all should wake up.
	w.Signal(nil)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d got error: %v", i, err)
		}
	}
}

func TestMapWaiter_Reset(t *testing.T) {
	w := newMapWaiter()

	// First cycle: signal then confirm done.
	w.Signal(nil)
	done, _ := w.Status()
	if !done {
		t.Fatal("should be done after Signal")
	}

	// Reset should clear the done state.
	w.Reset()
	done, _ = w.Status()
	if done {
		t.Error("should not be done after Reset")
	}

	// Second cycle: signal again.
	mapErr := &BufferMapError{Kind: BufferMapErrKindCancelled}
	w.Signal(mapErr)

	err := w.Wait()
	if err == nil || err.Kind != BufferMapErrKindCancelled {
		t.Errorf("second cycle Wait() = %v, want Canceled", err)
	}
}

// =============================================================================
// BeginMap state machine
// =============================================================================

func newTestBuffer(usage gputypes.BufferUsage, size uint64) *Buffer {
	return &Buffer{
		usage:       usage,
		size:        size,
		mapState:    BufferMapStateIdle,
		mapDataSlot: &mapDataSlot{},
	}
}

func TestBeginMap_Success(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)

	err := buf.BeginMap(MapModeInternalRead, 0, 1024)
	if err != nil {
		t.Fatalf("BeginMap() returned error: %v", err)
	}
	if buf.CurrentMapState() != BufferMapStatePending {
		t.Errorf("state = %v, want Pending", buf.CurrentMapState())
	}
}

func TestBeginMap_Destroyed(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	buf.mapState = BufferMapStateDestroyed

	err := buf.BeginMap(MapModeInternalRead, 0, 1024)
	if err == nil || err.Kind != BufferMapErrKindDestroyed {
		t.Errorf("BeginMap() on destroyed buffer = %v, want Destroyed", err)
	}
}

func TestBeginMap_AlreadyPending(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	buf.mapState = BufferMapStatePending

	err := buf.BeginMap(MapModeInternalRead, 0, 1024)
	if err == nil || err.Kind != BufferMapErrKindAlreadyPending {
		t.Errorf("BeginMap() on pending buffer = %v, want AlreadyPending", err)
	}
}

func TestBeginMap_AlreadyMapped(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	buf.mapState = BufferMapStateMapped

	err := buf.BeginMap(MapModeInternalRead, 0, 1024)
	if err == nil || err.Kind != BufferMapErrKindAlreadyMapped {
		t.Errorf("BeginMap() on mapped buffer = %v, want AlreadyMapped", err)
	}
}

func TestBeginMap_AlignmentErrors(t *testing.T) {
	tests := []struct {
		name   string
		offset uint64
		size   uint64
	}{
		{"offset not 8-aligned", 7, 4},
		{"size not 4-aligned", 0, 5},
		{"both misaligned", 3, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
			err := buf.BeginMap(MapModeInternalRead, tt.offset, tt.size)
			if err == nil || err.Kind != BufferMapErrKindAlignment {
				t.Errorf("BeginMap(%d, %d) = %v, want Alignment", tt.offset, tt.size, err)
			}
		})
	}
}

func TestBeginMap_RangeOverflow(t *testing.T) {
	tests := []struct {
		name   string
		offset uint64
		size   uint64
	}{
		{"offset > buffer size", 2048, 8},
		{"size > buffer size", 0, 2048},
		{"offset+size > buffer size", 512, 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
			err := buf.BeginMap(MapModeInternalRead, tt.offset, tt.size)
			if err == nil || err.Kind != BufferMapErrKindRangeOverflow {
				t.Errorf("BeginMap(%d, %d) = %v, want RangeOverflow", tt.offset, tt.size, err)
			}
		})
	}
}

func TestBeginMap_InvalidMode(t *testing.T) {
	tests := []struct {
		name  string
		usage gputypes.BufferUsage
		mode  MapModeInternal
	}{
		{"read without MAP_READ", gputypes.BufferUsageMapWrite, MapModeInternalRead},
		{"write without MAP_WRITE", gputypes.BufferUsageMapRead, MapModeInternalWrite},
		{"zero mode", gputypes.BufferUsageMapRead, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newTestBuffer(tt.usage, 1024)
			err := buf.BeginMap(tt.mode, 0, 1024)
			if err == nil || err.Kind != BufferMapErrKindInvalidMode {
				t.Errorf("BeginMap() = %v, want InvalidMode", err)
			}
		})
	}
}

// =============================================================================
// MarkDestroyed
// =============================================================================

func TestMarkDestroyed_IdleToDestroyed(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	buf.MarkDestroyed()
	if buf.CurrentMapState() != BufferMapStateDestroyed {
		t.Errorf("state = %v, want Destroyed", buf.CurrentMapState())
	}
}

func TestMarkDestroyed_PendingSignalsWaiter(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)

	// Enter Pending state.
	err := buf.BeginMap(MapModeInternalRead, 0, 1024)
	if err != nil {
		t.Fatalf("BeginMap failed: %v", err)
	}

	// The waiter should be signaled with Destroyed.
	waiter := buf.Waiter()
	buf.MarkDestroyed()

	mapErr := waiter.Wait()
	if mapErr == nil || mapErr.Kind != BufferMapErrKindDestroyed {
		t.Errorf("waiter got %v, want Destroyed", mapErr)
	}
}

func TestMarkDestroyed_Idempotent(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	buf.MarkDestroyed()
	buf.MarkDestroyed() // second call should be safe
	if buf.CurrentMapState() != BufferMapStateDestroyed {
		t.Errorf("state = %v after double MarkDestroyed, want Destroyed", buf.CurrentMapState())
	}
}

// =============================================================================
// UnmapBuffer
// =============================================================================

func TestUnmapBuffer_FromMapped(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	buf := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageMapRead, 1024, "test")

	// Manually set to Mapped state for testing unmap.
	md := buf.ensureMapData()
	md.mu.Lock()
	buf.mapState = BufferMapStateMapped
	md.pendingOffset = 0
	md.pendingSize = 1024
	md.pendingMode = MapModeInternalRead
	md.mu.Unlock()

	guard := device.SnatchLock().Read()
	mapErr := buf.UnmapBuffer(guard, *device.raw.Get(guard))
	guard.Release()

	if mapErr != nil {
		t.Errorf("UnmapBuffer() from Mapped = %v, want nil", mapErr)
	}
	if buf.CurrentMapState() != BufferMapStateIdle {
		t.Errorf("state = %v after Unmap, want Idle", buf.CurrentMapState())
	}
}

func TestUnmapBuffer_FromPending(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	buf := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageMapRead, 1024, "test")

	// Enter Pending.
	err := buf.BeginMap(MapModeInternalRead, 0, 1024)
	if err != nil {
		t.Fatalf("BeginMap failed: %v", err)
	}

	waiter := buf.Waiter()
	guard := device.SnatchLock().Read()
	mapErr := buf.UnmapBuffer(guard, *device.raw.Get(guard))
	guard.Release()

	if mapErr != nil {
		t.Errorf("UnmapBuffer() from Pending = %v, want nil", mapErr)
	}
	if buf.CurrentMapState() != BufferMapStateIdle {
		t.Errorf("state = %v after Unmap, want Idle", buf.CurrentMapState())
	}

	// Waiter should receive Canceled.
	wErr := waiter.Wait()
	if wErr == nil || wErr.Kind != BufferMapErrKindCancelled {
		t.Errorf("waiter got %v, want Canceled", wErr)
	}
}

func TestUnmapBuffer_FromIdle(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	// nil guard/device are ok here since the function checks state first.
	mapErr := buf.UnmapBuffer(SnatchGuard{}, nil)
	if mapErr == nil || mapErr.Kind != BufferMapErrKindNotMapped {
		t.Errorf("UnmapBuffer() from Idle = %v, want NotMapped", mapErr)
	}
}

func TestUnmapBuffer_FromDestroyed(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	buf.mapState = BufferMapStateDestroyed
	mapErr := buf.UnmapBuffer(SnatchGuard{}, nil)
	if mapErr == nil || mapErr.Kind != BufferMapErrKindDestroyed {
		t.Errorf("UnmapBuffer() from Destroyed = %v, want Destroyed", mapErr)
	}
}

// =============================================================================
// TryRegisterMappedRange
// =============================================================================

func TestTryRegisterMappedRange_Success(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	md := buf.ensureMapData()
	md.mu.Lock()
	buf.mapState = BufferMapStateMapped
	md.pendingOffset = 0
	md.pendingSize = 1024
	md.mu.Unlock()

	err := buf.TryRegisterMappedRange(0, 256)
	if err != nil {
		t.Fatalf("TryRegisterMappedRange(0, 256) failed: %v", err)
	}

	// Register another non-overlapping range.
	err = buf.TryRegisterMappedRange(256, 256)
	if err != nil {
		t.Fatalf("TryRegisterMappedRange(256, 256) failed: %v", err)
	}
}

func TestTryRegisterMappedRange_NotMapped(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	err := buf.TryRegisterMappedRange(0, 256)
	if err == nil || err.Kind != BufferMapErrKindNotMapped {
		t.Errorf("TryRegisterMappedRange on Idle buffer = %v, want NotMapped", err)
	}
}

func TestTryRegisterMappedRange_OutOfBounds(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	md := buf.ensureMapData()
	md.mu.Lock()
	buf.mapState = BufferMapStateMapped
	md.pendingOffset = 256
	md.pendingSize = 256
	md.mu.Unlock()

	// Before the mapped region.
	err := buf.TryRegisterMappedRange(0, 128)
	if err == nil || err.Kind != BufferMapErrKindRangeOverflow {
		t.Errorf("range before mapped region = %v, want RangeOverflow", err)
	}

	// After the mapped region.
	err = buf.TryRegisterMappedRange(600, 128)
	if err == nil || err.Kind != BufferMapErrKindRangeOverflow {
		t.Errorf("range after mapped region = %v, want RangeOverflow", err)
	}
}

func TestTryRegisterMappedRange_Overlap(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	md := buf.ensureMapData()
	md.mu.Lock()
	buf.mapState = BufferMapStateMapped
	md.pendingOffset = 0
	md.pendingSize = 1024
	md.mu.Unlock()

	err := buf.TryRegisterMappedRange(0, 512)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Overlapping range.
	err = buf.TryRegisterMappedRange(256, 512)
	if err == nil || err.Kind != BufferMapErrKindRangeOverlap {
		t.Errorf("overlapping range = %v, want RangeOverlap", err)
	}
}

// =============================================================================
// MappingInfo
// =============================================================================

func mappingInfoOK(buf *Buffer) bool {
	_, _, _, ok := buf.MappingInfo() //nolint:dogsled // test helper wraps multi-return
	return ok
}

func TestMappingInfo_NotMapped(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	if mappingInfoOK(buf) {
		t.Error("MappingInfo() should return ok=false for idle buffer")
	}
}

func TestMappingInfo_NilMapData(t *testing.T) {
	// Buffer without ever initializing map data.
	buf := &Buffer{mapState: BufferMapStateIdle}
	if mappingInfoOK(buf) {
		t.Error("MappingInfo() should return ok=false when mapData is nil")
	}
}

// =============================================================================
// CurrentMapState
// =============================================================================

func TestCurrentMapState_NoMapData(t *testing.T) {
	buf := &Buffer{mapState: BufferMapStateIdle}
	if buf.CurrentMapState() != BufferMapStateIdle {
		t.Errorf("CurrentMapState() = %v, want Idle", buf.CurrentMapState())
	}
}

func TestCurrentMapState_WithMapData(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	// Initialize map data.
	buf.ensureMapData()
	if buf.CurrentMapState() != BufferMapStateIdle {
		t.Errorf("CurrentMapState() = %v, want Idle", buf.CurrentMapState())
	}
}

// =============================================================================
// Generation
// =============================================================================

func TestGeneration_NilMapData(t *testing.T) {
	buf := &Buffer{mapState: BufferMapStateIdle}
	if buf.Generation() != 0 {
		t.Errorf("Generation() = %d, want 0 for nil mapData", buf.Generation())
	}
}

func TestGeneration_Increments(t *testing.T) {
	buf := newTestBuffer(gputypes.BufferUsageMapRead, 1024)
	gen0 := buf.Generation()

	// MarkDestroyed bumps generation.
	buf.MarkDestroyed()
	gen1 := buf.Generation()
	if gen1 <= gen0 {
		t.Errorf("Generation after MarkDestroyed = %d, want > %d", gen1, gen0)
	}
}
