package core

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
)

// ComputePassDescriptor describes how to create a compute pass.
type ComputePassDescriptor struct {
	// Label is an optional debug name for the compute pass.
	Label string

	// TimestampWrites are timestamp queries to write at pass boundaries (optional).
	TimestampWrites *ComputePassTimestampWrites
}

// ComputePassTimestampWrites describes timestamp query writes for a compute pass.
type ComputePassTimestampWrites struct {
	// QuerySet is the query set to write timestamps to.
	QuerySet QuerySetID

	// BeginningOfPassWriteIndex is the query index for pass start.
	// Use nil to skip.
	BeginningOfPassWriteIndex *uint32

	// EndOfPassWriteIndex is the query index for pass end.
	// Use nil to skip.
	EndOfPassWriteIndex *uint32
}

// ComputePassEncoder records compute commands within a compute pass.
// It wraps hal.ComputePassEncoder with validation and ID-based resource lookup.
type ComputePassEncoder struct {
	raw    hal.ComputePassEncoder
	device *Device
	ended  bool
}

// SetPipeline sets the active compute pipeline for subsequent dispatch calls.
// The pipeline must have been created on the same device as this encoder.
//
// Returns an error if the pipeline ID is invalid.
func (e *ComputePassEncoder) SetPipeline(pipeline ComputePipelineID) error {
	if e.ended {
		return fmt.Errorf("compute pass has already ended")
	}

	hub := GetGlobal().Hub()
	rawPipeline, err := hub.GetComputePipeline(pipeline)
	if err != nil {
		return fmt.Errorf("invalid compute pipeline: %w", err)
	}

	// TODO: When hal.ComputePipeline is available in the storage,
	// convert rawPipeline to hal.ComputePipeline and call e.raw.SetPipeline
	_ = rawPipeline
	// e.raw.SetPipeline(halPipeline)

	return nil
}

// SetBindGroup sets a bind group for the given index.
// The bind group provides resources (buffers, textures, samplers) to shaders.
//
// Parameters:
//   - index: The bind group index (0, 1, 2, or 3).
//   - group: The bind group ID to bind.
//   - offsets: Dynamic offsets for dynamic uniform/storage buffers (can be nil).
//
// Returns an error if the bind group ID is invalid or if the encoder has ended.
func (e *ComputePassEncoder) SetBindGroup(index uint32, group BindGroupID, offsets []uint32) error {
	if e.ended {
		return fmt.Errorf("compute pass has already ended")
	}

	// WebGPU spec: max 4 bind groups (0-3)
	if index > 3 {
		return fmt.Errorf("bind group index %d exceeds maximum (3)", index)
	}

	hub := GetGlobal().Hub()
	rawGroup, err := hub.GetBindGroup(group)
	if err != nil {
		return fmt.Errorf("invalid bind group: %w", err)
	}

	// TODO: When hal.BindGroup is available in the storage,
	// convert rawGroup to hal.BindGroup and call e.raw.SetBindGroup
	_ = rawGroup
	// e.raw.SetBindGroup(index, halGroup, offsets)

	return nil
}

// Dispatch dispatches compute work.
// This executes the compute shader with the specified number of workgroups.
//
// Parameters:
//   - x, y, z: The number of workgroups to dispatch in each dimension.
//
// Each workgroup runs the compute shader's workgroup_size threads.
// The total threads = x * y * z * workgroup_size.
//
// Note: This method does not return an error. Dispatch errors are deferred
// to command buffer submission time, matching the WebGPU error model.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if e.ended {
		// Record error for deferred validation
		return
	}

	if e.raw != nil {
		e.raw.Dispatch(x, y, z)
	}
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
// The dispatch parameters are read from the specified buffer.
//
// Parameters:
//   - buffer: The buffer containing DispatchIndirectArgs at the given offset.
//   - offset: The byte offset into the buffer (must be 4-byte aligned).
//
// The buffer must contain the following structure at the offset:
//
//	struct DispatchIndirectArgs {
//	    x: u32,     // Number of workgroups in X
//	    y: u32,     // Number of workgroups in Y
//	    z: u32,     // Number of workgroups in Z
//	}
//
// Returns an error if the buffer ID is invalid or the offset is not aligned.
func (e *ComputePassEncoder) DispatchIndirect(buffer BufferID, offset uint64) error {
	if e.ended {
		return fmt.Errorf("compute pass has already ended")
	}

	// Indirect dispatch requires 4-byte alignment
	if offset%4 != 0 {
		return fmt.Errorf("indirect dispatch offset must be 4-byte aligned, got %d", offset)
	}

	hub := GetGlobal().Hub()
	rawBuffer, err := hub.GetBuffer(buffer)
	if err != nil {
		return fmt.Errorf("invalid buffer: %w", err)
	}

	// TODO: When hal.Buffer is available in the storage,
	// convert rawBuffer to hal.Buffer and call e.raw.DispatchIndirect
	_ = rawBuffer
	// e.raw.DispatchIndirect(halBuffer, offset)

	return nil
}

