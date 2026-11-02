//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
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
	// pipelineSet tracks whether SetPipeline has been called.
	// Dispatch commands require a pipeline to be set first.
	pipelineSet bool
	// currentPipeline tracks the currently set pipeline for state restoration
	// after indirect dispatch validation. Set by SetPipeline.
	currentPipeline *ComputePipeline
	// binder holds the bind groups set on the pass with their dynamic
	// offsets, validates them at dispatch time and binds them to the backend
	// before each dispatch, matching Rust wgpu-core's Binder.
	binder binder
}

// trackRef Clone()'s a ResourceRef and appends directly to the parent
// CommandEncoder's trackedRefs. Eliminates intermediate pass-level slice
// and the copy at End(). Matches Rust where trackers live in the command
// encoder throughout recording — no per-pass intermediate storage.
func (p *ComputePassEncoder) trackRef(ref *core.ResourceRef) {
	if ref != nil {
		ref.Clone()
		p.encoder.trackedRefs = append(p.encoder.trackedRefs, ref)
	}
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	if pipeline == nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.SetPipeline: pipeline is nil"))
		return
	}
	p.pipelineSet = true
	p.currentPipeline = pipeline
	p.binder.updateExpectations(pipeline.bindGroupLayouts)
	p.binder.updateLateBufferBindingsFromPipeline(pipeline.lateSizedBufferGroups)
	p.trackRef(pipeline.ref)
	raw := p.core.RawPass()
	if raw != nil && pipeline.hal != nil {
		raw.SetPipeline(pipeline.hal)
	}
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline, or name an index the current pipeline does not use: the group
// is bound to the backend at the next dispatch whose pipeline layout uses it.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	limits := &p.encoder.device.core.Limits
	if err := validateSetBindGroup("ComputePass", index, group, offsets, limits); err != nil {
		p.encoder.setError(err)
		return
	}
	p.binder.setGroup(index, group, offsets)
	p.trackRef(group.ref)
	// Clone individual buffer ResourceRefs to keep them alive until GPU completes.
	// Matches Rust wgpu merge_bind_group → ResourceMetadata.insert(Arc<Buffer>).
	for _, buf := range group.boundBuffers {
		if buf.core != nil && buf.core.Ref != nil {
			p.trackRef(buf.core.Ref)
		}
	}
	// Track bind group itself for submit-time validation (VAL-B5 and, via
	// group.boundBuffers/boundTextures, VAL-A6). The group's own slices are
	// walked at Submit, so there is nothing to fan out here.
	p.encoder.trackBindGroup(group)
}

// flushBindGroups binds the bind groups whose backend binding is stale, right
// before a dispatch that passed validation (Rust wgpu-core compute.rs
// State::flush_bindings).
func (p *ComputePassEncoder) flushBindGroups() {
	if raw := p.core.RawPass(); raw != nil {
		p.binder.flush(raw)
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

	if !p.flushDispatch("Dispatch", nil, track.BufferUsesNone) {
		return
	}
	p.flushBindGroups()
	p.core.Dispatch(x, y, z)
}

// flushDispatch builds the usage scope of the next dispatch from the bind
// groups the current pipeline uses and the indirect buffer (if any), and
// records the barriers into it. Returns false, with the error recorded on the
// encoder, if two usages in the dispatch conflict.
//
// Matches Rust wgpu-core compute.rs State::flush_bindings, which merges only
// the bind groups the pipeline uses (binder.list_active).
func (p *ComputePassEncoder) flushDispatch(method string, indirect *Buffer, indirectUsage track.BufferUses) bool {
	var groups [MaxBindGroups]*core.BindGroupUsages
	active := groups[:0]
	for i := uint32(0); i < MaxBindGroups; i++ {
		if bg := p.binder.activeGroup(i); bg != nil {
			active = append(active, bg.usages)
		}
	}
	var indirectCore *core.Buffer
	if indirect != nil {
		indirectCore = indirect.core
	}
	if err := p.core.FlushDispatch(active, indirectCore, indirectUsage); err != nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.%s: %w", method, err))
		return false
	}
	return true
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
//
// When indirect dispatch validation is available (created at device init),
// a pre-dispatch validation shader checks workgroup counts against
// maxComputeWorkgroupsPerDimension and clamps to (0,0,0) if exceeded.
// This prevents GPU hangs/TDR from invalid indirect buffers.
//
// Matches Rust wgpu-core compute.rs:878-1074 (dispatch_indirect).
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	if !p.validateDispatchState("DispatchIndirect") {
		return
	}
	if buffer == nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.DispatchIndirect: buffer is nil"))
		return
	}
	// VAL-B3: Validate indirect buffer has INDIRECT usage.
	// Matches Rust wgpu-core compute.rs:896 (check_usage(BufferUsages::INDIRECT)).
	if buffer.Usage()&BufferUsageIndirect == 0 {
		p.encoder.setError(fmt.Errorf(
			"wgpu: ComputePass.DispatchIndirect: buffer %q missing BufferUsageIndirect usage: %w",
			buffer.Label(), ErrDispatchIndirectBufferUsage))
		return
	}
	// VAL-B3: Validate indirect buffer offset is 4-byte aligned.
	// Matches Rust wgpu-core compute.rs:899 (offset % 4 != 0).
	if offset%4 != 0 {
		p.encoder.setError(fmt.Errorf(
			"wgpu: ComputePass.DispatchIndirect: offset %d is not 4-byte aligned: %w",
			offset, ErrDispatchIndirectOffsetAlignment))
		return
	}
	// VAL-B3: Validate indirect args fit within buffer.
	// DispatchIndirect args: 3 x uint32 = 12 bytes. Matches Rust compute.rs:903-909.
	if offset+12 > buffer.Size() {
		p.encoder.setError(fmt.Errorf(
			"wgpu: ComputePass.DispatchIndirect: offset %d + 12 bytes exceeds buffer size %d: %w",
			offset, buffer.Size(), ErrDispatchIndirectBufferOverrun))
		return
	}
	p.trackRef(buffer.core.Ref)
	p.encoder.trackBuffer(buffer)

	// FEAT-COMPUTE-004: GPU-side indirect dispatch validation.
	// If the device has an IndirectValidation pipeline, run a pre-dispatch
	// validation shader that checks workgroup counts and either copies valid
	// values or zeroes them out. The actual DispatchIndirect then reads from
	// the validated destination buffer instead of the user's original buffer.
	//
	// Matches Rust wgpu-core compute.rs:921-1059.
	iv := p.encoder.device.core.IndirectValidation()
	raw := p.core.RawPass()
	validated := iv != nil && raw != nil

	// With validation, a validation shader reads the arguments as storage
	// before the dispatch reads them from its own buffer.
	indirectUsage := track.BufferUsesIndirect
	if validated {
		indirectUsage = track.BufferUsesStorageRead
	}
	if !p.flushDispatch("DispatchIndirect", buffer, indirectUsage) {
		return
	}
	p.flushBindGroups()

	if validated {
		p.dispatchIndirectValidated(buffer, offset, iv, raw)
		return
	}

	// Fallback: no validation available, dispatch directly.
	p.core.DispatchIndirect(buffer.coreBuffer(), offset)
}

