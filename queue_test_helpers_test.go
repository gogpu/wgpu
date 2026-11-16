//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/pendingwrites"
)

// The root package's queue fixtures. They assemble the public device around a
// queue the test controls; the queue's own implementation lives in
// internal/queue (ADR-070 S9), so the queue-facing assertions live there and
// these fixtures only build the shape the public API exposes.
//
// The helpers moved here from queue_barrier_test.go when the queue-internal
// tests moved to internal/queue: bind_group_usage_test.go,
// encoder_usage_scope_test.go, texture_copy_scope_test.go,
// texture_copy_lifetime_test.go and texture_view_identity_native_test.go keep
// using them.

// newTestDeviceWithQueue assembles a device around a hand-built queue: the
// queue submits through halQueue, holds pending as its pending-write batch
// (nil disables batching) and gets pool as the device's HAL encoder pool. The
// queue's device seam is the device itself, exactly as the production
// constructors wire it.
func newTestDeviceWithQueue(coreDevice *core.Device, pool *encoderPool, halQueue hal.Queue, pending *pendingwrites.Writes) *Device {
	device := &Device{core: coreDevice}
	replaceTestQueue(device, pool, halQueue, pending)
	return device
}

// replaceTestQueue installs a hand-built queue on device: the queue submits
// through halQueue, holds pending and gets pool as the device's HAL encoder
// pool. Its device seam is the device itself.
func replaceTestQueue(device *Device, pool *encoderPool, halQueue hal.Queue, pending *pendingwrites.Writes) {
	device.cmdEncoderPool = pool
	q := newQueue(halQueue, pending)
	q.impl.SetDevice(newQueueDeviceHost(device))
	device.queue = q
}

// newTestDeviceWithTracker creates a device that has a DeviceTracker and
// encoder pool, using the noop backend.
func newTestDeviceWithTracker(t *testing.T) (*Instance, *Adapter, *Device) {
	t.Helper()
	inst, adapter, device := newTestDevice(t)
	if device.core == nil || device.core.Tracker() == nil {
		t.Skip("device has no tracker (no HAL integration)")
	}
	return inst, adapter, device
}

// newTestDevice creates a noop test device (internal package wgpu access).
// The tracker tests record copies with nil regions and assert the tracker's
// state, not a real transfer, which needs the noop backend. Backends register
// by default now (ADR-083 §2.1), so CreateInstance(nil) would pick a real GPU.
func newTestDevice(t *testing.T) (*Instance, *Adapter, *Device) {
	t.Helper()
	return newTestDeviceThrough(t, nil)
}

// newTestDeviceWithHALQueue creates the same noop test device, but its queue
// submits through wrap(opened hal queue): the shape a test needs when it
// drives the queue's completion behavior (a delayed PollCompleted).
func newTestDeviceWithHALQueue(t *testing.T, wrap func(hal.Queue) hal.Queue) *Device {
	t.Helper()
	_, _, device := newTestDeviceThrough(t, wrap)
	return device
}

// newTestDeviceThrough creates a noop test device. A non-nil wrap replaces the
// backend's HAL queue before the device is built.
func newTestDeviceThrough(t *testing.T, wrap func(hal.Queue) hal.Queue) (*Instance, *Adapter, *Device) {
	t.Helper()
	instance, err := noop.NewBackend().CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		t.Fatalf("CreateInstance(noop): %v", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) != 1 {
		instance.Destroy()
		t.Fatalf("noop adapters: got %d, want 1", len(adapters))
	}
	open, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		instance.Destroy()
		t.Fatalf("Open(noop): %v", err)
	}
	halQueue := open.Queue
	if wrap != nil {
		halQueue = wrap(open.Queue)
	}
	device := newDeviceFromHAL(open.Device, halQueue, 0, adapters[0].Capabilities.Limits, "test-noop")
	// The caller defers device.Release(); Release is idempotent, so releasing
	// here too keeps the instance alive until the device is actually gone.
	t.Cleanup(func() {
		device.Release()
		adapters[0].Adapter.Destroy()
		instance.Destroy()
	})
	return nil, nil, device
}
