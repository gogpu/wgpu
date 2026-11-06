//go:build !(js && wasm)

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

// overlapAttachment is a color attachment for the overlap tests: a view,
// an optional depth slice and an optional resolve target.
type overlapAttachment struct {
	view    *TextureView
	slice   *uint32
	resolve *TextureView
}

func overlapPass(atts ...overlapAttachment) *RenderPassDescriptor {
	desc := &RenderPassDescriptor{Label: "overlap"}
	for _, a := range atts {
		desc.ColorAttachments = append(desc.ColorAttachments, RenderPassColorAttachment{
			View: a.view, DepthSlice: a.slice, ResolveTarget: a.resolve,
			LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		})
	}
	return desc
}

// overlapArrayTexture is a 16x16 2D texture with 4 array layers and 2 mips.
func overlapArrayTexture() *Texture {
	return NewTexture(&mockTexture{}, nil,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 4},
		2, 1, "layers")
}

// TestValidateRenderPassDescriptor_SubresourceOverlap covers the WebGPU
// GPURenderPassDescriptor rule that the pass's attachmentRegions (each color
// attachment's view with its depthSlice, and each resolve target) are
// pairwise disjoint, as Rust wgpu-core command/render.rs
// check_attachment_overlap checks it: the key is (texture, base mip level,
// array layer or depth slice).
func TestValidateRenderPassDescriptor_SubresourceOverlap(t *testing.T) {
	volume := depthSliceTexture3D()
	layers := overlapArrayTexture()
	flat := testTexture(gputypes.TextureFormatRGBA8Unorm, 16, 16, 1)
	other := testTexture(gputypes.TextureFormatRGBA8Unorm, 16, 16, 1)

	view3D := &TextureView{HAL: &mockTextureView{}, Parent: volume,
		Dimension: gputypes.TextureViewDimension3D, ArrayLayerCount: 1}
	flatView := &TextureView{HAL: &mockTextureView{}, Parent: flat,
		Dimension: gputypes.TextureViewDimension2D, ArrayLayerCount: 1}
	otherView := &TextureView{HAL: &mockTextureView{}, Parent: other,
		Dimension: gputypes.TextureViewDimension2D, ArrayLayerCount: 1}
	unknownView := &TextureView{HAL: &mockTextureView{}, Parent: flat}
	layer := func(base, count uint32) *TextureView {
		dim := gputypes.TextureViewDimension2DArray
		if count == 1 {
			dim = gputypes.TextureViewDimension2D
		}
		return &TextureView{HAL: &mockTextureView{}, Parent: layers, Dimension: dim,
			BaseArrayLayer: base, ArrayLayerCount: count}
	}
	s := depthSlicePtr

	tests := []struct {
		name    string
		desc    *RenderPassDescriptor
		wantErr bool
		index   int
		resolve bool
		msg     string
	}{
		{name: "3D different slices", desc: overlapPass(
			overlapAttachment{view: view3D, slice: s(0)}, overlapAttachment{view: view3D, slice: s(5)})},
		{name: "3D same slice of the same view", desc: overlapPass(
			overlapAttachment{view: view3D, slice: s(3)}, overlapAttachment{view: view3D, slice: s(3)}),
			wantErr: true, index: 1, msg: "color attachment's view's subresource at mip 0 and depth/array layer 3"},
		{name: "2D two textures", desc: overlapPass(
			overlapAttachment{view: flatView}, overlapAttachment{view: otherView})},
		{name: "2D same view twice", desc: overlapPass(
			overlapAttachment{view: flatView}, overlapAttachment{view: flatView}),
			wantErr: true, index: 1, msg: "color attachment's view's subresource at mip 0 and depth/array layer 0"},
		{name: "2D view and an unknown-range view of one texture", desc: overlapPass(
			overlapAttachment{view: flatView}, overlapAttachment{view: unknownView}),
			wantErr: true, index: 1, msg: "at mip 0 and depth/array layer 0"},
		{name: "array different single layers", desc: overlapPass(
			overlapAttachment{view: layer(0, 1)}, overlapAttachment{view: layer(1, 1)},
			overlapAttachment{view: layer(3, 1)})},
		{name: "array same layer from two views", desc: overlapPass(
			overlapAttachment{view: layer(2, 1)}, overlapAttachment{view: layer(2, 1)}),
			wantErr: true, index: 1, msg: "at mip 0 and depth/array layer 2"},
		{name: "array layer range overlaps a single layer", desc: overlapPass(
			overlapAttachment{view: layer(0, 3)}, overlapAttachment{view: layer(2, 1)}),
			wantErr: true, index: 1, msg: "at mip 0 and depth/array layer 2"},
		{name: "array disjoint layer ranges", desc: overlapPass(
			overlapAttachment{view: layer(0, 2)}, overlapAttachment{view: layer(2, 2)})},
		{name: "resolve target is another attachment's view", desc: overlapPass(
			overlapAttachment{view: flatView}, overlapAttachment{view: otherView, resolve: flatView}),
			wantErr: true, index: 1, resolve: true, msg: "resolve target's subresource at mip 0 and depth/array layer 0"},
		{name: "resolve target is its own attachment's view", desc: overlapPass(
			overlapAttachment{view: flatView, resolve: flatView}),
			wantErr: true, index: 0, resolve: true, msg: "resolve target's subresource at mip 0 and depth/array layer 0"},
		{name: "resolve target on a free layer", desc: overlapPass(
			overlapAttachment{view: otherView, resolve: layer(1, 1)}, overlapAttachment{view: layer(0, 1)})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateRenderPassDescriptor(tt.desc, gputypes.DefaultLimits())
			checkOverlapErr(t, err, tt.wantErr, tt.index, tt.resolve, tt.msg)
		})
	}
}

