//go:build !rust && !(js && wasm)

package encoder

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// indirectCountDrawConfig is the per-call configuration of
// RenderPassEncoder.executeIndirectCountDraw: the two indirect buffers, the
// feature-ledger names, the record stride and the callbacks that validate and
// record. The declaration lived in the root package from the S5 Prepare until
// the Move (it stayed there only so the planner would accept the moving set);
// it relocates here with the pass, and the white-box test
// renderpass_indirect_count_test.go follows it onto the destination's
// fixtures.
type indirectCountDrawConfig struct {
	validateDrawOp  string
	featureResource string
	indirectBuffer  *resource.Buffer
	indirectOffset  uint64
	countBuffer     *resource.Buffer
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
		p.encoder.SetError(err)
		return
	}
	if cfg.preValidate != nil {
		if err := cfg.preValidate(); err != nil {
			p.encoder.SetError(err)
			return
		}
	}
	if err := p.validateIndirectCountBuffers(
		cfg.indirectBuffer, cfg.indirectOffset,
		cfg.countBuffer, cfg.countOffset,
		cfg.maxDrawCount, cfg.recordStride,
	); err != nil {
		p.encoder.SetError(err)
		return
	}
	p.trackIndirectCountBuffers(cfg.indirectBuffer, cfg.countBuffer)
	cfg.record()
}

func (p *RenderPassEncoder) trackIndirectCountBuffers(indirectBuffer, countBuffer *resource.Buffer) {
	p.trackRef(indirectBuffer.CoreBuffer().Ref)
	p.trackRef(countBuffer.CoreBuffer().Ref)
	p.encoder.TrackBuffer(indirectBuffer)
	p.encoder.TrackBuffer(countBuffer)
}

func (p *RenderPassEncoder) validateIndexedIndirectCountPreconditions() error {
	if !p.indexBufferSet {
		return fmt.Errorf("wgpu: RenderPass.DrawIndexedIndirect: no index buffer set (call SetIndexBuffer first): %w",
			core.ErrDrawMissingIndexBuffer)
	}
	if p.currentStripIndexFormat != nil && p.indexBufferFormat != *p.currentStripIndexFormat {
		return fmt.Errorf(
			"wgpu: RenderPass.DrawIndexedIndirect: index buffer format %v does not match pipeline strip index format %v: %w",
			p.indexBufferFormat, *p.currentStripIndexFormat, core.ErrDrawIndexFormatMismatch)
	}
	return nil
}
