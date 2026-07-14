//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestRenderPassEncoderCountedIndirectRemainsUnsupported(t *testing.T) {
	enc := &CommandEncoder{}
	if err := enc.BeginEncoding("indirect"); err != nil {
		t.Fatal(err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{}})

	pass.DrawIndirect(&Buffer{id: 7, size: 64}, 0, 2)
	pass.SetIndexBuffer(&Buffer{id: 9, size: 64}, gputypes.IndexFormatUint32, 16)
	pass.DrawIndexedIndirect(&Buffer{id: 8, size: 64}, 0, 2)

	if len(enc.commands) != 1 {
		t.Fatalf("commands = %d, want only SetIndexBufferCommand", len(enc.commands))
	}
}
