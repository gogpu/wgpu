//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// Sentinel errors re-exported from HAL.
var (
	ErrDeviceLost      = hal.ErrDeviceLost
	ErrOutOfMemory     = hal.ErrDeviceOutOfMemory
	ErrSurfaceLost     = hal.ErrSurfaceLost
	ErrSurfaceOutdated = hal.ErrSurfaceOutdated
	ErrTimeout         = hal.ErrTimeout

	// ErrContextReleased is returned by Adapter.RequestDevice when the
	// backend's native context is gone. On GLES on Linux that happens when a
	// Wayland window's Surface is released before RequestDevice: the EGL
	// context built on the app's display dies with its last Surface and
	// Device. It wraps ErrDeviceLost, as WebGPU's requestDevice on an expired
	// adapter yields a lost device; request a new adapter, with a live
	// surface, and a new device.
	ErrContextReleased = hal.ErrContextReleased

	// ErrWorkgroupBarrierUnsupported is no longer returned since v0.34.8 and
	// is kept for compatibility, so code that checks it with errors.Is still
	// builds. In v0.34.7 Device.CreateComputePipeline returned it on the
	// software backend for a compute shader sharing workgroup memory across a
	// control barrier (a tree reduction, a shared tile); since v0.34.8 the
	// software backend runs such shaders (P1-SW-025).
	ErrWorkgroupBarrierUnsupported = hal.ErrWorkgroupBarrierUnsupported
)

// Public API sentinel errors.
var (
	// ErrReleased is returned when operating on a released resource. The value
	// is internal/core's on every build — the declaration carries no build
	// constraint, so the browser build binds the same value — and the extraction
	// children return it too. The same value on both sides is what keeps
	// errors.Is working through the public API in all three contexts (§15.1).
	ErrReleased = core.ErrReleased

	// Adapter-selection sentinels (ADR-083 §2.2). The selection paths return them
	// from internal/core; binding the same values here keeps errors.Is working
	// through the public API. Each condition names exactly one of them:
	//
	// ErrNoBackendRegistered is returned by Instance.RequestAdapter when the
	// requested backend mask selects no registered backend: the backend is
	// compiled out of the build or excluded from the mask.
	ErrNoBackendRegistered = core.ErrNoBackendRegistered

	// ErrNoHardwareAdapter is returned by Instance.RequestAdapter when backends
	// are registered for the requested mask but none produced an adapter on this
	// machine, an adapter not compatible with the requested surface included.
	ErrNoHardwareAdapter = core.ErrNoHardwareAdapter

	// ErrNoFallbackAdapter is returned by Instance.RequestAdapter with
	// ForceFallbackAdapter set when no CPU adapter is available, the software
	// backend included.
	ErrNoFallbackAdapter = core.ErrNoFallbackAdapter
)

