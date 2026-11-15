//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// renderExtentTexture is a width x height 2D texture with 3 mips.
func renderExtentTexture(format gputypes.TextureFormat, width, height uint32) *Texture {
	return NewTexture(&mockTexture{}, nil, format, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		3, 1, "extent")
}

func renderExtentView(tex *Texture, mip uint32) *TextureView {
	return &TextureView{HAL: &mockTextureView{}, Parent: tex,
		Dimension: gputypes.TextureViewDimension2D, BaseMipLevel: mip, ArrayLayerCount: 1}
}

// TestValidateRenderPassDescriptor_RenderExtent checks that the attachment
// size rule compares render extents, the size of each view's base mip level
// (WebGPU spec "compute render extent": max(1, size >> mipLevel)), as Rust
// wgpu-core compares TextureView render_extent, never the textures' mip 0
// sizes.
func TestValidateRenderPassDescriptor_RenderExtent(t *testing.T) {
	const color, depth = gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatDepth32Float
	big := renderExtentTexture(color, 16, 16)
	small := renderExtentTexture(color, 8, 8)
	odd := renderExtentTexture(color, 15, 9)
	oddHalf := renderExtentTexture(color, 7, 4)
	bigDepth := renderExtentTexture(depth, 32, 32)
	smallDepth := renderExtentTexture(depth, 8, 8)

	pass := func(ds *TextureView, views ...*TextureView) *RenderPassDescriptor {
		desc := &RenderPassDescriptor{Label: "extent"}
		for _, v := range views {
			desc.ColorAttachments = append(desc.ColorAttachments, RenderPassColorAttachment{
				View: v, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			})
		}
		if ds != nil {
			desc.DepthStencilAttachment = &RenderPassDepthStencilAttachment{View: ds}
		}
		return desc
	}
	tests := []struct {
		name          string
		desc          *RenderPassDescriptor
		wantErr       bool
		index         int
		width, height uint32 // the attachment's render extent in the error
	}{
		{name: "16x16 mip 1 with 8x8 mip 0", desc: pass(nil, renderExtentView(big, 1), renderExtentView(small, 0))},
		{name: "8x8 mip 0 with 16x16 mip 1", desc: pass(nil, renderExtentView(small, 0), renderExtentView(big, 1))},
		{name: "15x9 mip 1 with 7x4 mip 0", desc: pass(nil, renderExtentView(odd, 1), renderExtentView(oddHalf, 0))},
		{name: "16x16 mip 0 with 16x16 mip 1", desc: pass(nil, renderExtentView(big, 0), renderExtentView(big, 1)),
			wantErr: true, index: 1, width: 8, height: 8},
		{name: "depth 32x32 mip 2 with color 8x8", desc: pass(renderExtentView(bigDepth, 2), renderExtentView(small, 0))},
		{name: "depth 8x8 with color 16x16 mip 1", desc: pass(renderExtentView(smallDepth, 0), renderExtentView(big, 1))},
		{name: "depth 32x32 mip 1 with color 8x8", desc: pass(renderExtentView(bigDepth, 1), renderExtentView(small, 0)),
			wantErr: true, index: -1, width: 16, height: 16},
		{name: "depth-only 32x32 mip 2", desc: pass(renderExtentView(bigDepth, 2))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateRenderPassDescriptor(tt.desc, gputypes.DefaultLimits())
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var rpve *RenderPassValidationError
			if !errors.As(err, &rpve) || rpve.Kind != RenderPassErrorDimensionMismatch {
				t.Fatalf("error = %v, want DimensionMismatch", err)
			}
			if rpve.AttachmentIndex != tt.index || rpve.ActualWidth != tt.width || rpve.ActualHeight != tt.height {
				t.Fatalf("attachment %d actual %dx%d; want attachment %d actual %dx%d",
					rpve.AttachmentIndex, rpve.ActualWidth, rpve.ActualHeight, tt.index, tt.width, tt.height)
			}
		})
	}
}
