//go:build !rust && !(js && wasm)

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
//
// The implementation lives in internal/pipeline (ADR-070). This declaration is
// the public surface: every method is one delegation. A zero-value
// RenderPipeline has no implementation and Release is a no-op, as it was
// before the move; a nil *RenderPipeline panics on Release, as before.
type RenderPipeline struct {
	impl *pipeline.RenderPipeline
}

// Release drops the application's ownership reference to the render pipeline.
//
// If the pipeline is still referenced by in-flight GPU submissions, the HAL
// pipeline stays alive until the GPU completes and Triage drops all tracked
// refs (ADR-056: unified resource lifecycle).
func (p *RenderPipeline) Release() {
	if p.impl == nil {
		return
	}
	p.impl.Release()
}

// ComputePipeline represents a configured compute pipeline.
//
// The implementation lives in internal/pipeline (ADR-070); Release is one
// delegation. A zero-value ComputePipeline has no implementation and Release
// is a no-op, as it was before the move; a nil *ComputePipeline panics on
// Release, as before.
type ComputePipeline struct {
	impl *pipeline.ComputePipeline
}

// Release drops the application's ownership reference to the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.impl == nil {
		return
	}
	p.impl.Release()
}