// Draw-time validation sentinel errors.
// These are wrapped with additional context (e.g., method name) and surfaced
// as deferred errors on Finish(). Use errors.Is() to match programmatically.
//
// Matches Rust wgpu-core DrawError / DispatchError variants
// (command/render.rs:542-593, command/compute.rs:278-284).
//
// The draw sentinels are internal/core's values: the render pass that returns
// them now lives in internal/encoder, and binding the same values here keeps
// errors.Is working through the public API (§15.1). The dispatch sentinels
// stay local to this package until the compute pass moves (S6).
var (
	// ErrDrawMissingPipeline is returned when Draw/DrawIndexed/DrawIndirect is
	// called before SetPipeline on a render pass encoder.
	ErrDrawMissingPipeline = core.ErrDrawMissingPipeline

	// ErrDrawMissingBindGroup is returned when a draw command is issued but not
	// all bind groups required by the current pipeline have been set.
	ErrDrawMissingBindGroup = core.ErrDrawMissingBindGroup

	// ErrDrawIncompatibleBindGroup is returned when a draw command is issued but
	// the bind group set at a slot has an incompatible layout with the pipeline.
	ErrDrawIncompatibleBindGroup = core.ErrDrawIncompatibleBindGroup

	// ErrDrawMissingVertexBuffer is returned when a draw command is issued but
	// fewer vertex buffers have been set than the pipeline requires.
	ErrDrawMissingVertexBuffer = core.ErrDrawMissingVertexBuffer

	// ErrDrawMissingIndexBuffer is returned when DrawIndexed or
	// DrawIndexedIndirect is called before SetIndexBuffer.
	ErrDrawMissingIndexBuffer = core.ErrDrawMissingIndexBuffer

	// ErrDrawMissingBlendConstant is returned when a draw command is issued
	// but the pipeline uses BlendFactorConstant and SetBlendConstant was not called.
	ErrDrawMissingBlendConstant = core.ErrDrawMissingBlendConstant

	// ErrDrawLateBufferTooSmall is returned when a buffer bound via SetBindGroup
	// is smaller than the shader-required minimum for a binding whose layout
	// has MinBindingSize == 0.
	ErrDrawLateBufferTooSmall = core.ErrDrawLateBufferTooSmall

	// ErrDispatchMissingPipeline is returned when Dispatch/DispatchIndirect is
	// called before SetPipeline on a compute pass encoder.
	ErrDispatchMissingPipeline = core.ErrDispatchMissingPipeline

	// ErrDispatchMissingBindGroup is returned when a dispatch command is issued
	// but not all bind groups required by the current pipeline have been set.
	ErrDispatchMissingBindGroup = core.ErrDispatchMissingBindGroup

	// ErrDispatchIncompatibleBindGroup is returned when a dispatch command is
	// issued but the bind group set at a slot has an incompatible layout.
	ErrDispatchIncompatibleBindGroup = core.ErrDispatchIncompatibleBindGroup

	// ErrDispatchLateBufferTooSmall is returned when a buffer bound via
	// SetBindGroup is smaller than the shader-required minimum for a dispatch.
	ErrDispatchLateBufferTooSmall = core.ErrDispatchLateBufferTooSmall

	// ErrDispatchWorkgroupCountExceeded is returned when Dispatch is called with
	// workgroup counts exceeding the device limit.
	ErrDispatchWorkgroupCountExceeded = core.ErrDispatchWorkgroupCountExceeded

	// ErrDrawIndexFormatMismatch is returned when the index buffer format
	// does not match the pipeline's StripIndexFormat for strip topologies.
	// Matches Rust wgpu-core DrawError::UnmatchedIndexFormats (render.rs:576-580).
	ErrDrawIndexFormatMismatch = core.ErrDrawIndexFormatMismatch

	// ErrDrawIndirectBufferUsage is returned when DrawIndirect or
	// DrawIndexedIndirect is called with a buffer that lacks BufferUsageIndirect.
	// Matches Rust wgpu-core check_usage(BufferUsages::INDIRECT) (render.rs:2763).
	ErrDrawIndirectBufferUsage = core.ErrDrawIndirectBufferUsage

	// ErrDrawIndirectOffsetAlignment is returned when DrawIndirect or
	// DrawIndexedIndirect is called with an offset that is not 4-byte aligned.
	// Matches Rust wgpu-core UnalignedIndirectBufferOffset (render.rs:2766).
	ErrDrawIndirectOffsetAlignment = core.ErrDrawIndirectOffsetAlignment

	// ErrDispatchIndirectBufferUsage is returned when DispatchIndirect is called
	// with a buffer that lacks BufferUsageIndirect.
	// Matches Rust wgpu-core check_usage(BufferUsages::INDIRECT) (compute.rs:896).
	ErrDispatchIndirectBufferUsage = core.ErrDispatchIndirectBufferUsage

	// ErrDispatchIndirectOffsetAlignment is returned when DispatchIndirect is
	// called with an offset that is not 4-byte aligned.
	// Matches Rust wgpu-core UnalignedIndirectBufferOffset (compute.rs:899).
	ErrDispatchIndirectOffsetAlignment = core.ErrDispatchIndirectOffsetAlignment

	// ErrDrawIndirectBufferOverrun is returned when DrawIndirect or
	// DrawIndexedIndirect args extend past the end of the buffer.
	// Matches Rust wgpu-core IndirectBufferOverrun (render.rs:2772-2779).
	ErrDrawIndirectBufferOverrun = core.ErrDrawIndirectBufferOverrun

	// ErrDispatchIndirectBufferOverrun is returned when DispatchIndirect
	// args extend past the end of the buffer.
	// Matches Rust wgpu-core IndirectBufferOverrun (compute.rs:903-909).
	ErrDispatchIndirectBufferOverrun = core.ErrDispatchIndirectBufferOverrun
)

