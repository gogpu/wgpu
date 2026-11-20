//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// This file is the browser build's public declaration layer for the device-lost
// and uncaptured-error callbacks (P1-API-042 / #417). The registration state
// machine and the JS handlers live in internal/browser; what stays here is the
// frozen public shape and the conversion to the public DeviceLostInfo and
// GPUError.

import (
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/loststate"
)

// SetLostCallback registers fn to be invoked when the device is lost. Each
// registration is delivered at most once: setting a callback on an already-lost
// device invokes it once with the stored DeviceLostInfo, and replacing it
// afterwards does not replay. Passing nil clears the registration without
// delivering anything.
//
// On the browser build both ways a device is lost are reported: a
// driver-initiated loss (DeviceLostReasonUnknown) arrives through the
// GPUDevice.lost promise attached at construction, and (*Device).Release
// reports DeviceLostReasonDestroyed. The first loss observed wins, so a
// Release after a driver loss keeps the driver's reason. An uncaptured error is
// delivered through SetUncapturedErrorCallback.
//
// The callback runs on the goroutine of the call that observes the event —
// on this build the single JS thread running the WebGPU event loop — never on
// a background thread and never with an internal lock held. It may call
// LostInfo and other read-only accessors; it must not enqueue GPU work.
func (d *Device) SetLostCallback(fn func(DeviceLostInfo)) {
	b := d.lostBrowserDevice()
	if b == nil {
		return
	}
	if fn == nil {
		b.SetLostCallback(nil)
		return
	}
	b.SetLostCallback(browserLostDelivery(fn).deliver)
}

// SetUncapturedErrorCallback registers fn for errors that reached no error
// scope. The callback receives the same *GPUError type the error scopes
// capture, so errors.Is against ErrValidation, ErrOutOfMemory or ErrInternal
// classifies it identically. An error captured by a scope is never delivered
// here, and a replaced registration never receives an error already delivered.
// Passing nil clears the registration.
//
// On the browser build the source is the GPUDevice's uncapturederror event, so
// every uncaptured error the browser reports is delivered. The callback runs
// synchronously on the JS thread when the event arrives, with no internal lock
// held; it must not enqueue GPU work.
func (d *Device) SetUncapturedErrorCallback(fn func(*GPUError)) {
	b := d.lostBrowserDevice()
	if b == nil {
		return
	}
	if fn == nil {
		b.SetUncapturedErrorCallback(nil)
		return
	}
	b.SetUncapturedErrorCallback(browserErrorDelivery(fn).deliver)
}

// LostInfo reports the device's current lost state: ok is false while the
// device is alive, and after a loss it returns the stored DeviceLostInfo for
// late subscribers and polling alike. On the browser build the loss is latched
// when the GPUDevice.lost promise resolves, or at Release, whichever is first;
// the first observation wins.
func (d *Device) LostInfo() (DeviceLostInfo, bool) {
	b := d.lostBrowserDevice()
	if b == nil {
		return DeviceLostInfo{}, false
	}
	info, ok := b.LostInfo()
	if !ok {
		return DeviceLostInfo{}, false
	}
	return browserDeviceLostInfo(info), true
}

// lostBrowserDevice returns the browser device behind d, or nil for a nil or
// zero-value handle.
func (d *Device) lostBrowserDevice() *browser.Device {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.Browser()
}

// browserDeviceLostInfo converts the internal loss payload to the public type.
func browserDeviceLostInfo(info loststate.Info) DeviceLostInfo {
	return DeviceLostInfo{
		Reason:  publicLostReason(info.Reason),
		Message: info.Message,
	}
}

// browserLostDelivery adapts the caller's public lost callback to the internal
// delivery type as a METHOD VALUE, the shape the public-layer rule documents
// for a boundary conversion (ADR-070 §15.10).
type browserLostDelivery func(DeviceLostInfo)

func (fn browserLostDelivery) deliver(info loststate.Info) { fn(browserDeviceLostInfo(info)) }

// browserErrorDelivery is browserLostDelivery for the uncaptured-error
// callback: one conversion from the browser error payload to the public
// GPUError, so the public Unwrap keeps the same class identity.
type browserErrorDelivery func(*GPUError)

func (fn browserErrorDelivery) deliver(info *browser.GPUErrorInfo) {
	if info == nil {
		return
	}
	fn(&GPUError{Type: ErrorFilter(info.Type), Message: info.Message})
}
