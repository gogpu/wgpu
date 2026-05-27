//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// LateSizedBufferGroup holds the shader-required minimum buffer sizes for
// bind group entries whose layout specifies MinBindingSize == 0.
type LateSizedBufferGroup struct {
	ShaderSizes []uint64
}

// RenderPipeline represents a configured render pipeline.
// On Rust backend, this wraps go-webgpu/webgpu RenderPipeline.
type RenderPipeline struct {
	r        *rwgpu.RenderPipeline
	device   *Device
	released bool
}

// Release destroys the render pipeline.
func (p *RenderPipeline) Release() {
	if p.released {
		return
	}
	p.released = true
	if p.r != nil {
		p.r.Release()
	}
}

// ComputePipeline represents a configured compute pipeline.
// On Rust backend, this wraps go-webgpu/webgpu ComputePipeline.
type ComputePipeline struct {
	r        *rwgpu.ComputePipeline
	device   *Device
	released bool
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true
	if p.r != nil {
		p.r.Release()
	}
}
