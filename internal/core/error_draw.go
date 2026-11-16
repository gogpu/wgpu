//go:build !(js && wasm)

package core

import "errors"

// Draw-time validation sentinel errors. A render pass wraps them with the
// details of the failed draw, and the caller matches them with errors.Is
// after Finish.
//
// The definitions lived in the root package until the render pass moved into
// internal/encoder (ADR-070 S5): a destination declaration cannot name a root
// value, so the values live here and the root package re-exports the same
// ones (error.go), which keeps errors.Is working through the public API
// (§15.1). The dispatch sentinels followed with the compute pass (S6, see
// error_dispatch.go).
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

	// ErrDrawIndexFormatMismatch is returned when the index buffer format set
	// on the pass does not match the pipeline's strip index format.
	ErrDrawIndexFormatMismatch = errors.New("wgpu: index buffer format does not match pipeline strip index format")

	// ErrDrawIndirectBufferUsage is returned when an indirect draw buffer
	// lacks the INDIRECT usage.
	ErrDrawIndirectBufferUsage = errors.New("wgpu: indirect draw buffer missing INDIRECT usage")

	// ErrDrawIndirectOffsetAlignment is returned when an indirect draw offset
	// is not 4-byte aligned.
	ErrDrawIndirectOffsetAlignment = errors.New("wgpu: indirect draw buffer offset not 4-byte aligned")

	// ErrDrawIndirectBufferOverrun is returned when the indirect draw
	// arguments do not fit in the buffer.
	ErrDrawIndirectBufferOverrun = errors.New("wgpu: indirect draw args exceed buffer size")
)
