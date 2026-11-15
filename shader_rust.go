//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// ShaderModule represents a compiled shader module.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation.
type ShaderModule struct {
	impl *resource.ShaderModule
}

// Release destroys the shader module.
func (m *ShaderModule) Release() { m.impl.Release() }
