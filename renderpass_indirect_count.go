//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

type indirectCountDrawConfig struct {
	validateDrawOp  string
	featureResource string
	indirectBuffer  *Buffer
	indirectOffset  uint64
	countBuffer     *Buffer
	countOffset     uint64
	maxDrawCount    uint32
	recordStride    uint64
	preValidate     func() error
	record          func()
}

func (p *RenderPassEncoder) executeIndirectCountDraw(cfg indirectCountDrawConfig) {
	if cfg.maxDrawCount == 0 {
		return
	}
	if !p.validateDrawState(cfg.validateDrawOp) {
		return
	}
	if err := core.RequireFeature(
		p.encoder.device.Features(),
		gputypes.FeatureMultiDrawIndirectCount,
		cfg.featureResource,
	); err != nil {
		p.encoder.setError(err)
		return
	}
	if cfg.preValidate != nil {
		if err := cfg.preValidate(); err != nil {
			p.encoder.setError(err)
			return
		}
	}
	if err := p.validateIndirectCountBuffers(
		cfg.indirectBuffer, cfg.indirectOffset,
		cfg.countBuffer, cfg.countOffset,
		cfg.maxDrawCount, cfg.recordStride,
	); err != nil {
		p.encoder.setError(err)
		return
	}
	p.trackIndirectCountBuffers(cfg.indirectBuffer, cfg.countBuffer)
	cfg.record()
}

func (p *RenderPassEncoder) trackIndirectCountBuffers(indirectBuffer, countBuffer *Buffer) {
	p.trackRef(indirectBuffer.core.Ref)
	p.trackRef(countBuffer.core.Ref)
	p.encoder.trackBuffer(indirectBuffer)
	p.encoder.trackBuffer(countBuffer)
}

func (p *RenderPassEncoder) validateIndexedIndirectCountPreconditions() error {
	if !p.indexBufferSet {
		return fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: no index buffer set (call SetIndexBuffer first): %w",
			ErrDrawMissingIndexBuffer)
	}
	if p.currentStripIndexFormat != nil && p.indexBufferFormat != *p.currentStripIndexFormat {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndexedIndirect: index buffer format %v does not match pipeline strip index format %v: %w",
			p.indexBufferFormat, *p.currentStripIndexFormat, ErrDrawIndexFormatMismatch)
	}
	return nil
}
