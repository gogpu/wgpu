//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// This file is the rust build's public declaration layer for the device-lost
// and uncaptured-error callbacks (P1-API-042 / #417). The registration state
// machine lives in internal/device; what stays here is the frozen public shape
// and the conversion to the public DeviceLostInfo and GPUError.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/loststate"
)

// SetLostCallback registers fn to be invoked when the device is lost. Each
// registration is delivered at most once: setting a callback on an already-lost
// device invokes it once with the stored DeviceLostInfo, and replacing it
// afterwards does not replay. Passing nil clears the registration.
//
// Per-build limit with -tags rust: the pinned binding, webgpu v0.5.5, exposes
// no device-lost callback, so the only loss this build reports is the one
// (*Device).Release produces (DeviceLostReasonDestroyed); a driver-initiated
// loss is not observed and fn does not fire for it. The event source is wired
// at the webgpu v0.6.0 bump (card 0085). The callback runs on the goroutine of
// the call that observes the event (a concurrent call may deliver a pending
// event first; never on a background thread), with no internal lock held; it
// may call LostInfo and other read-only accessors, and must not enqueue GPU
// work.
func (d *Device) SetLostCallback(fn func(DeviceLostInfo)) {
	if d == nil || d.impl == nil {
		return
	}
	if fn == nil {
		d.impl.SetLostCallback(nil)
		return
	}
	d.impl.SetLostCallback(rustLostDelivery(fn).deliver)
}

// SetUncapturedErrorCallback registers fn for errors that reached no error
// scope, delivered with the same *GPUError type the scopes capture, so
// errors.Is against ErrValidation, ErrOutOfMemory or ErrInternal classifies it
// identically. An error captured by a scope is never delivered here. Passing
// nil clears the registration.
//
// Per-build limit with -tags rust: webgpu v0.5.5 exposes no uncaptured-error
// callback, so fn is never invoked at this pin; the source is wired at the
// webgpu v0.6.0 bump (card 0085). The callback would run on the goroutine that
// observes the event, with no internal lock held.
func (d *Device) SetUncapturedErrorCallback(fn func(*GPUError)) {
	if d == nil || d.impl == nil {
		return
	}
	if fn == nil {
		d.impl.SetUncapturedErrorCallback(nil)
		return
	}
	d.impl.SetUncapturedErrorCallback(rustErrorDelivery(fn).deliver)
}

// LostInfo reports the device's current lost state: ok is false while the
// device is alive, and after a loss it returns the stored DeviceLostInfo.
//
// Per-build limit with -tags rust: at webgpu v0.5.5 the only loss observable is
// the one (*Device).Release produces (DeviceLostReasonDestroyed); a
// driver-initiated loss is not reported until the webgpu v0.6.0 bump wires the
// lost callback (card 0085). The first loss observed wins.
func (d *Device) LostInfo() (DeviceLostInfo, bool) {
	if d == nil || d.impl == nil {
		return DeviceLostInfo{}, false
	}
	info, ok := d.impl.LostInfo()
	if !ok {
		return DeviceLostInfo{}, false
	}
	return rustDeviceLostInfo(info), true
}

// rustDeviceLostInfo converts the internal loss payload to the public type.
func rustDeviceLostInfo(info loststate.Info) DeviceLostInfo {
	return DeviceLostInfo{
		Reason:  publicLostReason(info.Reason),
		Message: info.Message,
	}
}

// rustLostDelivery adapts the caller's public lost callback to the internal
// delivery type as a METHOD VALUE, the shape the public-layer rule documents
// for a boundary conversion (ADR-070 §15.10).
type rustLostDelivery func(DeviceLostInfo)

func (fn rustLostDelivery) deliver(info loststate.Info) { fn(rustDeviceLostInfo(info)) }

// rustErrorDelivery is rustLostDelivery for the uncaptured-error callback: one
// conversion from the core error the scopes produce to the public GPUError, so
// the public Unwrap keeps the same class identity. No source reaches it at the
// webgpu v0.5.5 pin (card 0085 wires the delivery at the v0.6.0 bump); the
// adapter is the pinned seam shape.
type rustErrorDelivery func(*GPUError)

func (fn rustErrorDelivery) deliver(e *core.GPUError) {
	if e == nil {
		return
	}
	fn(&GPUError{Type: ErrorFilter(e.Type), Message: e.Message})
}