// dispatchIndirectValidated runs the GPU-side validation shader before the
// actual indirect dispatch. It saves/restores the user's compute pipeline
// and bind group state around the validation, matching Rust wgpu-core's
// approach in compute.rs:928-1059.
//
// Every input of the validation dispatch comes from bind groups built before
// recording (core.IndirectValidation.DispatchParams): the commands of every
// DispatchIndirect, in any pass or encoder, read their own arguments, with no
// buffer write between recording and execution.
//
// Flow:
//  1. End the user's pass; transition the dst buffer to storage
//  2. Validation pass: pipeline, bind groups, Dispatch(1,1,1)
//  3. Transition the dst buffer back to INDIRECT
//  4. New pass: restore user pipeline + bind groups,
//     DispatchIndirect from validated dst buffer at offset 0
func (p *ComputePassEncoder) dispatchIndirectValidated(
	buffer *Buffer,
	offset uint64,
	iv *core.IndirectValidation,
	raw hal.ComputePassEncoder,
) {
	params, err := iv.DispatchParams(buffer.coreBuffer(), offset)
	if err != nil {
		p.encoder.setError(fmt.Errorf("wgpu: ComputePass.DispatchIndirect: indirect validation: %w", err))
		return
	}

	// The HAL compute pass records no barriers, so the validation runs in
	// passes of its own between the barriers on the parent encoder.
	parentEncoder := p.encoder.core.RawEncoder()
	if parentEncoder != nil {
		// End the current compute pass temporarily to insert barriers.
		raw.End()

		parentEncoder.TransitionBuffers([]hal.BufferBarrier{
			{
				Buffer: params.DstBuffer,
				Usage: hal.BufferUsageTransition{
					OldUsage: gputypes.BufferUsageIndirect,
					NewUsage: gputypes.BufferUsageStorage,
				},
			},
		})

		validationPass := parentEncoder.BeginComputePass(&hal.ComputePassDescriptor{
			Label: "indirect_dispatch_validation",
		})
		validationPass.SetPipeline(params.Pipeline)
		validationPass.SetBindGroup(0, params.DstBindGroup, nil)
		validationPass.SetBindGroup(1, params.SrcBindGroup, nil)
		validationPass.Dispatch(1, 1, 1)
		validationPass.End()

		parentEncoder.TransitionBuffers([]hal.BufferBarrier{
			{
				Buffer: params.DstBuffer,
				Usage: hal.BufferUsageTransition{
					OldUsage: gputypes.BufferUsageStorage,
					NewUsage: gputypes.BufferUsageIndirect,
				},
			},
		})

		userPass := parentEncoder.BeginComputePass(&hal.ComputePassDescriptor{
			Label: "indirect_dispatch_user",
		})
		p.restoreComputeState(userPass)
		userPass.DispatchIndirect(params.DstBuffer, 0)

		// Replace the core encoder's raw pass with the new one.
		// This is needed because End() + BeginComputePass created a new pass.
		p.core.ReplaceRawPass(userPass)
	} else {
		// No parent encoder access -- fall back to unvalidated dispatch.
		p.core.DispatchIndirect(buffer.coreBuffer(), offset)
	}
}

// restoreComputeState restores the user's pipeline and bind groups on a new
// HAL compute pass encoder after the validation pass. This is needed because
// the validation pass temporarily replaced the user's pipeline/bind groups.
// Matches Rust wgpu-core compute.rs:1001-1036 (reset state after validation).
func (p *ComputePassEncoder) restoreComputeState(raw hal.ComputePassEncoder) {
	// Restore user's compute pipeline.
	if p.pipelineSet && p.currentPipeline != nil && p.currentPipeline.hal != nil {
		raw.SetPipeline(p.currentPipeline.hal)
	}

	// Restore user's bind groups: the new backend pass has none.
	p.binder.bindValid(raw, 0)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	return p.core.End()
}
