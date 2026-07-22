//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// releaseTrackingDevice embeds a real HAL device and records only the two
// lifecycle calls whose ordering is required by Device.Release. Embedding keeps
// this test seam local without expanding the HAL or public wgpu interfaces.
type releaseTrackingDevice struct {
	hal.Device
	events *[]string
}

func (d *releaseTrackingDevice) WaitIdle() error {
	*d.events = append(*d.events, "wait-idle")
	return d.Device.WaitIdle()
}

func (d *releaseTrackingDevice) Destroy() {
	*d.events = append(*d.events, "destroy")
	d.Device.Destroy()
}

func TestDeviceReleaseWaitsBeforeHALDestroy(t *testing.T) {
	instance, err := (noop.API{}).CreateInstance(nil)
	if err != nil {
		t.Fatalf("noop CreateInstance: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) != 1 {
		t.Fatalf("noop adapter count = %d, want 1", len(adapters))
	}
	limits := gputypes.DefaultLimits()
	opened, err := adapters[0].Adapter.Open(0, limits)
	if err != nil {
		t.Fatalf("open noop adapter: %v", err)
	}

	events := make([]string, 0, 2)
	tracked := &releaseTrackingDevice{Device: opened.Device, events: &events}
	device := &Device{
		core:  core.NewDevice(tracked, nil, 0, limits, "release-order-test"),
		queue: &Queue{hal: opened.Queue, halDevice: tracked},
	}
	device.queue.device = device

	device.Release()

	if len(events) != 2 || events[0] != "wait-idle" || events[1] != "destroy" {
		t.Fatalf("HAL lifecycle events = %v, want [wait-idle destroy]", events)
	}
}
