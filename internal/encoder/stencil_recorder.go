//go:build !rust && !(js && wasm)

package encoder

// The stencil blit's recorder adapter, relocated from the root package by
// S4's Move (the "staying-side shuttle" file stencil_recorder_native.go of
// commit ae8631e). It adapts a CommandEncoder to the Recorder seam the
// stencil upload payload records through, and it is what the encoder's own
// stencil lowerings record into (lowerStencilCopy).
//
// The queue, which stays in the root package until S9, records its staging
// uploads through a root-side adapter over the same seam
// (stencil_recorder_native.go), which delegates here: the payload never names
// the encoder type, and the destination never names the payload.
//
// It is an adapter rather than methods on CommandEncoder itself: the seam's
// methods are exported, and the public encoder declaration layer must not grow
// internal seam surface (ADR-070 §15.1).

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// BlitRecorder returns the Recorder view of the encoder: the seam the stencil
// upload payload records its passes through.
func (e *CommandEncoder) BlitRecorder() Recorder { return stencilRecorder{e} }

// stencilRecorder adapts a CommandEncoder to Recorder.
type stencilRecorder struct{ e *CommandEncoder }

// Own hands a resource the blit created to the encoder, which releases it once
// the command buffer is spent.
func (r stencilRecorder) Own(x Releasable) { r.e.Own(x) }

// BeginLayerPass opens one blit layer's render pass, binds the layer's bind
// group at index 0 and returns the pass's command recorder.
//
// The group is bound to the HAL pass directly: the frontend built both the
// group and the pipeline layout for this pass alone, so the public pass
// encoder's binder — a cache that validates user bind groups against a
// pipeline layout — has nothing to check here. The group's usages still join
// the pass's usage scope, so core transitions the resources it binds before
// the pass (CoreCommandEncoder.EndRenderPass).
func (r stencilRecorder) BeginLayerPass(desc *core.RenderPassDescriptor, group *resource.BindGroup) (PassRecorder, error) {
	if r.e.released {
		return nil, core.ErrReleased
	}
	corePass, err := r.e.core.BeginRenderPass(desc)
	if err != nil {
		return nil, err
	}
	if group != nil {
		if err := corePass.MergeBindGroup(group.Usages()); err != nil {
			// A deferred error, like the public pass encoder's: the pass stays
			// open for End and the error surfaces at Finish.
			r.e.SetError(fmt.Errorf("wgpu: stencil blit: bind group: %w", err))
		}
	}
	pass := &stencilPass{e: r.e, core: corePass, raw: corePass.RawPass(), group: group}
	if r.e.test != nil {
		pass.test = r.e.test
		pass.testPass = r.e.test.PassBegan(r.e.testEncoder, false, desc.Label, nil, nil)
		pass.test.BindGroupBound(pass.testPass, 0, nil)
	}
	return pass, nil
}

// stencilPass records one blit layer's commands. It drives the core pass for
// the state core tracks (scissor, stencil reference, draws) and the HAL pass
// for the pipeline and bind group bindings, for the reason BeginLayerPass
// gives.
type stencilPass struct {
	e          *CommandEncoder
	core       *core.CoreRenderPassEncoder
	raw        hal.RenderPassEncoder
	group      *resource.BindGroup
	groupBound bool
	test       core.TestHooks
	testPass   int
}

// SetPipeline binds the HAL pipeline and keeps it alive until the GPU
// completes the submission, as the public pass encoder does for user
// pipelines (the blit's pipelines are device-cached; the reference outlives
// device teardown races).
func (p *stencilPass) SetPipeline(pl *pipeline.RenderPipeline) {
	if pl == nil {
		return
	}
	p.e.TrackRef(pl.Ref())
	if p.raw != nil && pl.HAL() != nil {
		p.raw.SetPipeline(pl.HAL())
	}
}

// SetStencilReference sets the stencil reference value.
func (p *stencilPass) SetStencilReference(ref uint32) { p.core.SetStencilReference(ref) }

// SetScissorRect sets the scissor rectangle for clipping.
func (p *stencilPass) SetScissorRect(x, y, width, height uint32) {
	p.core.SetScissorRect(gputypes.ScissorRect{X: x, Y: y, Width: width, Height: height})
}

// Draw draws primitives.
func (p *stencilPass) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	args := gputypes.DrawArgs{VertexCount: vertexCount, InstanceCount: instanceCount, FirstVertex: firstVertex, FirstInstance: firstInstance}
	p.bindGroup()
	p.core.Draw(args)
	if p.test != nil {
		p.test.Drew(p.testPass, args)
	}
}

// bindGroup binds the layer's bind group to the HAL pass on the first draw,
// not at the pass opening: a backend resolves the group's buffers and textures
// against the CURRENT pipeline layout when it receives the group (GLES reads
// it from the bound program's layout), and the pass's pipeline is set between
// the opening and the first draw. The public pass encoder's binder defers its
// backend binding to the first draw for the same reason.
func (p *stencilPass) bindGroup() {
	if p.groupBound || p.raw == nil || p.group == nil {
		return
	}
	p.groupBound = true
	if halGroup := p.group.HAL(); halGroup != nil {
		p.raw.SetBindGroup(0, halGroup, nil)
	}
}

// End ends the render pass.
func (p *stencilPass) End() error { return p.core.End() }
