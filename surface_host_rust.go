//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/wgpu/internal/surface"
)

// surfaceDeviceHost adapts *Device to the surface domain's DeviceHost seam on
// the rust build: the one wgpu-native handle a surface is configured against
// (ADR-070 §15.9). The concrete device stays in this package until S11 moves it
// into internal/device; this adapter is the only place the destination's seam
// meets it.
//
// Why an adapter and not the Device itself: R() would have to be an exported
// method on the public Device naming a wgpu-native type, and the API golden
// prints every exported method.
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

// R returns the wgpu-native device a surface is configured against.
func (h surfaceDeviceHost) R() *rwgpu.Device { return h.d.r }
