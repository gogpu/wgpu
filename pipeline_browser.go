//go:build js && wasm

package wgpu

import "github.com/gogpu/wgpu/internal/browser"

// LateSizedBufferGroup holds the shader-required minimum buffer sizes for
// bind group entries whose layout specifies MinBindingSize == 0.
type LateSizedBufferGroup struct {
	ShaderSizes []uint64
}

// RenderPipeline represents a configured render pipeline.
type RenderPipeline struct {
	browser  *browser.RenderPipeline
	released bool
}

// GetBindGroupLayout returns the bind group layout at the given index.
// This wraps GPURenderPipeline.getBindGroupLayout(index) for "auto" layouts.
func (p *RenderPipeline) GetBindGroupLayout(index uint32) *BindGroupLayout {
	if p.browser == nil {
		return nil
	}
	bl := p.browser.GetBindGroupLayout(index)
	return &BindGroupLayout{
		browser: bl,
	}
}

// Release destroys the render pipeline.
func (p *RenderPipeline) Release() {
	if p.released {
		return
	}
	p.released = true
}

// ComputePipeline represents a configured compute pipeline.
type ComputePipeline struct {
	browser  *browser.ComputePipeline
	released bool
}

// GetBindGroupLayout returns the bind group layout at the given index.
// This wraps GPUComputePipeline.getBindGroupLayout(index) for "auto" layouts.
func (p *ComputePipeline) GetBindGroupLayout(index uint32) *BindGroupLayout {
	if p.browser == nil {
		return nil
	}
	bl := p.browser.GetBindGroupLayout(index)
	return &BindGroupLayout{
		browser: bl,
	}
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true
}
