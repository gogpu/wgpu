//go:build rust

package wgpu

import (
	"errors"
	"testing"
)

// TestConvertRenderPassDescriptorRustDepthSlice checks that a set DepthSlice
// is refused under -tags rust instead of being dropped: go-webgpu does not
// map depthSlice yet, so the pass would otherwise render into slice 0. A
// non-nil 0xFFFFFFFF is refused the same way, never read as "undefined".
func TestConvertRenderPassDescriptorRustDepthSlice(t *testing.T) {
	for _, v := range []uint32{0, 3, 0xFFFFFFFF} {
		slice := v
		_, err := convertRenderPassDescriptorRust(&RenderPassDescriptor{
			ColorAttachments: []RenderPassColorAttachment{{}, {DepthSlice: &slice}},
		})
		if !errors.Is(err, errRustDepthSliceUnmapped) {
			t.Fatalf("DepthSlice %d: error = %v, want %v", v, err, errRustDepthSliceUnmapped)
		}
	}
	if _, err := convertRenderPassDescriptorRust(&RenderPassDescriptor{
		ColorAttachments: []RenderPassColorAttachment{{}},
	}); err != nil {
		t.Fatalf("nil DepthSlice: %v", err)
	}
}
