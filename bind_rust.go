//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// BindGroupLayout defines the structure of resource bindings for shaders.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation.
type BindGroupLayout struct {
	impl *resource.BindGroupLayout
}

// Release destroys the bind group layout. A zero-value wrapper panics, as the
// zero-value tests pin.
func (l *BindGroupLayout) Release() { l.impl.Release() }

// PipelineLayout defines the bind group layout arrangement for a pipeline.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation.
type PipelineLayout struct {
	impl *resource.PipelineLayout
}

// Release destroys the pipeline layout. A zero-value wrapper panics, as the
// zero-value tests pin.
func (l *PipelineLayout) Release() { l.impl.Release() }

// BindGroup represents bound GPU resources for shader access.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation.
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
