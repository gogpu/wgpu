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
