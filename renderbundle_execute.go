//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// refLabel returns the debug label of a resource reference, or "".
func refLabel(ref *core.ResourceRef) string {
	if ref == nil {
		return ""
	}
	return ref.Label()
}

// executeBundle checks that a bundle fits this pass, merges its usages into
// the pass's usage scope, keeps its resources alive until the GPU completes
// the submission, and replays its commands into the backend pass.
// Matches Rust wgpu-core command/render.rs execute_bundle and
// command/bundle.rs RenderBundle::execute.
func (p *RenderPassEncoder) executeBundle(bundle *RenderBundle) error {
	if err := p.checkBundle(bundle); err != nil {
		return err
	}
	if err := p.core.MergeRenderBundle(bundle.scope); err != nil {
		return fmt.Errorf("bundle %q: %w", bundle.label, err)
	}
	for _, ref := range bundle.refs {
		p.trackRef(ref)
	}
	for _, group := range bundle.groups {
		p.encoder.trackBindGroup(group)
	}
	for _, buffer := range bundle.buffers {
		p.encoder.trackBuffer(buffer.impl)
	}
	if err := bundle.replay(p.core.RawPass()); err != nil {
		return fmt.Errorf("bundle %q: %w", bundle.label, err)
	}
	return nil
}

// checkBundle checks that a bundle can run in this pass: same device, the
// pass's attachment formats and sample count, and read-only where the pass's
// depth/stencil is. Matches the checks of Rust render.rs execute_bundle.
func (p *RenderPassEncoder) checkBundle(bundle *RenderBundle) error {
	if bundle == nil {
		return fmt.Errorf("%w: render bundle is nil", errBundleCommand)
	}
	if bundle.released.Load() {
		return fmt.Errorf("bundle %q: %w", bundle.label, ErrReleased)
	}
	if bundle.bundleContents == nil {
		return fmt.Errorf("%w: render bundle %q was not created by RenderBundleEncoder.Finish", errBundleCommand, bundle.label)
	}
	if bundle.device != p.encoder.device {
		return fmt.Errorf("bundle %q: %w", bundle.label, errBundleDevice)
	}
	if ctx := p.core.PassContext(); ctx != nil {
		if err := ctx.CheckCompatible(&bundle.target.context, bundle.label); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.label, err)
		}
	}
	if (p.depthReadOnly && !bundle.target.depthReadOnly) || (p.stencilReadOnly && !bundle.target.stencilReadOnly) {
		return fmt.Errorf("bundle %q: %w", bundle.label, errBundleReadOnly)
	}
	return nil
}

// ExecuteBundles executes reusable render command bundles in this render pass.
// Each bundle's commands are replayed into the pass, its resources join the
// pass's usage scope, and the pass is left without a pipeline, bind groups,
// vertex buffers or index buffer: draws after it must set them again.
//
// ExecuteBundles and resetBundleState keep their home here, next to
// executeBundle, instead of moving with the rest of renderpass_native.go in
// S5: they name *RenderBundle, which stays in this package through S6, and the
// destination cannot name a root type. The Move gives the internal encoder a
// small method for the replay-and-reset step this file then calls.
func (p *RenderPassEncoder) ExecuteBundles(bundles ...*RenderBundle) {
	for _, bundle := range bundles {
		if err := p.executeBundle(bundle); err != nil {
			p.encoder.setError(fmt.Errorf("wgpu: RenderPass.ExecuteBundles: %w", err))
			break
		}
	}
	if len(bundles) > 0 {
		p.resetBundleState()
	}
}

// resetBundleState clears the pipeline, bind groups, vertex buffers and
// index buffer after ExecuteBundles: a bundle leaves the backend with its own
// state, and WebGPU requires them to be set again (executeBundles; Rust
// wgpu-core State::reset_bundle). Without it the binder would take the
// backend's bindings for current.
func (p *RenderPassEncoder) resetBundleState() {
	p.binder.reset()
	p.pipelineSet = false
	p.requiredVertexBuffers = 0
	p.currentStripIndexFormat = nil
	p.vertexBufferCount = 0
	p.indexBufferSet = false
}

// replay records the bundle's resolved commands into a backend render pass.
// Finish validated them, so no state is tracked here.
// Matches Rust wgpu-core command/bundle.rs RenderBundle::execute.
func (b *RenderBundle) replay(raw hal.RenderPassEncoder) error {
	if raw == nil {
		return nil
	}
	for i := range b.commands {
		cmd := &b.commands[i]
		switch cmd.kind {
		case bundleSetPipeline:
			raw.SetPipeline(cmd.pipeline.impl.HAL())
		case bundleSetBindGroup:
			raw.SetBindGroup(cmd.slot, cmd.group.HAL(), cmd.offsets)
		case bundleSetVertexBuffer:
			buffer := cmd.buffer.impl.HALBuffer()
			if buffer == nil {
				return fmt.Errorf("vertex buffer %q: %w", cmd.buffer.Label(), core.ErrResourceDestroyed)
			}
			raw.SetVertexBuffer(cmd.slot, buffer, cmd.offset)
		case bundleSetIndexBuffer:
			buffer := cmd.buffer.impl.HALBuffer()
			if buffer == nil {
				return fmt.Errorf("index buffer %q: %w", cmd.buffer.Label(), core.ErrResourceDestroyed)
			}
			raw.SetIndexBuffer(buffer, cmd.format, cmd.offset)
		case bundleDraw:
			raw.Draw(cmd.draw)
		case bundleDrawIndexed:
			raw.DrawIndexed(cmd.drawIndexed)
		}
	}
	return nil
}
