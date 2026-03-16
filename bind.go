package wgpu

import "github.com/gogpu/wgpu/hal"

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	hal      hal.BindGroupLayout
	device   *Device
	released bool
}

// Release destroys the bind group layout.
func (l *BindGroupLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	halDevice := l.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyBindGroupLayout(l.hal)
	}
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
type PipelineLayout struct {
	hal      hal.PipelineLayout
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this layout.
	// Used for validation in SetBindGroup.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts used to create this pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
}

// Release destroys the pipeline layout.
func (l *PipelineLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	halDevice := l.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyPipelineLayout(l.hal)
	}
}

// BindGroup represents bound GPU resources for shader access.
type BindGroup struct {
	hal      hal.BindGroup
	device   *Device
	released bool
	// layout is the bind group layout used to create this bind group.
	// Stored for draw-time compatibility validation via the binder.
	layout *BindGroupLayout
}

// Release destroys the bind group.
func (g *BindGroup) Release() {
	if g.released {
		return
	}
	g.released = true
	halDevice := g.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyBindGroup(g.hal)
	}
}
