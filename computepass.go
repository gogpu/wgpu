package wgpu

import (
	"github.com/gogpu/wgpu/core"
)

// ComputePassEncoder records compute dispatch commands.
//
// Created by CommandEncoder.BeginComputePass().
// Must be ended with End() before the CommandEncoder can be finished.
//
// NOT thread-safe.
type ComputePassEncoder struct {
	core    *core.CoreComputePassEncoder
	encoder *CommandEncoder
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	if pipeline == nil {
		return
	}
	// Core's SetPipeline tracks pipeline state internally.
	// Full HAL integration pending: requires core.ComputePipeline with HAL handle.
	p.core.SetPipeline(nil)
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil {
		return
	}
	// Direct HAL delegate pending: core doesn't have HAL bind group integration yet.
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	p.core.Dispatch(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	if buffer == nil {
		return
	}
	p.core.DispatchIndirect(buffer.coreBuffer(), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	return p.core.End()
}
