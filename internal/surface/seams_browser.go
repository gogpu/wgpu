//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package surface holds the surface domain's implementation (ADR-070 S10).
//
// On the browser build the domain's whole view of the device is the GPUDevice
// wrapper a canvas context is configured against. The Surface implementation
// moves here in the S10 Move; the root package keeps a declaration layer over
// it.
package surface

import "github.com/gogpu/wgpu/internal/browser"

// DeviceHost is the surface domain's seam view of the device, sized by measure:
// one method. The browser surface never touches the device beyond the GPUDevice
// its canvas context is configured with, so Browser() is the whole seam.
// The root host adapts at the call boundary until S11 moves the device.
type DeviceHost interface {
	// Browser returns the browser device whose GPUDevice the canvas context is
	// configured against.
	Browser() *browser.Device
}
