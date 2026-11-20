//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"errors"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/loststate"
)

// CheckHealth returns an error when the backend reports the device as lost or
// unhealthy, and nil when the backend has no health check.
//
// A lost device's error satisfies errors.Is(err, ErrDeviceLost). Today only
// the DX12 backend implements the check (DEVICE_REMOVED diagnostics); the
// other backends have no health signal and return nil.
//
// The device-lost callback (Device.SetLostCallback) is the preferred
// observation path on every build: it fires on the loss the runtime observes
// itself (submit, poll, present) and needs no polling. CheckHealth stays as a
// polling convenience for callers that already have a frame loop.
//
// A removal observed here also takes the lost path — LostInfo reports it and
// the lost callback fires — so polling CheckHealth and the callback cannot
// disagree (review P3-2). The delivery runs after the snatch read guard is
// released, outside every lock a callback can re-enter.
func (d *Device) CheckHealth() error {
	if d == nil {
		return nil
	}
	if d.released.Load() {
		return core.ErrReleased
	}
	if d.core == nil || !d.core.HasHAL() {
		return nil
	}
	err := d.checkHALHealth()
	if err == nil || !errors.Is(err, hal.ErrDeviceLost) {
		return err
	}
	d.core.MarkLost(loststate.ReasonUnknown, err.Error())
	d.core.DispatchPending()
	return err
}

// checkHALHealth calls the backend's CheckHealth under the snatch read guard: a
// concurrent Release must not destroy the HAL device (and its diagnostic state)
// mid-check.
func (d *Device) checkHALHealth() error {
	guard := d.core.SnatchLock().Read()
	defer guard.Release()
	halDevice := d.core.Raw(guard)
	if halDevice == nil {
		return nil
	}
	checker, ok := halDevice.(interface{ CheckHealth(string) error })
	if !ok {
		return nil
	}
	return checker.CheckHealth("Device.CheckHealth")
}
