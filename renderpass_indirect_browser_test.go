//go:build js && wasm

package wgpu

import (
	"slices"
	"syscall/js"
	"testing"

	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/resource"
)

func TestBrowserDrawIndexedIndirectInvalidSpanDelegatesOneFailingCall(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize uint64
		offset     uint64
		drawCount  uint32
		want       []float64
	}{
		{name: "later record fails", bufferSize: 59, offset: 0, drawCount: 3, want: []float64{40}},
		{name: "count one preserves offset", bufferSize: 20, offset: 24, drawCount: 1, want: []float64{24}},
		{name: "arithmetic overflow", bufferSize: 64, offset: ^uint64(0) - 3, drawCount: 2, want: []float64{64}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var offsets []float64
			draw := js.FuncOf(func(_ js.Value, args []js.Value) any {
				offsets = append(offsets, args[1].Float())
				return nil
			})
			defer draw.Release()

			rawPass := js.Global().Get("Object").New()
			rawPass.Set("drawIndexedIndirect", draw)
			pass := &RenderPassEncoder{browser: browser.NewRenderPassEncoder(rawPass)}

			rawBuffer := js.Global().Get("Object").New()
			rawBuffer.Set("size", float64(test.bufferSize))
			rawBuffer.Set("usage", 0)
			buffer := &Buffer{impl: resource.NewBufferBrowser(
				browser.NewBuffer(rawBuffer),
				uint64(test.bufferSize),
				0,
				resource.MapStateUnmapped,
			)}

			pass.MultiDrawIndexedIndirect(buffer, test.offset, test.drawCount)

			if !slices.Equal(offsets, test.want) {
				t.Fatalf("underlying offsets = %v, want %v", offsets, test.want)
			}
		})
	}
}
