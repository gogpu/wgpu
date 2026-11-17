//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file declares the device domain's view of the instance on the rust
// build. It is the measured access set of the moving device code on tree
// 1d1778e: the device reads the instance handle in exactly two places —
// PopErrorScope (device_rust.go, :503, :506) and the HAL handle report
// (hal_interop_rust.go, :41). Both want the same plain handle, so one method
// is the whole seam (ADR-070 §15.9). The root host adapts at the call boundary
// until the device moves.

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// InstanceHost is the device domain's view of the instance on the rust build,
// sized by measure: one method. wgpu-native's instance is a plain handle; a
// device stores nothing else from it.
type InstanceHost interface {
	// InstanceR returns the wgpu-native instance the device was created from,
	// or nil when the device is unowned. PopErrorScope pops the error scope on
	// it, and the HAL handle report reads it; a nil result is reported as a
	// zero instance handle, never dereferenced.
	InstanceR() *rwgpu.Instance
}
