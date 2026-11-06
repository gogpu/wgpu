//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestBeginRenderPassRenderExtent checks through the public API that the
// attachment size rule compares the size of each view's mip level (WebGPU
// spec "compute render extent"), not the textures' mip 0 sizes: mip 1 of a
// 16x16 texture pairs with an 8x8 texture, mip 0 and mip 1 of one texture
// do not.
func TestBeginRenderPassRenderExtent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newTex := func(size, mips uint32, format gputypes.TextureFormat) *wgpu.Texture {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         "extent",
			Size:          wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
			MipLevelCount: mips,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Format:        format,
			Usage:         wgpu.TextureUsageRenderAttachment,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	newView := func(tex *wgpu.Texture, mip uint32) *wgpu.TextureView {
		view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
			BaseMipLevel: mip, MipLevelCount: 1, ArrayLayerCount: 1,
		})
		if err != nil {
			t.Fatalf("CreateTextureView: %v", err)
		}
		t.Cleanup(view.Release)
		return view
	}
	big := newTex(16, 2, gputypes.TextureFormatRGBA8Unorm)
	small := newTex(8, 1, gputypes.TextureFormatRGBA8Unorm)
	depth := newTex(16, 2, gputypes.TextureFormatDepth32Float)

	pass := func(ds *wgpu.TextureView, views ...*wgpu.TextureView) *wgpu.RenderPassDescriptor {
		desc := &wgpu.RenderPassDescriptor{}
		for _, v := range views {
			desc.ColorAttachments = append(desc.ColorAttachments, wgpu.RenderPassColorAttachment{
				View: v, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			})
		}
		if ds != nil {
			desc.DepthStencilAttachment = &wgpu.RenderPassDepthStencilAttachment{
				View: ds, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
			}
		}
		return desc
	}
	tests := []struct {
		name    string
		desc    *wgpu.RenderPassDescriptor
		wantErr string
	}{
		{"mip1-of-16-with-8", pass(nil, newView(big, 1), newView(small, 0)), ""},
		{"depth-mip1-with-8", pass(newView(depth, 1), newView(small, 0)), ""},
		{"mip0-with-mip1", pass(nil, newView(big, 0), newView(big, 1)),
			"color attachment [1] has dimensions 8x8, but attachment [0] has dimensions 16x16"},
		{"depth-mip0-with-8", pass(newView(depth, 0), newView(small, 0)),
			"has dimensions 16x16, but attachment [0] has dimensions 8x8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkBeginRenderPass(t, device, tt.desc, tt.wantErr)
		})
	}
}
