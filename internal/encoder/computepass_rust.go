//go:build rust

package encoder

import (
	rwgpu "github.com/go-webgpu/webgpu/wgpu"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// ComputePassEncoder records compute dispatch commands.
// On Rust backend, this wraps go-webgpu/webgpu ComputePassEncoder.
type ComputePassEncoder struct {
	r        *rwgpu.ComputePassEncoder
	released bool
}

// NewComputePassEncoder builds the rust compute pass the encoder began.
func NewComputePassEncoder(pass *rwgpu.ComputePassEncoder) *ComputePassEncoder {
	return &ComputePassEncoder{r: pass}
}

// SetPipeline sets the active compute pipeline.
func (p *ComputePassEncoder) SetPipeline(pl *pipeline.ComputePipeline) {
	if pl == nil || pl.R() == nil {
		return
	}
	p.r.SetPipeline(pl.R())
}

// SetBindGroup sets a bind group for the given index.
func (p *ComputePassEncoder) SetBindGroup(index uint32, group *resource.BindGroup, offsets []uint32) {
	if group == nil || group.Rust() == nil {
		return
	}
	p.r.SetBindGroup(index, group.Rust(), offsets)
}

// Dispatch dispatches compute work.
func (p *ComputePassEncoder) Dispatch(x, y, z uint32) {
	// go-webgpu uses DispatchWorkgroups instead of Dispatch.
	p.r.DispatchWorkgroups(x, y, z)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *ComputePassEncoder) DispatchIndirect(buffer *resource.Buffer, offset uint64) {
	if buffer == nil || buffer.R() == nil {
		return
	}
	p.r.DispatchWorkgroupsIndirect(buffer.R(), offset)
}

// End ends the compute pass.
func (p *ComputePassEncoder) End() error {
	if p.released {
		return core.ErrReleased
	}
	p.released = true
	p.r.End()
	return nil
}
