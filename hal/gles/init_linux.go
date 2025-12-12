// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import "github.com/gogpu/wgpu/hal"

// init registers the OpenGL ES backend with the HAL registry.
func init() {
	hal.RegisterBackend(Backend{})
}
