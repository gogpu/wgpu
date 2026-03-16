package wgpu

import (
	"fmt"

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
	// currentPipelineBindGroupCount tracks the bind group count of the
	// currently set pipeline. Used by SetBindGroup to validate that the
	// group index is within the pipeline layout bounds. Zero means no
	// pipeline has been set yet.
	currentPipelineBindGroupCount uint32
	// pipelineSet tracks whether SetPipeline has been called.
	// Dispatch commands require a pipeline to be set first.
	pipelineSet bool
	// binder tracks bind group assignments and validates compatibility
	// at dispatch time, matching Rust wgpu-core's Binder pattern.
	binder binder
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	if pipeline == nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.SetPipeline: pipeline is nil"))
		return
	}
	p.currentPipelineBindGroupCount = pipeline.bindGroupCount
	p.pipelineSet = true
	p.binder.updateExpectations(pipeline.bindGroupLayouts)
	raw := p.core.RawPass()
	if raw != nil && pipeline.hal != nil {
		raw.SetPipeline(pipeline.hal)
	}
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.SetBindGroup: bind group is nil"))
		return
	}
	// Hard cap: WebGPU allows at most MaxBindGroups (8) bind group slots.
	if index >= MaxBindGroups {
		p.encoder.setError(fmt.Errorf(
			"wgpu: ComputePass.SetBindGroup: index %d >= MaxBindGroups (%d)",
			index, MaxBindGroups,
		))
		return
	}
	// Validate that the group index is within the current pipeline's layout.
	if p.currentPipelineBindGroupCount > 0 && index >= p.currentPipelineBindGroupCount {
		p.encoder.setError(fmt.Errorf(
			"wgpu: ComputePass.SetBindGroup: group index %d exceeds pipeline layout bind group count %d",
			index, p.currentPipelineBindGroupCount,
		))
		return
	}
	p.binder.assign(index, group.layout)
	raw := p.core.RawPass()
	if raw != nil && group.hal != nil {
		raw.SetBindGroup(index, group.hal, offsets)
	}
}

// validateDispatchState checks that a pipeline has been set and all bind groups
// are compatible before a dispatch call.
// Returns true if validation passes, false if an error was recorded.
func (p *ComputePassEncoder) validateDispatchState(method string) bool {
	if !p.pipelineSet {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.%s: no pipeline set (call SetPipeline first)", method))
		return false
	}
	if err := p.binder.checkCompatibility(); err != nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.%s: %w", method, err))
		return false
	}
	return true
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if !p.validateDispatchState("Dispatch") {
		return
	}
	p.core.Dispatch(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	if !p.validateDispatchState("DispatchIndirect") {
		return
	}
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.DispatchIndirect: buffer is nil"))
		return
	}
	p.core.DispatchIndirect(buffer.coreBuffer(), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	return p.core.End()
}
