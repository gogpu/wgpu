//go:build !(js && wasm)

package wgpu

import (
	"errors"
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
	p.binder.updateLateBufferBindingsFromPipeline(pipeline.lateSizedBufferGroups)
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
	p.binder.assignBindGroup(index, group)
	p.trackRef(group.ref)
	// Track bind group resources for submit-time validation (VAL-A6).
	for _, buf := range group.boundBuffers {
		p.encoder.trackBuffer(buf)
	}
	for _, tex := range group.boundTextures {
		p.encoder.trackTexture(tex)
	}
	raw := p.core.RawPass()
	if raw != nil && group.hal != nil {
		raw.SetBindGroup(index, group.hal, offsets)
	}
}

// validateDispatchState checks that a pipeline has been set and all bind groups
// are compatible before a dispatch call.
// Returns true if validation passes, false if an error was recorded.
//
// Each validation failure wraps a typed sentinel error so that callers can
// use errors.Is() to identify the failure category programmatically.
// Matches Rust wgpu-core State::is_ready (command/compute.rs:278-284).
func (p *ComputePassEncoder) validateDispatchState(method string) bool {
	if !p.pipelineSet {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.%s: no pipeline set (call SetPipeline first): %w",
			method, ErrDispatchMissingPipeline))
		return false
	}
	if err := p.binder.checkCompatibility(); err != nil {
		sentinel := ErrDispatchMissingBindGroup
		if errors.Is(err, errBindGroupIncompatible) {
			sentinel = ErrDispatchIncompatibleBindGroup
		}
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.%s: %w: %w", method, sentinel, err))
		return false
	}
	// Late buffer binding size validation: check that bound buffers are large enough
	// for bindings with MinBindingSize == 0. Matches Rust wgpu-core's is_ready()
	// call to check_late_buffer_bindings before dispatch (compute.rs:278-285).
	if err := p.binder.checkLateBufferBindings(); err != nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.%s: %w: %w", method, ErrDispatchLateBufferTooSmall, err))
		return false
	}
	return true
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if !p.validateDispatchState("Dispatch") {
		return
	}

	// VAL-009: Validate workgroup counts against device limits.
	// Matches Rust wgpu-core compute.rs:853-870.
	// (0, 0, 0) is allowed as a no-op per spec.
	limit := p.encoder.device.core.Limits.MaxComputeWorkgroupsPerDimension
	if x > limit || y > limit || z > limit {
		p.encoder.setError(fmt.Errorf(
			"wgpu: ComputePass.Dispatch: workgroup count (%d, %d, %d) exceeds device limit %d: %w",
			x, y, z, limit, ErrDispatchWorkgroupCountExceeded))
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
	p.encoder.trackBuffer(buffer)
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
