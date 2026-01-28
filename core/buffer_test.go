package core

import (
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestBuffer_NewBuffer(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex|gputypes.BufferUsageCopySrc, 1024, "TestBuffer")

	if buffer == nil {
		t.Fatal("NewBuffer returned nil")
	}
	if !buffer.HasHAL() {
		t.Error("Buffer.HasHAL() should return true")
	}
	if buffer.Device() != device {
		t.Error("Buffer.Device() should return parent device")
	}
	if buffer.Usage() != gputypes.BufferUsageVertex|gputypes.BufferUsageCopySrc {
		t.Error("Buffer.Usage() incorrect")
	}
	if buffer.Size() != 1024 {
		t.Error("Buffer.Size() should return 1024")
	}
	if buffer.Label() != "TestBuffer" {
		t.Error("Buffer.Label() should return 'TestBuffer'")
	}
}

func TestBuffer_RawAccess(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 1024, "TestBuffer")

	lock := device.SnatchLock()
	guard := lock.Read()
	defer guard.Release()

	raw := buffer.Raw(guard)
	if raw == nil {
		t.Error("Raw() should not return nil")
	}
}

func TestBuffer_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 1024, "TestBuffer")

	if buffer.IsDestroyed() {
		t.Error("Buffer should not be destroyed initially")
	}

	buffer.Destroy()

	if !buffer.IsDestroyed() {
		t.Error("Buffer should be destroyed after Destroy()")
	}
}

func TestBuffer_DestroyIdempotent(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 1024, "TestBuffer")

	// Multiple destroy calls should be safe
	buffer.Destroy()
	buffer.Destroy()
	buffer.Destroy()

	if !buffer.IsDestroyed() {
		t.Error("Buffer should be destroyed")
	}
}

func TestBuffer_RawAfterDestroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 1024, "TestBuffer")

	buffer.Destroy()

	lock := device.SnatchLock()
	guard := lock.Read()
	defer guard.Release()

	raw := buffer.Raw(guard)
	if raw != nil {
		t.Error("Raw() should return nil after destroy")
	}
}

func TestBuffer_MapState(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageMapRead, 1024, "TestBuffer")

	if buffer.MapState() != BufferMapStateIdle {
		t.Error("Initial map state should be Idle")
	}

	buffer.SetMapState(BufferMapStatePending)
	if buffer.MapState() != BufferMapStatePending {
		t.Error("Map state should be Pending")
	}

	buffer.SetMapState(BufferMapStateMapped)
	if buffer.MapState() != BufferMapStateMapped {
		t.Error("Map state should be Mapped")
	}

	buffer.SetMapState(BufferMapStateIdle)
	if buffer.MapState() != BufferMapStateIdle {
		t.Error("Map state should be back to Idle")
	}
}

func TestBuffer_InitTracker(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	// Create buffer with 16KB to have 4 chunks of 4KB each
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 16384, "TestBuffer")

	// Initially nothing is initialized
	if buffer.IsInitialized(0, 4096) {
		t.Error("Region should not be initialized initially")
	}

	// Mark first chunk as initialized
	buffer.MarkInitialized(0, 4096)
	if !buffer.IsInitialized(0, 4096) {
		t.Error("First chunk should be initialized")
	}

	// Second chunk still not initialized
	if buffer.IsInitialized(4096, 4096) {
		t.Error("Second chunk should not be initialized")
	}

	// Mark all as initialized
	buffer.MarkInitialized(0, 16384)
	if !buffer.IsInitialized(0, 16384) {
		t.Error("All chunks should be initialized")
	}
}

func TestBuffer_TrackingData(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 1024, "TestBuffer")

	td := buffer.TrackingData()
	if td == nil {
		t.Fatal("TrackingData() should not return nil")
	}
	if td.Index() != InvalidTrackerIndex {
		t.Error("Tracker index should be invalid (stub implementation)")
	}
}

func TestBuffer_ConcurrentAccess(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	halBuffer := mockBuffer{}
	buffer := NewBuffer(halBuffer, device, gputypes.BufferUsageVertex, 1024, "TestBuffer")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock := device.SnatchLock()
			guard := lock.Read()
			defer guard.Release()
			_ = buffer.Raw(guard)
		}()
	}
	wg.Wait()
}

func TestBuffer_NoHAL(t *testing.T) {
	// Buffer without HAL integration (ID-based API style)
	buffer := &Buffer{}

	if buffer.HasHAL() {
		t.Error("Buffer without HAL should return false")
	}
	if !buffer.IsDestroyed() {
		t.Error("Buffer without HAL raw should be considered destroyed")
	}
	if buffer.Device() != nil {
		t.Error("Device should be nil")
	}

	// Destroy should be safe
	buffer.Destroy()
}

func TestBufferInitTracker_EdgeCases(t *testing.T) {
	// Zero size buffer
	tracker := NewBufferInitTracker(0)
	if !tracker.IsInitialized(0, 0) {
		t.Error("Empty tracker should return true for IsInitialized")
	}
	// Should not panic
	tracker.MarkInitialized(0, 0)

	// Nil tracker
	var nilTracker *BufferInitTracker
	if !nilTracker.IsInitialized(0, 100) {
		t.Error("Nil tracker should return true for IsInitialized")
	}
	// Should not panic
	nilTracker.MarkInitialized(0, 100)

	// Small buffer (less than chunk size)
	smallTracker := NewBufferInitTracker(100)
	smallTracker.MarkInitialized(0, 100)
	if !smallTracker.IsInitialized(0, 100) {
		t.Error("Small buffer should be initialized")
	}

	// Partial initialization
	partialTracker := NewBufferInitTracker(8192) // 2 chunks
	partialTracker.MarkInitialized(0, 4096)      // Only first chunk
	if !partialTracker.IsInitialized(0, 4096) {
		t.Error("First chunk should be initialized")
	}
	if partialTracker.IsInitialized(0, 8192) {
		t.Error("Full range should not be initialized")
	}
}

func TestBufferMapState_Constants(t *testing.T) {
	// Verify enum values are distinct
	if BufferMapStateIdle == BufferMapStatePending {
		t.Error("Idle and Pending should be different")
	}
	if BufferMapStatePending == BufferMapStateMapped {
		t.Error("Pending and Mapped should be different")
	}
	if BufferMapStateIdle == BufferMapStateMapped {
		t.Error("Idle and Mapped should be different")
	}
}

func TestTrackerIndex_InvalidConstant(t *testing.T) {
	if InvalidTrackerIndex == 0 {
		t.Error("InvalidTrackerIndex should not be 0")
	}
	// Should be max uint32
	if InvalidTrackerIndex != ^TrackerIndex(0) {
		t.Error("InvalidTrackerIndex should be max uint32")
	}
}
