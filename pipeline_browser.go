//go:build js && wasm

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
// The implementation lives in internal/pipeline (ADR-070); the methods are
// delegations and GetBindGroupLayout wraps the browser's layout object in a
// fresh public wrapper per call, as the pre-move type did.
type RenderPipeline struct {
	impl *pipeline.RenderPipeline
}

// GetBindGroupLayout returns the bind group layout at the given index.
// This wraps GPURenderPipeline.getBindGroupLayout(index) for "auto" layouts.
// A zero-value RenderPipeline has no implementation and returns nil.
func (p *RenderPipeline) GetBindGroupLayout(index uint32) *BindGroupLayout {
	if p.impl == nil {
		return nil
	}
	bl := p.impl.GetBindGroupLayout(index)
	if bl == nil {
		return nil
	}
	return &BindGroupLayout{impl: bl}
}

// Release destroys the render pipeline.
func (p *RenderPipeline) Release() {
	if p.impl == nil {
		return
	}
	p.impl.Release()
}

// ComputePipeline represents a configured compute pipeline.
//
// The implementation lives in internal/pipeline (ADR-070); the methods are
// delegations and GetBindGroupLayout wraps the browser's layout object in a
// fresh public wrapper per call, as the pre-move type did.
type ComputePipeline struct {
	impl *pipeline.ComputePipeline
}

// GetBindGroupLayout returns the bind group layout at the given index.
// This wraps GPUComputePipeline.getBindGroupLayout(index) for "auto" layouts.
// A zero-value ComputePipeline has no implementation and returns nil.
func (p *ComputePipeline) GetBindGroupLayout(index uint32) *BindGroupLayout {
	if p.impl == nil {
		return nil
	}
	bl := p.impl.GetBindGroupLayout(index)
	if bl == nil {
		return nil
	}
	return &BindGroupLayout{impl: bl}
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.impl == nil {
		return
	}
	p.impl.Release()
}
