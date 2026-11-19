//go:build rust

package pipeline

import (
	"sync/atomic"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// LateSizedBufferGroup holds the shader-required minimum buffer sizes for
// bind group entries whose layout specifies MinBindingSize == 0.
type LateSizedBufferGroup struct {
	ShaderSizes []uint64
}

// RenderPipeline represents a configured render pipeline.
// On Rust backend, this wraps go-webgpu/webgpu RenderPipeline.
type RenderPipeline struct {
	r *rwgpu.RenderPipeline
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewRenderPipeline builds a render pipeline over a go-webgpu object.
func NewRenderPipeline(r *rwgpu.RenderPipeline) *RenderPipeline {
	return &RenderPipeline{r: r, released: new(atomic.Bool)}
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
	return p != nil && p.released != nil && p.released.Load()
}

// Release destroys the render pipeline.
func (p *RenderPipeline) Release() {
	if p.released == nil || !p.released.CompareAndSwap(false, true) {
		return
	}
	if p.r != nil {
		p.r.Release()
	}
}

// ComputePipeline represents a configured compute pipeline.
// On Rust backend, this wraps go-webgpu/webgpu ComputePipeline.
type ComputePipeline struct {
	r *rwgpu.ComputePipeline
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewComputePipeline builds a compute pipeline over a go-webgpu object.
func NewComputePipeline(r *rwgpu.ComputePipeline) *ComputePipeline {
	return &ComputePipeline{r: r, released: new(atomic.Bool)}
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
	return p != nil && p.released != nil && p.released.Load()
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.released == nil || !p.released.CompareAndSwap(false, true) {
		return
	}
	if p.r != nil {
		p.r.Release()
	}
}
