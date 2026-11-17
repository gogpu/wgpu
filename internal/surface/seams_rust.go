//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package surface holds the surface domain's implementation (ADR-070 S10).
//
// On the rust build the domain's whole view of the device is the wgpu-native
// handle a surface is configured against. The Surface implementation lives
// here; the root package keeps a declaration layer over it.
package surface

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// DeviceHost is the surface domain's seam view of the device, sized by measure:
// one method. wgpu-native's WGPUDevice is a plain handle; a surface stores
// nothing else from its device, so R() is the whole seam (ADR-070 §15.9).
// The root host adapts at the call boundary until S11 moves the device.
type DeviceHost interface {
	// R returns the wgpu-native device a surface is configured against; its
	// configure call is the one place the surface hands it on.
	R() *rwgpu.Device
}
