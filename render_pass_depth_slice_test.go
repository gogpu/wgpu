//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestBeginRenderPassDepthSliceValidation checks the depthSlice rules of the
// WebGPU spec (GPURenderPassColorAttachment Valid Usage) through the public
// API: a 3D view needs a slice below the depth of its mip level, any other
// view must not have one. A non-nil 0xFFFFFFFF (WGPU_DEPTH_SLICE_UNDEFINED in
// webgpu.h) is a value like any other, never "undefined".
func TestBeginRenderPassDepthSliceValidation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newTex := func(dim gputypes.TextureDimension, depth uint32) *wgpu.Texture {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         "depth-slice",
			Size:          wgpu.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: depth},
			MipLevelCount: 3,
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
	tex3D := newTex(gputypes.TextureDimension3D, 8)
	tex2D := newTex(gputypes.TextureDimension2D, 1)
	slice := func(v uint32) *uint32 { return &v }

	tests := []struct {
		name    string
		view    *wgpu.TextureView
		slice   *uint32
		wantErr string // empty: BeginRenderPass succeeds
	}{
		{"3d-mip0-slice0", newView(tex3D, 0), slice(0), ""},
		{"3d-mip0-last-slice", newView(tex3D, 0), slice(7), ""},
		{"3d-mip1-last-slice", newView(tex3D, 1), slice(3), ""},
		{"3d-missing-slice", newView(tex3D, 0), nil, "requires depth slice to be provided"},
		{"3d-mip0-slice-at-depth", newView(tex3D, 0), slice(8), "depth slice must be less than 8 but is 8"},
		{"3d-mip2-slice-at-depth", newView(tex3D, 2), slice(2), "depth slice must be less than 2 but is 2"},
		{"3d-undefined-sentinel", newView(tex3D, 0), slice(0xFFFFFFFF), "depth slice must be less than 8 but is 4294967295"},
		{"2d-no-slice", newView(tex2D, 0), nil, ""},
		{"2d-with-slice", newView(tex2D, 0), slice(0), "depth slice was provided but the color attachment's view is not 3D"},
		{"2d-undefined-sentinel", newView(tex2D, 0), slice(0xFFFFFFFF), "depth slice was provided but the color attachment's view is not 3D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			defer enc.DiscardEncoding()
			pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
				ColorAttachments: []wgpu.RenderPassColorAttachment{{
					View:       tt.view,
					DepthSlice: tt.slice,
					LoadOp:     gputypes.LoadOpClear,
					StoreOp:    gputypes.StoreOpStore,
				}},
			})
			if tt.wantErr == "" {
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
				t.Fatalf("BeginRenderPass: no error, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("BeginRenderPass error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
