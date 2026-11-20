//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/loststate"
)

// This file is the native device domain's half of the device-lost and
// uncaptured-error callbacks (P1-API-042 / #417). The state machine, the latch
// and the two-phase dispatch live on the core device, which is where the
// encoder funnel and the queue record; this type is the seam every other
// package (and the root declaration layer) reaches them through, and it adds
// the two methods the surface domain's host seam declares.

// SetLostCallback replaces the device-lost registration; nil clears it without
// delivering anything.
//
// A registration made on an already-lost device receives the stored info
// exactly once, at this call's return: no device lock is held here, so the
// late-subscriber delivery is invoked directly (C5). A replaced registration
// never receives a past event again.
func (d *Device) SetLostCallback(fn func(loststate.Info)) {
	if d == nil || d.core == nil {
		return
	}
	if fire, ok := d.core.SetLostCallback(fn); ok && fire.Fn != nil {
		fire.Fn(fire.Info)
	}
	d.DispatchPending()
}

// SetUncapturedErrorCallback replaces the uncaptured-error registration; nil
// clears it and a replaced registration never receives a past event again. The
// callback receives the SAME *core.GPUError class the error scopes capture, so
// errors.Is against the category sentinels classifies both identically.
func (d *Device) SetUncapturedErrorCallback(fn func(*core.GPUError)) {
	if d == nil || d.core == nil {
		return
	}
	d.core.SetUncapturedErrorCallback(fn)
}

// LostInfo reports the device's current lost state; ok is false on a live
// device.
func (d *Device) LostInfo() (loststate.Info, bool) {
	if d == nil || d.core == nil {
		return loststate.Info{}, false
	}
	return d.core.LostInfo()
}

// DispatchPending delivers the callbacks the device's recording paths queued
// and invokes them, outside every device lock (C4). Entry points call it after
// releasing their own locks.
func (d *Device) DispatchPending() {
	if d == nil || d.core == nil {
		return
	}
	d.core.DispatchPending()
}

// MarkDeviceLost records a driver-initiated device loss. It is the surface
// domain's host seam method: the surface observes it on the acquire and present
// paths (D5, C3) and has no reason to know the reason enum — a surface-observed
// loss is always the driver's (Unknown).
//
// Only queues the delivery; the surface calls DispatchDeviceCallbacks once its
// own call has released the core surface's lock.
func (d *Device) MarkDeviceLost(message string) {
	if d == nil || d.core == nil {
		return
	}
	d.core.MarkLost(loststate.ReasonUnknown, message)
}

// DispatchDeviceCallbacks is the surface domain's host seam method for
// DispatchPending: the surface reports and delivers through the host it holds,
// never through a core pointer of its own.
func (d *Device) DispatchDeviceCallbacks() { d.DispatchPending() }
