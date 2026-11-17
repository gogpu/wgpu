//go:build !(js && wasm)

package core

import "errors"

// Dispatch-time validation sentinel errors. A compute pass wraps them with the
// details of the failed dispatch, and the caller matches them with errors.Is
// after Finish.
//
// The definitions lived in the root package until the compute pass moved into
// internal/encoder (ADR-070 S6): a destination declaration cannot name a root
// value, so the values live here and the root package re-exports the same
// ones (error.go), which keeps errors.Is working through the public API
// (§15.1) — the ErrDraw* precedent from S5.
var (
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

	// ErrDispatchIndirectBufferUsage is returned when DispatchIndirect is called
	// with a buffer that lacks BufferUsageIndirect.
	// Matches Rust wgpu-core check_usage(BufferUsages::INDIRECT) (compute.rs:896).
	ErrDispatchIndirectBufferUsage = errors.New("wgpu: indirect dispatch buffer missing INDIRECT usage")

	// ErrDispatchIndirectOffsetAlignment is returned when DispatchIndirect is
	// called with an offset that is not 4-byte aligned.
	// Matches Rust wgpu-core UnalignedIndirectBufferOffset (compute.rs:899).
	ErrDispatchIndirectOffsetAlignment = errors.New("wgpu: indirect dispatch buffer offset not 4-byte aligned")

	// ErrDispatchIndirectBufferOverrun is returned when DispatchIndirect
	// args extend past the end of the buffer.
	// Matches Rust wgpu-core IndirectBufferOverrun (compute.rs:903-909).
	ErrDispatchIndirectBufferOverrun = errors.New("wgpu: indirect dispatch args exceed buffer size")
)
