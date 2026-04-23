//go:build js && wasm

package wgpu

// ComputePassEncoder records compute dispatch commands.
type ComputePassEncoder struct {
	released bool
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	panic("wgpu: browser backend not yet implemented")
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	panic("wgpu: browser backend not yet implemented")
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	panic("wgpu: browser backend not yet implemented")
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	panic("wgpu: browser backend not yet implemented")
}
