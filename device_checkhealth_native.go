//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// CheckHealth returns an error when the backend reports the device as lost or
// unhealthy, and nil when the backend has no health check.
//
// A lost device's error satisfies errors.Is(err, ErrDeviceLost). Today only
// the DX12 backend implements the check (DEVICE_REMOVED diagnostics); the
// other backends have no health signal and return nil.
func (d *Device) CheckHealth() error {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.CheckHealth()
}
