// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import "github.com/gogpu/wgpu/hal"

// init registers the Metal backend with the HAL registry.
// This is called automatically on package import.
//
// The Metal backend is only available on macOS and iOS.
func init() {
	hal.RegisterBackend(Backend{})
}
