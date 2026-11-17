//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/pipeline"

// LateSizedBufferGroup holds the shader-required minimum buffer sizes for
// bind group entries whose layout specifies MinBindingSize == 0.
//
// The groups the pipelines carry live in internal/pipeline (ADR-070); this
// declaration is the public shape.
type LateSizedBufferGroup struct {
	ShaderSizes []uint64
}

// RenderPipeline represents a configured render pipeline.
// On Rust backend, the implementation in internal/pipeline (ADR-070) wraps a
// go-webgpu/webgpu render pipeline; Release is one delegation.
type RenderPipeline struct {
	impl *pipeline.RenderPipeline
}

// Release destroys the render pipeline.
func (p *RenderPipeline) Release() {
	if p.impl == nil {
		return
	}
	p.impl.Release()
}

// ComputePipeline represents a configured compute pipeline.
// On Rust backend, the implementation in internal/pipeline (ADR-070) wraps a
// go-webgpu/webgpu compute pipeline; Release is one delegation.
type ComputePipeline struct {
	impl *pipeline.ComputePipeline
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.impl == nil {
		return
	}
	p.impl.Release()
}
