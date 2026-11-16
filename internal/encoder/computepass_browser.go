//go:build js && wasm

package encoder

import (
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// ComputePassEncoder records compute dispatch commands.
// On browser, this wraps a GPUComputePassEncoder via internal/browser.ComputePassEncoder.
type ComputePassEncoder struct {
	browser  *browser.ComputePassEncoder
	released bool
}

// NewComputePassEncoder builds the browser compute pass the encoder began.
func NewComputePassEncoder(pass *browser.ComputePassEncoder) *ComputePassEncoder {
	return &ComputePassEncoder{browser: pass}
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pl *pipeline.ComputePipeline) {
	if pl == nil || pl.Browser() == nil {
		return
	}
	p.browser.SetPipeline(pl.Browser().Ref())
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *resource.BindGroup, offsets []uint32) {
	if group == nil || group.Browser() == nil {
		return
	}
	p.browser.SetBindGroup(index, group.Browser().Ref(), offsets)
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	p.browser.DispatchWorkgroups(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *resource.Buffer, offset uint64) {
	if buffer == nil || buffer.Browser() == nil {
		return
	}
	p.browser.DispatchIndirect(buffer.Browser().Ref(), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	if p.released {
		return resource.ErrReleased
	}
	p.released = true
	p.browser.End()
	return nil
}
