// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The sRGB view tests reinterpret an RGBA8Unorm texture through an
// RGBA8UnormSrgb view (texture viewFormats). WebGPU converts by the view's
// format: rendering through the view encodes to sRGB, sampling through it
// decodes before filtering, and a copy of the texture moves the stored bytes
// unchanged.

// TestSrgbViewRender renders through an sRGB view of an RGBA8Unorm texture.
// The stored bytes are the sRGB encoding of the fragment value. The software
// backend wrote attachments in the texture's format, so the view's encoding
// was lost.
func TestSrgbViewRender(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsViewFormats, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		srgb := gputypes.TextureFormatRGBA8UnormSrgb
		tex, _ := newColorTexture(t, device, gputypes.TextureFormatRGBA8Unorm, []gputypes.TextureFormat{srgb},
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		colorPass{
			view:  newFormatView(t, device, tex, srgb),
			wgsl:  colorQuadShader("", "vec4<f32>", unormValue),
			state: gputypes.ColorTargetState{Format: srgb, WriteMask: gputypes.ColorWriteMaskAll},
		}.run(t, device)
		want := []byte{124, 170, 203, 255}
		for i, got := range readColorTexels(t, device, tex, 4) {
			if !bytesNear(got, want, 1) {
				t.Fatalf("texel %d = %v, want %v (sRGB-encoded through the view)", i, got, want)
			}
		}
	})
}

// srgbSampleWGSL samples the 2x1 texture halfway between its two texels with
// a linear sampler and writes the result to an RGBA8Unorm target.
const srgbSampleWGSL = `
@group(0) @binding(0) var src: texture_2d<f32>;
@group(0) @binding(1) var smp: sampler;
`

// TestSrgbViewSample samples an RGBA8Unorm texture through an sRGB view with
// linear filtering, halfway between a black texel and a texel of codes
// (188, 255, 94). WebGPU decodes each texel before filtering, so the result
// is the mean of the decoded values, not the decoding of the mean code. The
// software backend sampled in the texture's format and returned the raw
// codes' mean.
func TestSrgbViewSample(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsViewFormats, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		srgb := gputypes.TextureFormatRGBA8UnormSrgb
		src, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         "srgb-view-source",
			Size:          wgpu.Extent3D{Width: 2, Height: 1, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
			ViewFormats:   []gputypes.TextureFormat{srgb},
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(src.Release)
		texels := []byte{0, 0, 0, 255, 188, 255, 94, 255}
		if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: src}, texels,
			&wgpu.ImageDataLayout{BytesPerRow: 8, RowsPerImage: 1},
			&wgpu.Extent3D{Width: 2, Height: 1, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("WriteTexture: %v", err)
		}

		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{
				Binding: 0, Visibility: wgpu.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D},
			},
			gputypes.BindGroupLayoutEntry{
				Binding: 1, Visibility: wgpu.ShaderStageFragment,
				Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
			})
		sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
			Label:        "linear",
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MagFilter:    gputypes.FilterModeLinear,
			MinFilter:    gputypes.FilterModeLinear,
		})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		t.Cleanup(sampler.Release)
		group := newBindGroup(t, device, bgl,
			wgpu.BindGroupEntry{Binding: 0, TextureView: newFormatView(t, device, src, srgb)},
			wgpu.BindGroupEntry{Binding: 1, Sampler: sampler})

		target, targetView := newColorTexture(t, device, gputypes.TextureFormatRGBA8Unorm, nil,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		colorPass{
			view:   targetView,
			wgsl:   colorQuadShader(srgbSampleWGSL, "vec4<f32>", "textureSampleLevel(src, smp, vec2<f32>(0.5, 0.5), 0.0)"),
			state:  gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			layout: bgl,
			group:  group,
		}.run(t, device)

		half := func(code byte) byte { return byte(math.Round(255 * srgbDecode(code) / 2)) }
		want := []byte{half(188), half(255), half(94), 255}
		for i, got := range readColorTexels(t, device, target, 4) {
			if !bytesNear(got, want, 2) {
				t.Fatalf("texel %d = %v, want %v (decoded before filtering)", i, got, want)
			}
		}
	})
}
