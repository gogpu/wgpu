//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/surface"
)

// surfaceDeviceHost adapts *Device to the surface domain's DeviceHost seam
// (ADR-070 §15.3, ruling of 2026-11-16): the surface reaches the device's core
// state, its released flag, the HAL device and the HAL queue through it. The
// concrete device stays in this package until S11 moves it into
// internal/device; this adapter is the only place the destination's seam meets
// it.
//
// Why an adapter and not the Device itself: the seam's Poll (embedded from
// core.DeviceView) takes core's PollType and the public Device.Poll takes the
// root PollType, and Go forbids two methods of one type from sharing a name
// even with different parameter types; the seam's other methods would also have
// to be public methods on Device naming internal types, which §15.1 forbids.
// The embedded value is the shared S1 adapter (device_view_native.go), cached
// on the device and reused by every child that holds it.
type surfaceDeviceHost struct {
	d *Device
	core.DeviceView
}

var _ surface.DeviceHost = surfaceDeviceHost{}

// newSurfaceDeviceHost returns the seam value of d, or an untyped nil for a nil
// device: the surface's "not configured" checks must keep their meaning across
// the interface.
func newSurfaceDeviceHost(d *Device) surface.DeviceHost {
	if d == nil {
		return nil
	}
	return surfaceDeviceHost{d: d, DeviceView: d.seamView()}
}

// HALQueue returns the HAL queue presentation goes through, or an untyped nil
// when the device has none: the surface's nil check must hold across the
// interface, so a nil *queue.Queue is never boxed here.
func (h surfaceDeviceHost) HALQueue() hal.Queue {
	if h.d.queue == nil {
		return nil
	}
	return h.d.queue.impl.HALQueue()
}
