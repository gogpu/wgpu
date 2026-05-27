//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// ComputePassEncoder records compute dispatch commands.
// On Rust backend, this wraps go-webgpu/webgpu ComputePassEncoder.
type ComputePassEncoder struct {
	r        *rwgpu.ComputePassEncoder
	released bool
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	if pipeline == nil || pipeline.r == nil {
		return
	}
	p.r.SetPipeline(pipeline.r)
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil || group.r == nil {
		return
	}
	p.r.SetBindGroup(index, group.r, offsets)
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	// go-webgpu uses DispatchWorkgroups instead of Dispatch.
	p.r.DispatchWorkgroups(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	if buffer == nil || buffer.r == nil {
		return
	}
	p.r.DispatchWorkgroupsIndirect(buffer.r, offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	if p.released {
		return ErrReleased
	}
	p.released = true
	p.r.End()
	return nil
}
