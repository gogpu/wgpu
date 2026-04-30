//go:build !(js && wasm)

package wgpu

import (
	"errors"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Sentinel errors re-exported from HAL.
var (
	ErrDeviceLost      = hal.ErrDeviceLost
	ErrOutOfMemory     = hal.ErrDeviceOutOfMemory
	ErrSurfaceLost     = hal.ErrSurfaceLost
	ErrSurfaceOutdated = hal.ErrSurfaceOutdated
	ErrTimeout         = hal.ErrTimeout
)

// Public API sentinel errors.
var (
	// ErrReleased is returned when operating on a released resource.
	ErrReleased = errors.New("wgpu: resource already released")

	// ErrNoAdapters is returned when no GPU adapters are found.
	ErrNoAdapters = errors.New("wgpu: no GPU adapters available")

	// ErrNoBackends is returned when no backends are registered.
	ErrNoBackends = errors.New("wgpu: no backends registered (import a backend package)")
)

// Draw-time validation sentinel errors.
// These are wrapped with additional context (e.g., method name) and surfaced
// as deferred errors on Finish(). Use errors.Is() to match programmatically.
//
// Matches Rust wgpu-core DrawError / DispatchError variants
// (command/render.rs:542-593, command/compute.rs:278-284).
var (
	// ErrDrawMissingPipeline is returned when Draw/DrawIndexed/DrawIndirect is
	// called before SetPipeline on a render pass encoder.
	ErrDrawMissingPipeline = errors.New("wgpu: draw called without SetPipeline")

	// ErrDrawMissingBindGroup is returned when a draw command is issued but not
	// all bind groups required by the current pipeline have been set.
	ErrDrawMissingBindGroup = errors.New("wgpu: draw called with missing bind group")

	// ErrDrawIncompatibleBindGroup is returned when a draw command is issued but
	// the bind group set at a slot has an incompatible layout with the pipeline.
	ErrDrawIncompatibleBindGroup = errors.New("wgpu: draw called with incompatible bind group layout")

	// ErrDrawMissingVertexBuffer is returned when a draw command is issued but
	// fewer vertex buffers have been set than the pipeline requires.
	ErrDrawMissingVertexBuffer = errors.New("wgpu: draw called with insufficient vertex buffers")

	// ErrDrawMissingIndexBuffer is returned when DrawIndexed or
	// DrawIndexedIndirect is called before SetIndexBuffer.
	ErrDrawMissingIndexBuffer = errors.New("wgpu: DrawIndexed called without SetIndexBuffer")

	// ErrDrawMissingBlendConstant is returned when a draw command is issued
	// but the pipeline uses BlendFactorConstant and SetBlendConstant was not called.
	ErrDrawMissingBlendConstant = errors.New("wgpu: draw called without SetBlendConstant (pipeline uses constant blend factor)")

	// ErrDrawLateBufferTooSmall is returned when a buffer bound via SetBindGroup
	// is smaller than the shader-required minimum for a binding whose layout
	// has MinBindingSize == 0.
	ErrDrawLateBufferTooSmall = errors.New("wgpu: bound buffer smaller than shader-required minimum")

	// ErrDispatchMissingPipeline is returned when Dispatch/DispatchIndirect is
	// called before SetPipeline on a compute pass encoder.
	ErrDispatchMissingPipeline = errors.New("wgpu: dispatch called without SetPipeline")

	// ErrDispatchMissingBindGroup is returned when a dispatch command is issued
	// but not all bind groups required by the current pipeline have been set.
	ErrDispatchMissingBindGroup = errors.New("wgpu: dispatch called with missing bind group")

	// ErrDispatchIncompatibleBindGroup is returned when a dispatch command is
	// issued but the bind group set at a slot has an incompatible layout.
	ErrDispatchIncompatibleBindGroup = errors.New("wgpu: dispatch called with incompatible bind group layout")

	// ErrDispatchLateBufferTooSmall is returned when a buffer bound via
	// SetBindGroup is smaller than the shader-required minimum for a dispatch.
	ErrDispatchLateBufferTooSmall = errors.New("wgpu: dispatch: bound buffer smaller than shader-required minimum")

	// ErrDispatchWorkgroupCountExceeded is returned when Dispatch is called with
	// workgroup counts exceeding the device limit.
	ErrDispatchWorkgroupCountExceeded = errors.New("wgpu: dispatch workgroup count exceeds device limit")

	// ErrDrawIndexFormatMismatch is returned when the index buffer format
	// does not match the pipeline's StripIndexFormat for strip topologies.
	// Matches Rust wgpu-core DrawError::UnmatchedIndexFormats (render.rs:576-580).
	ErrDrawIndexFormatMismatch = errors.New("wgpu: index buffer format does not match pipeline strip index format")

	// ErrDrawIndirectBufferUsage is returned when DrawIndirect or
	// DrawIndexedIndirect is called with a buffer that lacks BufferUsageIndirect.
	// Matches Rust wgpu-core check_usage(BufferUsages::INDIRECT) (render.rs:2763).
	ErrDrawIndirectBufferUsage = errors.New("wgpu: indirect draw buffer missing INDIRECT usage")

	// ErrDrawIndirectOffsetAlignment is returned when DrawIndirect or
	// DrawIndexedIndirect is called with an offset that is not 4-byte aligned.
	// Matches Rust wgpu-core UnalignedIndirectBufferOffset (render.rs:2766).
	ErrDrawIndirectOffsetAlignment = errors.New("wgpu: indirect draw buffer offset not 4-byte aligned")

	// ErrDispatchIndirectBufferUsage is returned when DispatchIndirect is called
	// with a buffer that lacks BufferUsageIndirect.
	// Matches Rust wgpu-core check_usage(BufferUsages::INDIRECT) (compute.rs:896).
	ErrDispatchIndirectBufferUsage = errors.New("wgpu: indirect dispatch buffer missing INDIRECT usage")

	// ErrDispatchIndirectOffsetAlignment is returned when DispatchIndirect is
	// called with an offset that is not 4-byte aligned.
	// Matches Rust wgpu-core UnalignedIndirectBufferOffset (compute.rs:899).
	ErrDispatchIndirectOffsetAlignment = errors.New("wgpu: indirect dispatch buffer offset not 4-byte aligned")

	// ErrDrawIndirectBufferOverrun is returned when DrawIndirect or
	// DrawIndexedIndirect args extend past the end of the buffer.
	// Matches Rust wgpu-core IndirectBufferOverrun (render.rs:2772-2779).
	ErrDrawIndirectBufferOverrun = errors.New("wgpu: indirect draw args exceed buffer size")

	// ErrDispatchIndirectBufferOverrun is returned when DispatchIndirect
	// args extend past the end of the buffer.
	// Matches Rust wgpu-core IndirectBufferOverrun (compute.rs:903-909).
	ErrDispatchIndirectBufferOverrun = errors.New("wgpu: indirect dispatch args exceed buffer size")
)

// Queue.Submit validation sentinel errors (VAL-A6).
// Returned when Submit detects that a command buffer references resources in
// an invalid state. These match Rust wgpu-core QueueSubmitError variants
// (device/queue.rs:484-516).
var (
	// ErrSubmitBufferDestroyed is returned when a submitted command buffer
	// references a buffer that has been released/destroyed.
	// Matches Rust QueueSubmitError::DestroyedResource for buffers.
	ErrSubmitBufferDestroyed = errors.New("wgpu: Submit: command buffer references destroyed buffer")

	// ErrSubmitBufferMapped is returned when a submitted command buffer
	// references a buffer that is currently mapped. Submitting GPU commands
	// that read/write a mapped buffer is a data race.
	// Matches Rust QueueSubmitError::BufferStillMapped.
	ErrSubmitBufferMapped = errors.New("wgpu: Submit: command buffer references mapped buffer")

	// ErrSubmitTextureDestroyed is returned when a submitted command buffer
	// references a texture that has been released/destroyed.
	// Matches Rust QueueSubmitError::DestroyedResource for textures.
	ErrSubmitTextureDestroyed = errors.New("wgpu: Submit: command buffer references destroyed texture")

	// ErrSubmitBindGroupDestroyed is returned when a submitted command buffer
	// references a bind group that has been released/destroyed.
	// Matches Rust wgpu-core validate_command_buffer bind_group.try_raw()
	// (device/queue.rs:1815-1817).
	ErrSubmitBindGroupDestroyed = errors.New("wgpu: Submit: command buffer references destroyed bind group")

	// ErrSubmitCommandBufferInvalid is returned when a command buffer has
	// already been submitted or was never properly finished.
	// Matches Rust QueueSubmitError::CommandEncoder for invalid command buffers.
	ErrSubmitCommandBufferInvalid = errors.New("wgpu: Submit: command buffer is invalid (already submitted or encoding error)")
)

// Re-export error types from core.
type GPUError = core.GPUError
type ErrorFilter = core.ErrorFilter

const (
	ErrorFilterValidation  = core.ErrorFilterValidation
	ErrorFilterOutOfMemory = core.ErrorFilterOutOfMemory
	ErrorFilterInternal    = core.ErrorFilterInternal
)
