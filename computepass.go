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
	// trackedRefs accumulates Clone'd ResourceRefs for resources used in
	// this compute pass. Transferred to the parent CommandEncoder on End().
	// Phase 2: per-command-buffer resource tracking.
	trackedRefs []*core.ResourceRef
}

// trackRef Clone()'s a ResourceRef and accumulates it for later transfer
// to the parent CommandEncoder. This keeps the resource alive until the
// GPU completes the submission containing this compute pass.
func (p *ComputePassEncoder) trackRef(ref *core.ResourceRef) {
	if ref != nil {
		ref.Clone()
		p.trackedRefs = append(p.trackedRefs, ref)
	}
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
	p.trackRef(pipeline.ref)
	raw := p.core.RawPass()
	if raw != nil && pipeline.hal != nil {
		raw.SetPipeline(pipeline.hal)
	}
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if err := validateSetBindGroup("ComputePass", index, group, offsets, p.currentPipelineBindGroupCount); err != nil {
		p.encoder.setError(err)
		return
	}
	p.binder.assign(index, group.layout)
	p.trackRef(group.ref)
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
	p.trackRef(buffer.core.Ref)
	p.core.DispatchIndirect(buffer.coreBuffer(), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	// Transfer tracked refs to parent CommandEncoder before ending.
	if len(p.trackedRefs) > 0 {
		p.encoder.trackedRefs = append(p.encoder.trackedRefs, p.trackedRefs...)
		p.trackedRefs = nil
	}
	return p.core.End()
}
