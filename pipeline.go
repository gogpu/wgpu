package wgpu

import "github.com/gogpu/wgpu/hal"

// RenderPipeline represents a configured render pipeline.
type RenderPipeline struct {
	hal      hal.RenderPipeline
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this pipeline's
	// layout. Used by RenderPassEncoder.SetBindGroup to validate that
	// the group index is within bounds before issuing the HAL call.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts from the pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
	// requiredVertexBuffers is the number of vertex buffer layouts declared
	// in the pipeline's vertex state. Draw calls validate that at least this
	// many vertex buffers have been set via SetVertexBuffer.
	requiredVertexBuffers uint32
}

// Release destroys the render pipeline.
func (p *RenderPipeline) Release() {
	if p.released {
		return
	}
	p.released = true
	halDevice := p.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyRenderPipeline(p.hal)
	}
}

// ComputePipeline represents a configured compute pipeline.
type ComputePipeline struct {
	hal      hal.ComputePipeline
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this pipeline's
	// layout. Used by ComputePassEncoder.SetBindGroup to validate that
	// the group index is within bounds before issuing the HAL call.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts from the pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
}

// Release destroys the compute pipeline.
func (p *ComputePipeline) Release() {
	if p.released {
		return
	}
	p.released = true
	halDevice := p.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyComputePipeline(p.hal)
	}
}
