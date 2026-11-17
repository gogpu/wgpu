//go:build rust

package wgpu

import (
	"github.com/gogpu/wgpu/internal/encoder"
)

// ComputePassEncoder records compute dispatch commands.
// On Rust backend, this wraps go-webgpu/webgpu ComputePassEncoder.
//
// The implementation lives in internal/encoder (ADR-070 S6); this declaration
// is the public surface: every method is one delegation.
type ComputePassEncoder struct {
	impl *encoder.ComputePassEncoder
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	p.impl.SetPipeline(encoderComputePipeline(pipeline))
}

// SetBindGroup sets a bind group for the given index.
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
