//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// BindGroupLayout defines the structure of resource bindings for shaders.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation. A zero-value BindGroupLayout has no implementation: Release
// panics on it, as the pre-move type did (a nil device view).
type BindGroupLayout struct {
	impl *resource.BindGroupLayout
}

// Release destroys the bind group layout.
func (l *BindGroupLayout) Release() { l.impl.Release() }

// PipelineLayout defines the bind group layout arrangement for a pipeline.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation. A zero-value PipelineLayout has no implementation: Release
// panics on it, as the pre-move type did.
type PipelineLayout struct {
	impl *resource.PipelineLayout
}

// Release drops the application's reference to the pipeline layout.
func (l *PipelineLayout) Release() { l.impl.Release() }

// BindGroup represents bound GPU resources for shader access.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation. A zero-value BindGroup has no implementation and Release is a
// no-op, as it was before the move.
type BindGroup struct {
	impl *resource.BindGroup
}

// Release marks the bind group for destruction.
func (g *BindGroup) Release() {
	if g.impl == nil {
		return
	}
	g.impl.Release()
}
