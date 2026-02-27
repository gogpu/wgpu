package wgpu

import "github.com/gogpu/wgpu/hal"

// RenderPipeline represents a configured render pipeline.
type RenderPipeline struct {
	hal      hal.RenderPipeline
	device   *Device
	released bool
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
