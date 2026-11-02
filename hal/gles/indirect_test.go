//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestRenderPassEncoderIndirectDraws checks the recording of indirect draws:
// one command per call with its record count and offset (Rust wgpu-hal gles
// command.rs draw_indirect / draw_indexed_indirect), and nothing for a range
// past the buffer. Counted indirect draws stay unsupported.
func TestRenderPassEncoderIndirectDraws(t *testing.T) {
	enc := &CommandEncoder{}
	if err := enc.BeginEncoding("indirect"); err != nil {
		t.Fatal(err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{}})

	pass.DrawIndirect(&Buffer{id: 7, size: 64}, 16, 2)
	pass.SetIndexBuffer(&Buffer{id: 9, size: 64}, gputypes.IndexFormatUint32, 16)
	pass.DrawIndexedIndirect(&Buffer{id: 8, size: 64}, 4, 2)
	pass.DrawIndirect(&Buffer{id: 7, size: 64}, 40, 2)        // 40 + 2*16 > 64
	pass.DrawIndexedIndirect(&Buffer{id: 8, size: 64}, 28, 2) // 28 + 2*20 > 64
	pass.DrawIndirectCount(&Buffer{id: 7, size: 64}, 0, &Buffer{id: 6, size: 4}, 0, 2)
	pass.DrawIndexedIndirectCount(&Buffer{id: 8, size: 64}, 0, &Buffer{id: 6, size: 4}, 0, 2)

	type rec struct {
		buffer  uint32
		count   uint32
		offset  uint64
		indexed bool
	}
	var got []rec
	for _, c := range enc.commands {
		switch c := c.(type) {
		case *DrawIndirectCommand:
			got = append(got, rec{c.buffer, c.count, c.offset, false})
		case *DrawIndexedIndirectCommand:
			if c.indexFormat != gputypes.IndexFormatUint32 {
				t.Errorf("indexed indirect index format %v, want Uint32", c.indexFormat)
			}
			got = append(got, rec{c.buffer, c.count, c.offset, true})
		}
	}
	want := []rec{{7, 2, 16, false}, {8, 2, 4, true}}
	if len(got) != len(want) {
		t.Fatalf("indirect commands %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("indirect command %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