func checkOverlapErr(t *testing.T, err error, wantErr bool, index int, resolve bool, msg string) {
	t.Helper()
	if !wantErr {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	var rpve *RenderPassValidationError
	if !errors.As(err, &rpve) {
		t.Fatalf("error = %v, want *RenderPassValidationError", err)
	}
	if rpve.Kind != RenderPassErrorSubresourceOverlap || rpve.AttachmentIndex != index || rpve.ResolveTarget != resolve {
		t.Fatalf("kind = %v, attachment = %d, resolve = %v; want SubresourceOverlap, %d, %v",
			rpve.Kind, rpve.AttachmentIndex, rpve.ResolveTarget, index, resolve)
	}
	if !strings.Contains(err.Error(), msg) || !strings.Contains(err.Error(), "is already attached to this render pass") {
		t.Fatalf("error text %q does not contain %q", err.Error(), msg)
	}
}

// TestAttachmentRegions_MipLevelsAreDistinct checks the overlap key directly:
// the same layer or depth slice at another mip level is another subresource.
// A render pass cannot show it, since two mips of one texture have different
// render extents and fail the size rule first.
func TestAttachmentRegions_MipLevelsAreDistinct(t *testing.T) {
	volume := depthSliceTexture3D()
	layers := overlapArrayTexture()
	at := func(tex *Texture, dim gputypes.TextureViewDimension, mip, layer uint32) *TextureView {
		return &TextureView{Parent: tex, Dimension: dim, BaseMipLevel: mip, BaseArrayLayer: layer, ArrayLayerCount: 1}
	}
	regions := attachmentRegions{}
	for _, c := range []struct {
		view  *TextureView
		slice *uint32
	}{
		{at(volume, gputypes.TextureViewDimension3D, 0, 0), depthSlicePtr(1)},
		{at(volume, gputypes.TextureViewDimension3D, 1, 0), depthSlicePtr(1)},
		{at(layers, gputypes.TextureViewDimension2D, 0, 1), nil},
		{at(layers, gputypes.TextureViewDimension2D, 1, 1), nil},
	} {
		if mip, layer, ok := regions.add(c.view, c.slice); !ok {
			t.Fatalf("mip %d layer %d reported as already attached", mip, layer)
		}
	}
	if mip, layer, ok := regions.add(at(layers, gputypes.TextureViewDimension2D, 1, 1), nil); ok || mip != 1 || layer != 1 {
		t.Fatalf("second claim of mip 1 layer 1: ok = %v, mip %d, layer %d", ok, mip, layer)
	}
}
