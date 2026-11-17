//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/internal/encoder"
)

// ComputePassEncoder records compute dispatch commands.
//
// Created by CommandEncoder.BeginComputePass().
// Must be ended with End() before the CommandEncoder can be finished.
//
// NOT thread-safe.
//
// The implementation lives in internal/encoder (ADR-070 S6); this declaration
// is the public surface: every method is one delegation, and the wrappers are
// converted to the implementation's handle types at the call boundary.
type ComputePassEncoder struct {
	impl *encoder.ComputePassEncoder
}

// newComputePassEncoder builds the public pass over the destination's.
func newComputePassEncoder(start *encoder.ComputePassStart, e *CommandEncoder) *ComputePassEncoder {
	return &ComputePassEncoder{impl: encoder.NewComputePassEncoder(start, e.impl)}
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	p.impl.SetPipeline(encoderComputePipeline(pipeline))
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline, or name an index the current pipeline does not use: the group
// is bound to the backend at the next dispatch whose pipeline layout uses it.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	p.impl.SetBindGroup(index, encoderBindGroup(group), offsets)
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	p.impl.Dispatch(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	p.impl.DispatchIndirect(encoderBuffer(buffer), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	return p.impl.End()
}
