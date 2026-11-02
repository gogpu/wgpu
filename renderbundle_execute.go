//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
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
		p.encoder.trackBuffer(buffer)
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
			raw.SetPipeline(cmd.pipeline.hal)
		case bundleSetBindGroup:
			raw.SetBindGroup(cmd.slot, cmd.group.hal, cmd.offsets)
		case bundleSetVertexBuffer:
			buffer := cmd.buffer.halBuffer()
			if buffer == nil {
				return fmt.Errorf("vertex buffer %q: %w", cmd.buffer.Label(), core.ErrResourceDestroyed)
			}
			raw.SetVertexBuffer(cmd.slot, buffer, cmd.offset)
		case bundleSetIndexBuffer:
			buffer := cmd.buffer.halBuffer()
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
