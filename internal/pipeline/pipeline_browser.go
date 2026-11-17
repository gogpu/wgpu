//go:build js && wasm

package pipeline

import (
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/resource"
)

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

// NewRenderPipeline builds a render pipeline over a browser object.
func NewRenderPipeline(b *browser.RenderPipeline) *RenderPipeline {
	return &RenderPipeline{browser: b}
}

// Browser returns the browser render pipeline object, or nil.
func (p *RenderPipeline) Browser() *browser.RenderPipeline {
	if p == nil {
		return nil
	}
	return p.browser
}

// IsReleased reports whether Release has been called.
func (p *RenderPipeline) IsReleased() bool {
	return p != nil && p.released
}

// GetBindGroupLayout returns the bind group layout at the given index.
// This wraps GPURenderPipeline.getBindGroupLayout(index) for "auto" layouts.
func (p *RenderPipeline) GetBindGroupLayout(index uint32) *resource.BindGroupLayout {
	if p.browser == nil {
		return nil
	}
	return resource.NewBindGroupLayout(p.browser.GetBindGroupLayout(index))
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

// NewComputePipeline builds a compute pipeline over a browser object.
func NewComputePipeline(b *browser.ComputePipeline) *ComputePipeline {
	return &ComputePipeline{browser: b}
}

// Browser returns the browser compute pipeline object, or nil.
func (p *ComputePipeline) Browser() *browser.ComputePipeline {
	if p == nil {
		return nil
	}
	return p.browser
}

// IsReleased reports whether Release has been called.
func (p *ComputePipeline) IsReleased() bool {
	return p != nil && p.released
}

// GetBindGroupLayout returns the bind group layout at the given index.
// This wraps GPUComputePipeline.getBindGroupLayout(index) for "auto" layouts.
func (p *ComputePipeline) GetBindGroupLayout(index uint32) *resource.BindGroupLayout {
	if p.browser == nil {
		return nil
	}
	return resource.NewBindGroupLayout(p.browser.GetBindGroupLayout(index))
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true
}