// End finishes the compute pass.
// After this call, the encoder cannot be used again.
// Any subsequent method calls will return errors.
func (e *ComputePassEncoder) End() {
	if e.ended {
		return
	}

	e.ended = true

	if e.raw != nil {
		e.raw.End()
	}
}

// CommandEncoderState tracks the state of a command encoder.
type CommandEncoderState int

const (
	// CommandEncoderStateRecording means the encoder is actively recording commands.
	CommandEncoderStateRecording CommandEncoderState = iota

	// CommandEncoderStateEnded means the encoder has finished and produced a command buffer.
	CommandEncoderStateEnded

	// CommandEncoderStateError means the encoder encountered an error.
	CommandEncoderStateError
)

// CommandEncoderImpl provides command encoder functionality.
// It wraps hal.CommandEncoder with validation and ID-based resource lookup.
type CommandEncoderImpl struct {
	raw    hal.CommandEncoder
	device *Device
	state  CommandEncoderState
	label  string
}

// BeginComputePass begins a new compute pass within this command encoder.
// The returned ComputePassEncoder is used to record compute commands.
//
// Parameters:
//   - desc: Optional descriptor with label and timestamp writes.
//     Pass nil for default settings.
//
// The compute pass must be ended with End() before:
//   - Beginning another pass (compute or render)
//   - Finishing the command encoder
//
// Returns the compute pass encoder and any error encountered.
func (e *CommandEncoderImpl) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	if e.state != CommandEncoderStateRecording {
		return nil, fmt.Errorf("command encoder is not in recording state")
	}

	// Convert core descriptor to HAL descriptor
	halDesc := &hal.ComputePassDescriptor{}
	if desc != nil {
		halDesc.Label = desc.Label

		if desc.TimestampWrites != nil {
			// TODO: Look up the query set from the hub and convert to HAL type
			// For now, we skip timestamp writes until QuerySet is fully implemented
			halDesc.TimestampWrites = nil
		}
	}

	// Begin the compute pass on the underlying HAL encoder
	var rawPass hal.ComputePassEncoder
	if e.raw != nil {
		rawPass = e.raw.BeginComputePass(halDesc)
	}

	return &ComputePassEncoder{
		raw:    rawPass,
		device: e.device,
		ended:  false,
	}, nil
}

// DeviceCreateCommandEncoder creates a new command encoder for recording GPU commands.
// This is the entry point for recording command buffers.
//
// Parameters:
//   - id: The device ID to create the encoder on.
//   - label: Optional debug label for the encoder.
//
// Returns the command encoder ID and any error encountered.
func DeviceCreateCommandEncoder(id DeviceID, label string) (CommandEncoderID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return CommandEncoderID{}, fmt.Errorf("invalid device: %w", err)
	}

	// Create a placeholder command encoder
	// In a full implementation, this would create the HAL command encoder
	encoder := CommandEncoder{}
	encoderID := hub.RegisterCommandEncoder(encoder)

	return encoderID, nil
}

// CommandEncoderFinish finishes recording and returns a command buffer.
// The command encoder cannot be used after this call.
//
// Parameters:
//   - id: The command encoder ID to finish.
//
// Returns the command buffer ID and any error encountered.
func CommandEncoderFinish(id CommandEncoderID) (CommandBufferID, error) {
	hub := GetGlobal().Hub()

	// Verify the encoder exists
	_, err := hub.GetCommandEncoder(id)
	if err != nil {
		return CommandBufferID{}, fmt.Errorf("invalid command encoder: %w", err)
	}

	// TODO: Call EndEncoding on the HAL encoder and get the command buffer

	// Create a placeholder command buffer
	cmdBuffer := CommandBuffer{}
	cmdBufferID := hub.RegisterCommandBuffer(cmdBuffer)

	// Unregister the encoder (it's consumed)
	_, _ = hub.UnregisterCommandEncoder(id)

	return cmdBufferID, nil
}
