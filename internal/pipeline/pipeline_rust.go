//go:build rust

package pipeline

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
	released bool
}

// NewRenderPipeline builds a render pipeline over a go-webgpu object.
func NewRenderPipeline(r *rwgpu.RenderPipeline) *RenderPipeline {
	return &RenderPipeline{r: r}
}

// R returns the go-webgpu render pipeline object, or nil.
func (p *RenderPipeline) R() *rwgpu.RenderPipeline {
	if p == nil {
		return nil
	}
	return p.r
}

// IsReleased reports whether Release has been called.
func (p *RenderPipeline) IsReleased() bool {
	return p != nil && p.released
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
	released bool
}

// NewComputePipeline builds a compute pipeline over a go-webgpu object.
func NewComputePipeline(r *rwgpu.ComputePipeline) *ComputePipeline {
	return &ComputePipeline{r: r}
}

// R returns the go-webgpu compute pipeline object, or nil.
func (p *ComputePipeline) R() *rwgpu.ComputePipeline {
	if p == nil {
		return nil
	}
	return p.r
}

// IsReleased reports whether Release has been called.
func (p *ComputePipeline) IsReleased() bool {
	return p != nil && p.released
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
