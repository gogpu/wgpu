//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/surface"
)

// surfaceDeviceHost adapts *Device to the surface domain's DeviceHost seam on
// the browser build: the GPUDevice wrapper a canvas context is configured
// against. The concrete device stays in this package until S11 moves it into
// internal/device; this adapter is the only place the destination's seam meets
// it.
//
// Why an adapter and not the Device itself: Browser() would have to be an
// exported method on the public Device naming an internal type, which §15.1
// forbids (and the API golden prints every exported method).
type surfaceDeviceHost struct{ d *Device }

var _ surface.DeviceHost = surfaceDeviceHost{}

// newSurfaceDeviceHost returns the seam value of d, or an untyped nil for a nil
// device: the surface's "not configured" checks must keep their meaning across
// the interface.
func newSurfaceDeviceHost(d *Device) surface.DeviceHost {
	if d == nil {
		return nil
	}
	return surfaceDeviceHost{d: d}
}

// Browser returns the browser device whose GPUDevice the canvas context is
// configured against.
func (h surfaceDeviceHost) Browser() *browser.Device { return h.d.browser }
