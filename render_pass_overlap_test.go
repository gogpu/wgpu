//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestBeginRenderPassAttachmentOverlap checks through the public API that
// the attachmentRegions of a render pass (each color attachment's view with
// its depthSlice, each resolve target) must be pairwise disjoint (WebGPU
// spec, GPURenderPassDescriptor Valid Usage): different layers of one
// texture or different slices of one 3D view are valid, the same layer or
// slice twice is not. The 2d-array view with the default layer count, and
// the view with the default mip count, check that CreateTextureView resolves
// each count to the rest of the texture: a view of 4 layers or 2 mips is not
// a renderable texture view.
func TestBeginRenderPassAttachmentOverlap(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newTex := func(dim gputypes.TextureDimension, depth, mips uint32) *wgpu.Texture {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         "overlap",
			Size:          wgpu.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: depth},
			MipLevelCount: mips,
			SampleCount:   1,
			Dimension:     dim,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         wgpu.TextureUsageRenderAttachment,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	newView := func(tex *wgpu.Texture, dim gputypes.TextureViewDimension, mips, base, count uint32) *wgpu.TextureView {
		view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
			Dimension: dim, MipLevelCount: mips, BaseArrayLayer: base, ArrayLayerCount: count,
		})
		if err != nil {
			t.Fatalf("CreateTextureView: %v", err)
		}
		t.Cleanup(view.Release)
		return view
	}
	layers := newTex(gputypes.TextureDimension2D, 4, 1)
	volume := newTex(gputypes.TextureDimension3D, 4, 1)
	layer := func(i uint32) *wgpu.TextureView {
		return newView(layers, gputypes.TextureViewDimension2D, 1, i, 1)
	}
	allLayers := newView(layers, gputypes.TextureViewDimension2DArray, 1, 0, 0)
	allMips := newView(newTex(gputypes.TextureDimension2D, 1, 2), gputypes.TextureViewDimension2D, 0, 0, 1)
	vol := newView(volume, gputypes.TextureViewDimension3D, 1, 0, 1)
	slice := func(v uint32) *uint32 { return &v }

	type att struct {
		view  *wgpu.TextureView
		slice *uint32
	}
	tests := []struct {
		name    string
		atts    []att
		wantErr string // empty: BeginRenderPass succeeds
	}{
		{"array-layers-0-and-1", []att{{view: layer(0)}, {view: layer(1)}}, ""},
		{"array-layer-2-twice", []att{{view: layer(2)}, {view: layer(2)}},
			"color attachment [1]: color attachment's view's subresource at mip 0 and depth/array layer 2 is already attached to this render pass"},
		{"array-default-layer-count-is-4", []att{{view: allLayers}},
			"this texture view has more than one array layer; view array layers: 4"},
		{"default-mip-count-is-2", []att{{view: allMips}},
			"this texture view has more than one mipmap level; view mipmap levels: 2"},
		{"3d-slices-1-and-2", []att{{view: vol, slice: slice(1)}, {view: vol, slice: slice(2)}}, ""},
		{"3d-slice-2-twice", []att{{view: vol, slice: slice(2)}, {view: vol, slice: slice(2)}},
			"color attachment [1]: color attachment's view's subresource at mip 0 and depth/array layer 2 is already attached"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := &wgpu.RenderPassDescriptor{}
			for _, a := range tt.atts {
				desc.ColorAttachments = append(desc.ColorAttachments, wgpu.RenderPassColorAttachment{
					View: a.view, DepthSlice: a.slice,
					LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
				})
			}
			checkBeginRenderPass(t, device, desc, tt.wantErr)
		})
	}
}

// checkBeginRenderPass begins and ends a pass with desc and checks that
// BeginRenderPass fails with an error containing wantErr, or succeeds when
// wantErr is empty.
func checkBeginRenderPass(t *testing.T, device *wgpu.Device, desc *wgpu.RenderPassDescriptor, wantErr string) {
	t.Helper()
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	defer enc.DiscardEncoding()
	pass, err := enc.BeginRenderPass(desc)
	if wantErr == "" {
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		return
	}
	if err == nil {
		_ = pass.End()
		t.Fatalf("BeginRenderPass: no error, want %q", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("BeginRenderPass error = %q, want it to contain %q", err, wantErr)
	}
}
