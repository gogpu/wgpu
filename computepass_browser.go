//go:build js && wasm

package wgpu

import "github.com/gogpu/wgpu/internal/browser"

// ComputePassEncoder records compute dispatch commands.
// On browser, this wraps a GPUComputePassEncoder via internal/browser.ComputePassEncoder.
type ComputePassEncoder struct {
	browser  *browser.ComputePassEncoder
	released bool
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	if pipeline == nil || pipeline.browser == nil {
		return
	}
	p.browser.SetPipeline(pipeline.browser.Ref())
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if group == nil || group.browser == nil {
		return
	}
	p.browser.SetBindGroup(index, group.browser.Ref(), offsets)
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	p.browser.DispatchWorkgroups(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	if buffer == nil || buffer.browser == nil {
		return
	}
	p.browser.DispatchIndirect(buffer.browser.Ref(), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	if p.released {
		return ErrReleased
	}
	p.released = true
	p.browser.End()
	return nil
}
