//go:build js && wasm

package wgpu

// LateSizedBufferGroup holds the shader-required minimum buffer sizes for
// bind group entries whose layout specifies MinBindingSize == 0.
type LateSizedBufferGroup struct {
	ShaderSizes []uint64
}

// RenderPipeline represents a configured render pipeline.
type RenderPipeline struct {
	released bool
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
	released bool
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true
}
