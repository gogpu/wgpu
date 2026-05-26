//go:build !(js && wasm)

package wgpu

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
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
	// currentPipelineBindGroupCount tracks the bind group count of the
	// currently set pipeline. Used by SetBindGroup to validate that the
	// group index is within the pipeline layout bounds. Zero means no
	// pipeline has been set yet.
	currentPipelineBindGroupCount uint32
	// pipelineSet tracks whether SetPipeline has been called.
	// Dispatch commands require a pipeline to be set first.
	pipelineSet bool
	// currentPipeline tracks the currently set pipeline for state restoration
	// after indirect dispatch validation. Set by SetPipeline.
	currentPipeline *ComputePipeline
	// assignedBindGroups tracks the bind groups set at each slot for state
	// restoration after indirect dispatch validation.
	assignedBindGroups [MaxBindGroups]*BindGroup
	// assignedDynOffsets tracks the dynamic offsets for each bind group slot
	// for state restoration after indirect dispatch validation.
	assignedDynOffsets [MaxBindGroups][]uint32
	// binder tracks bind group assignments and validates compatibility
	// at dispatch time, matching Rust wgpu-core's Binder pattern.
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
	p.currentPipelineBindGroupCount = pipeline.bindGroupCount
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

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if err := validateSetBindGroup("ComputePass", index, group, offsets, p.currentPipelineBindGroupCount); err != nil {
		p.encoder.setError(err)
		return
	}
	p.binder.assign(index, group.layout)
	p.binder.assignBindGroup(index, group)
	p.assignedBindGroups[index] = group
	p.assignedDynOffsets[index] = offsets
	p.trackRef(group.ref)
	// Clone individual buffer ResourceRefs to keep them alive until GPU completes.
	// Matches Rust wgpu merge_bind_group → ResourceMetadata.insert(Arc<Buffer>).
	for _, buf := range group.boundBuffers {
		if buf.core != nil && buf.core.Ref != nil {
			p.trackRef(buf.core.Ref)
		}
	}
	// Track bind group itself for submit-time validation (VAL-B5).
	p.encoder.trackBindGroup(group)
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
	if iv != nil && raw != nil {
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
// Flow:
//  1. Write params (max_workgroups, src_offset) to uniform buffer
//  2. Set validation pipeline + bind groups
//  3. Transition src buffer for storage read, dst buffer for storage write
//  4. Dispatch(1,1,1) -- validation shader
//  5. Transition dst buffer back to INDIRECT usage
//  6. Restore user pipeline + bind groups
//  7. DispatchIndirect from validated dst buffer at offset 0
func (p *ComputePassEncoder) dispatchIndirectValidated(
	buffer *Buffer,
	offset uint64,
	iv *core.IndirectValidation,
	raw hal.ComputePassEncoder,
) {
	halDevice := p.encoder.device.halDevice()
	if halDevice == nil {
		p.core.DispatchIndirect(buffer.coreBuffer(), offset)
		return
	}

	// Create source bind group for the user's indirect buffer.
	halSrcBuf := buffer.halBuffer()
	if halSrcBuf == nil {
		p.core.DispatchIndirect(buffer.coreBuffer(), offset)
		return
	}

	srcBindGroup, err := iv.CreateSrcBindGroup(halDevice, halSrcBuf, buffer.Size())
	if err != nil {
		// Validation resource creation failed -- fall back to unvalidated dispatch.
		p.core.DispatchIndirect(buffer.coreBuffer(), offset)
		return
	}
	// Schedule cleanup of the ephemeral src bind group.
	defer halDevice.DestroyBindGroup(srcBindGroup)

	// Write params to the uniform buffer. The params struct is:
	//   max_workgroups: u32 (baked into shader, but also in uniform for flexibility)
	//   src_offset:     u32 (byte offset / 4, since shader indexes u32 array)
	srcOffset := uint32(offset / 4) //nolint:gosec // offset is validated to be < buffer.Size()
	var paramsData [8]byte
	binary.LittleEndian.PutUint32(paramsData[0:4], iv.MaxWorkgroups())
	binary.LittleEndian.PutUint32(paramsData[4:8], srcOffset)

	// Write params via the HAL queue's WriteBuffer. This is a CPU-side write
	// that will be visible to the GPU before the next dispatch.
	if p.encoder.device.queue != nil && p.encoder.device.queue.hal != nil {
		if writeErr := p.encoder.device.queue.hal.WriteBuffer(iv.ParamsBuffer(), 0, paramsData[:]); writeErr != nil {
			// Params write failed -- fall back to unvalidated dispatch.
			p.core.DispatchIndirect(buffer.coreBuffer(), offset)
			return
		}
	} else {
		p.core.DispatchIndirect(buffer.coreBuffer(), offset)
		return
	}

	// Step 1: Set validation compute pipeline.
	raw.SetPipeline(iv.Pipeline())

	// Step 2: Bind destination group (group 0: dst buffer + params uniform).
	raw.SetBindGroup(0, iv.DstBindGroup(), nil)

	// Step 3: Bind source group (group 1: user's indirect buffer).
	raw.SetBindGroup(1, srcBindGroup, nil)

	// Step 4: Transition dst buffer from INDIRECT to STORAGE_READ_WRITE.
	// The compute pass encoder needs the parent command encoder for barriers.
	parentEncoder := p.encoder.core.RawEncoder()
	if parentEncoder != nil {
		// End the current compute pass temporarily to insert barriers.
		raw.End()

		parentEncoder.TransitionBuffers([]hal.BufferBarrier{
			{
				Buffer: iv.DstBuffer(),
				Usage: hal.BufferUsageTransition{
					OldUsage: gputypes.BufferUsageIndirect,
					NewUsage: gputypes.BufferUsageStorage,
				},
			},
		})

		// Re-begin compute pass for the validation dispatch.
		validationPass := parentEncoder.BeginComputePass(&hal.ComputePassDescriptor{
			Label: "indirect_dispatch_validation",
		})

		// Set validation pipeline and bind groups in the new pass.
		validationPass.SetPipeline(iv.Pipeline())
		validationPass.SetBindGroup(0, iv.DstBindGroup(), nil)
		validationPass.SetBindGroup(1, srcBindGroup, nil)

		// Step 5: Dispatch validation shader (single workgroup).
		validationPass.Dispatch(1, 1, 1)
		validationPass.End()

		// Step 6: Transition dst buffer back from STORAGE to INDIRECT.
		parentEncoder.TransitionBuffers([]hal.BufferBarrier{
			{
				Buffer: iv.DstBuffer(),
				Usage: hal.BufferUsageTransition{
					OldUsage: gputypes.BufferUsageStorage,
					NewUsage: gputypes.BufferUsageIndirect,
				},
			},
		})

		// Step 7: Re-begin compute pass for the user's actual dispatch.
		userPass := parentEncoder.BeginComputePass(&hal.ComputePassDescriptor{
			Label: "indirect_dispatch_user",
		})

		// Restore user pipeline and bind groups.
		p.restoreComputeState(userPass)

		// Step 8: DispatchIndirect from validated buffer at offset 0.
		userPass.DispatchIndirect(iv.DstBuffer(), 0)

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

	// Restore user's bind groups.
	for i := uint32(0); i < p.currentPipelineBindGroupCount; i++ {
		bg := p.assignedBindGroups[i]
		if bg != nil && bg.hal != nil {
			raw.SetBindGroup(i, bg.hal, p.assignedDynOffsets[i])
		}
	}
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	return p.core.End()
}