// Queue.Submit validation sentinel errors (VAL-A6).
// Returned when Submit detects that a command buffer references resources in
// an invalid state. These match Rust wgpu-core QueueSubmitError variants
// (device/queue.rs:484-516).
//
// The values are internal/core's: Submit validation moved with the queue to
// internal/queue (ADR-070 S9), and binding the same values here keeps
// errors.Is working through the public API (§15.1).
var (
	// ErrSubmitBufferDestroyed is returned when a submitted command buffer
	// references a buffer whose GPU memory is gone. Release() after recording
	// is not that: the command buffer keeps the buffer alive until its work
	// completes, as a dropped Arc<Buffer> stays alive in Rust wgpu-core.
	// Matches Rust QueueSubmitError::DestroyedResource for buffers.
	ErrSubmitBufferDestroyed = core.ErrSubmitBufferDestroyed

	// ErrSubmitBufferMapped is returned when a submitted command buffer
	// references a buffer that is currently mapped. Submitting GPU commands
	// that read/write a mapped buffer is a data race.
	// Matches Rust QueueSubmitError::BufferStillMapped.
	ErrSubmitBufferMapped = core.ErrSubmitBufferMapped

	// ErrSubmitTextureDestroyed is returned when a submitted command buffer
	// references a texture whose work can no longer run: its device is gone,
	// or a surface texture's acquisition has ended. Release() after recording
	// is not that: the command buffer keeps the texture alive until its work
	// completes. Matches Rust QueueSubmitError::DestroyedResource for textures.
	ErrSubmitTextureDestroyed = core.ErrSubmitTextureDestroyed

	// ErrSubmitBindGroupDestroyed reports a submitted command buffer that
	// references a destroyed bind group. The native backends no longer return
	// it: Release() after recording is a drop, the command buffer holds the
	// bind group until its work completes, and Submit checks the resources the
	// group binds instead, with the buffer and texture errors above (Rust
	// bind_group.try_raw, device/queue.rs:2171-2180). It is kept so code that
	// matches it still compiles. No internal package returns it, so it stays
	// declared here.
	ErrSubmitBindGroupDestroyed = errors.New("wgpu: Submit: command buffer references destroyed bind group")

	// ErrSubmitQuerySetDestroyed reports a submitted command buffer that
	// references a query set whose HAL query set is gone. Release() after
	// recording is not that: the command buffer keeps the set alive until its
	// work completes. Matches Rust QueueSubmitError::DestroyedResource for
	// query sets (query_set.try_raw, device/queue.rs:2167-2168).
	ErrSubmitQuerySetDestroyed = core.ErrSubmitQuerySetDestroyed

	// ErrSubmitCommandBufferInvalid is returned when a command buffer has
	// already been submitted or was never properly finished.
	// Matches Rust QueueSubmitError::CommandEncoder for invalid command buffers.
	ErrSubmitCommandBufferInvalid = core.ErrSubmitCommandBufferInvalid
)

// GPUError represents a captured GPU error from an error scope.
//
// It is a defined type in this package (ADR-070 §15: a public API never
// aliases an internal type); the shape is the one the core type had.
type GPUError struct {
	// Type identifies the category of the error.
	Type ErrorFilter

	// Message provides a human-readable description of the error.
	Message string
}

// Error implements the error interface.
func (e *GPUError) Error() string {
	return fmt.Sprintf("GPU %s error: %s", e.Type, e.Message)
}

// ErrorFilter selects which errors an error scope captures.
type ErrorFilter int

const (
	// ErrorFilterValidation captures validation errors.
	// These occur when API usage violates the WebGPU specification
	// (e.g., invalid descriptors, missing resources, state errors).
	ErrorFilterValidation ErrorFilter = iota

	// ErrorFilterOutOfMemory captures out-of-memory errors.
	// These occur when GPU memory allocation fails.
	ErrorFilterOutOfMemory

	// ErrorFilterInternal captures internal errors.
	// These occur due to implementation bugs or unexpected GPU behavior.
	ErrorFilterInternal
)

// String returns a human-readable name for the error filter. It keeps the
// method set the core alias exposed publicly before v0.35.0. The text table
// lives in internal/core, keyed by the ErrorFilter constants, and serves both
// builds that declare this type (native and rust), so their texts cannot
// drift apart.
func (f ErrorFilter) String() string {
	return core.ErrorFilter(f).String()
}
