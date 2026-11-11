// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// healthDevice is a HAL device that adds CheckHealth to a real backend device.
type healthDevice struct {
	hal.Device
	err error
}

func (d *healthDevice) CheckHealth(string) error { return d.err }

// openNoopHAL opens a private noop HAL device, its queue and its limits.
func openNoopHAL(t *testing.T) (hal.Device, hal.Queue, gputypes.Limits, func()) {
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
		adapters[0].Adapter.Destroy()
		instance.Destroy()
		t.Fatalf("Open(noop): %v", err)
	}
	return open.Device, open.Queue, adapters[0].Capabilities.Limits, func() {
		adapters[0].Adapter.Destroy()
		instance.Destroy()
	}
}

func TestDeviceCheckHealthDispatch(t *testing.T) {
	halDevice, halQueue, limits, closeHAL := openNoopHAL(t)
	t.Cleanup(closeHAL)

	healthErr := errors.New("dx12: device removed")
	device := newDeviceFromHAL(&healthDevice{Device: halDevice, err: healthErr}, halQueue, 0, limits, "test-health")
	t.Cleanup(device.Release)

	if got := device.CheckHealth(); !errors.Is(got, healthErr) {
		t.Fatalf("CheckHealth = %v, want the backend's error", got)
	}

	device.Release()
	if got := device.CheckHealth(); !errors.Is(got, ErrReleased) {
		t.Fatalf("CheckHealth after Release = %v, want ErrReleased", got)
	}

	var nilDevice *Device
	if got := nilDevice.CheckHealth(); got != nil {
		t.Fatalf("CheckHealth on a nil device = %v, want nil", got)
	}
}

func TestDeviceCheckHealthWithoutBackendCheck(t *testing.T) {
	halDevice, halQueue, limits, closeHAL := openNoopHAL(t)
	t.Cleanup(closeHAL)

	// The noop backend has no health signal: CheckHealth reports nil.
	device := newDeviceFromHAL(halDevice, halQueue, 0, limits, "test-noop")
	t.Cleanup(device.Release)

	if got := device.CheckHealth(); got != nil {
		t.Fatalf("CheckHealth = %v, want nil", got)
	}
}
