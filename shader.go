//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// ShaderModule represents a compiled shader module.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation. A zero-value ShaderModule has no implementation: Release panics
// on it, as the pre-move type did (a nil device view); the zero-value tests
// pin that contract.
type ShaderModule struct {
	impl *resource.ShaderModule
}

// Release destroys the shader module.
func (m *ShaderModule) Release() { m.impl.Release() }
