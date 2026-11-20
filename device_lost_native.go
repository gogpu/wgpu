//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// This file is the native build's public declaration layer for the device-lost
// and uncaptured-error callbacks (P1-API-042 / #417). The registration state
// machine lives in internal/core, behind internal/device; what stays here is
// the frozen public shape and the conversion to the public DeviceLostInfo and
// GPUError. The browser and rust builds declare the same three methods in their
// own files, each with its own per-build note.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/loststate"
)

// SetLostCallback registers fn to be invoked when the device is lost. Each
// registration is delivered at most once: setting a callback on an already-lost
// device invokes it once with the stored DeviceLostInfo, and replacing it
// afterwards does not replay. Passing nil clears the registration.
//
// On this build (native: Vulkan, DX12, Metal, GLES and the software backend)
// both reasons are observed: a driver-initiated loss through Submit, Poll or
// Present reports DeviceLostReasonUnknown, and (*Device).Release reports
// DeviceLostReasonDestroyed. The first observation wins — a release after a
// driver loss keeps the driver's reason and does not fire again.
//
// fn runs on the goroutine of the call that observes the event — a concurrent
// call may deliver a pending event first, and it never runs on a background
// thread — with no internal device, queue or encoder lock held. It may call
// LostInfo and other read-only accessors; it must not enqueue GPU work.
func (d *Device) SetLostCallback(fn func(DeviceLostInfo)) {
	if d == nil || d.impl == nil {
		return
	}
	if fn == nil {
		d.impl.SetLostCallback(nil)
		return
	}
	d.impl.SetLostCallback(nativeLostDelivery(fn).deliver)
}

// SetUncapturedErrorCallback registers fn for errors that reached no error
// scope, delivered with the same *GPUError type the scopes capture, so
// errors.Is(err, ErrValidation), ErrOutOfMemory and ErrInternal classify it
// identically. An error captured by a pushed scope is never delivered here.
// Passing nil clears the registration, and a replaced registration never
// receives an error already delivered.
//
// On this build the source is every error the runtime reports internally —
// encode-time validation, submit-class validation, encoder budget exhaustion
// (an Internal error, never a device loss) — and fn runs on the goroutine that
// observed the error, with no internal lock held.
func (d *Device) SetUncapturedErrorCallback(fn func(*GPUError)) {
	if d == nil || d.impl == nil {
		return
	}
	if fn == nil {
		d.impl.SetUncapturedErrorCallback(nil)
		return
	}
	d.impl.SetUncapturedErrorCallback(nativeErrorDelivery(fn).deliver)
}

// LostInfo reports the device's current lost state: ok is false while the
// device is alive, and after a loss it returns the stored DeviceLostInfo. It is
// the answer for a subscriber that registered after the loss, and the reason
// PopErrorScope resolves with (nil, nil) on a lost device.
//
// On this build both reasons are observable; the first observation wins.
func (d *Device) LostInfo() (DeviceLostInfo, bool) {
	if d == nil || d.impl == nil {
		return DeviceLostInfo{}, false
	}
	info, ok := d.impl.LostInfo()
	if !ok {
		return DeviceLostInfo{}, false
	}
	return nativeDeviceLostInfo(info), true
}

// nativeDeviceLostInfo converts the internal loss payload to the public type.
func nativeDeviceLostInfo(info loststate.Info) DeviceLostInfo {
	return DeviceLostInfo{
		Reason:  publicLostReason(info.Reason),
		Message: info.Message,
	}
}

// nativeLostDelivery adapts the caller's public lost callback to the internal
// delivery type as a METHOD VALUE, the shape the public-layer rule documents
// for a boundary conversion (ADR-070 §15.10: a public package holds declarations
// and pure projections, and a closure is a flagged construct while a method on
// an unexported function type is the reviewed equivalent).
type nativeLostDelivery func(DeviceLostInfo)

func (fn nativeLostDelivery) deliver(info loststate.Info) { fn(nativeDeviceLostInfo(info)) }

// nativeErrorDelivery is nativeLostDelivery for the uncaptured-error callback:
// one conversion from the core error the scopes produce to the public GPUError,
// so the public Unwrap keeps the same class identity.
type nativeErrorDelivery func(*GPUError)

func (fn nativeErrorDelivery) deliver(e *core.GPUError) {
	if e == nil {
		return
	}
	fn(&GPUError{Type: ErrorFilter(e.Type), Message: e.Message})
}
