// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// mipTextureSize is the width and height of mip level 0 of the texture
// TestSampleMipLevelView samples: level 2 is 4x4, so texel (1, 1) exists at
// every level the test reads.
const mipTextureSize = 16

// newMipTexture creates a 16x16 RGBA8 texture with 3 mip levels and fills
// level k with layerColor(k).
func newMipTexture(t *testing.T, device *wgpu.Device) *wgpu.Texture {
	t.Helper()
	const levels = 3
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "mips",
		Size:          wgpu.Extent3D{Width: mipTextureSize, Height: mipTextureSize, DepthOrArrayLayers: 1},
		MipLevelCount: levels,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage: wgpu.TextureUsageTextureBinding | wgpu.TextureUsageRenderAttachment |
			wgpu.TextureUsageCopySrc | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	for k := uint32(0); k < levels; k++ {
		size := uint32(mipTextureSize) >> k
		c := layerColor(k)
		texels := make([]byte, size*size*4)
		for i := 0; i < len(texels); i += 4 {
			copy(texels[i:], c[:])
		}
		if err := device.Queue().WriteTexture(
			&wgpu.ImageCopyTexture{Texture: tex, MipLevel: k}, texels,
			&wgpu.ImageDataLayout{BytesPerRow: size * 4, RowsPerImage: size},
			&wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("WriteTexture level %d: %v", k, err)
		}
	}
	return tex
}

func newMipView(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, base uint32) *wgpu.TextureView {
	t.Helper()
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label: "mip-view", Dimension: gputypes.TextureViewDimension2D, BaseMipLevel: base, MipLevelCount: 1,
		ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return view
}

// TestSampleMipLevelView reads level 0 of a view whose base mip level is k:
// the shader must read mip level k of the texture. It then renders into
// level 0 of the same texture and reads it back. Regression test for GLES,
// which bound the texture without its view's mip range (Rust wgpu-hal sets
// TEXTURE_BASE_LEVEL and TEXTURE_MAX_LEVEL at every bind, gles/queue.rs
// C::BindTexture), so every view sampled level 0. The render into level 0
// after a bind of level 2 checks that the per-bind level range does not leave
// level 0 unusable as a color attachment.
func TestSampleMipLevelView(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		tex := newMipTexture(t, device)
		for _, k := range []uint32{0, 1, 2} {
			t.Run(fmt.Sprintf("mip%d", k), func(t *testing.T) {
				view := newMipView(t, device, tex, k)
				enc := newEncoder(t, device)
				target, staging := sampleIntoTarget(t, device, enc, view)
				submit(t, device, enc)
				checkTargetPixel(t, device, target, staging, layerColor(k))
			})
		}
		t.Run("render-to-mip0-after-mip2", func(t *testing.T) {
			enc := newEncoder(t, device)
			sampleIntoTarget(t, device, enc, newMipView(t, device, tex, 2))
			pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
				ColorAttachments: []wgpu.RenderPassColorAttachment{{
					View: newMipView(t, device, tex, 0), LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					ClearValue: gputypes.Color{G: 1, A: 1},
				}},
			})
			if err != nil {
				t.Fatalf("BeginRenderPass: %v", err)
			}
			if err := pass.End(); err != nil {
				t.Fatalf("RenderPass.End: %v", err)
			}
			staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
			enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
				BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: usageTargetSize},
				TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
				Size:         wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
			}})
			submit(t, device, enc)
			checkTargetPixel(t, device, nil, staging, [4]byte{0, 255, 0, 255})
		})
	})
}

// TestSampleCubeArrayView samples face 0 (+X) of every cube of a cube-array
// view of all 12 layers of a square texture: cube c must read layer 6*c. On
// GLES the texture is a TEXTURE_CUBE_MAP_ARRAY (a cube-compatible 2D texture
// with a multiple of 6 layers other than 6, as in Rust wgpu-hal
// get_info_from_desc). Regression test for GLES, which advertised
// DownlevelFlagsCubeArrayTextures but created such textures as 2D arrays, so
// no cube-array view could be sampled.
func TestSampleCubeArrayView(t *testing.T) {
	const layers = 12
	forEachBackendAdapter(t, gputypes.DownlevelFlagsCubeArrayTextures, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		tex := newLayeredTexture(t, device, layers, wgpu.TextureUsageTextureBinding)
		view := newViewOf(t, device, tex, gputypes.TextureViewDimensionCubeArray, 0, layers)
		for _, cube := range []uint32{0, 1} {
			t.Run(fmt.Sprintf("cube%d", cube), func(t *testing.T) {
				enc := newEncoder(t, device)
				target, staging := sampleCubeIntoTarget(t, device, enc, view, gputypes.TextureViewDimensionCubeArray, cube)
				submit(t, device, enc)
				checkTargetPixel(t, device, target, staging, layerColor(6*cube))
			})
		}
	})
}

const sampleStencilWGSL = `
@group(0) @binding(0) var input: texture_2d<u32>;

@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    let uv = vec2<f32>(f32((index << 1u) & 2u), f32(index & 2u));
    return vec4<f32>(uv * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    let s = textureLoad(input, vec2<i32>(position.xy), 0).r;
    return vec4<f32>(f32(s) / 255.0, 0.0, 0.0, 1.0);
}
`

const sampleDepthWGSL = `
@group(0) @binding(0) var input: texture_depth_2d;

@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    let uv = vec2<f32>(f32((index << 1u) & 2u), f32(index & 2u));
    return vec4<f32>(uv * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(textureLoad(input, vec2<i32>(position.xy), 0), 0.0, 0.0, 1.0);
}
`

// TestSampleDepthStencilAspectView clears a Depth24PlusStencil8 attachment to
// depth 0.5 and stencil 90, then samples its stencil-only view, its
// depth-only view and its stencil-only view again: each must read its own
// aspect. Regression test for GLES, which bound the texture without
// DEPTH_STENCIL_TEXTURE_MODE, so a stencil-only view sampled the depth (Rust
// wgpu-hal gles/queue.rs C::BindTexture sets it on ES 3.1 and GL 4.3). The
// depth-only bind after a stencil-only one checks that the mode is set at
// every bind, not left from the bind before. DX12 sampled 0 from a
// stencil-only view until its SRV swizzled the stencil from green to red.
func TestSampleDepthStencilAspectView(t *testing.T) {
	const stencil = 90
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		ds, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         "depth-stencil",
			Size:          wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Format:        gputypes.TextureFormatDepth24PlusStencil8,
			Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(ds.Release)
		aspectView := func(aspect gputypes.TextureAspect) *wgpu.TextureView {
			view, err := device.CreateTextureView(ds, &wgpu.TextureViewDescriptor{
				Label: "aspect", Dimension: gputypes.TextureViewDimension2D, Aspect: aspect,
				MipLevelCount: 1, ArrayLayerCount: 1,
			})
			if err != nil {
				t.Fatalf("CreateTextureView: %v", err)
			}
			t.Cleanup(view.Release)
			return view
		}

		enc := newEncoder(t, device)
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
				View:        aspectView(gputypes.TextureAspectAll),
				DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 0.5,
				StencilLoadOp: gputypes.LoadOpClear, StencilStoreOp: gputypes.StoreOpStore, StencilClearValue: stencil,
			},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("RenderPass.End: %v", err)
		}
		stencilView, depthView := aspectView(gputypes.TextureAspectStencilOnly), aspectView(gputypes.TextureAspectDepthOnly)
		stagings := make([]*wgpu.Buffer, 0, 3)
		for _, s := range []struct {
			view       *wgpu.TextureView
			sampleType gputypes.TextureSampleType
			wgsl       string
		}{
			{stencilView, gputypes.TextureSampleTypeUint, sampleStencilWGSL},
			{depthView, gputypes.TextureSampleTypeDepth, sampleDepthWGSL},
			{stencilView, gputypes.TextureSampleTypeUint, sampleStencilWGSL},
		} {
			stagings = append(stagings, sampleAspectIntoTarget(t, device, enc, s.view, s.sampleType, s.wgsl))
		}
		submit(t, device, enc)
		for i, want := range []byte{stencil, 128, stencil} {
			words := readPublicBuffer(t, stagings[i], 256*usageTargetSize/4)
			if got := byte(words[(256+4)/4]); got < want-1 || got > want+1 {
				t.Errorf("sample %d: red of texel (1, 1) = %d, want %d", i, got, want)
			}
		}
	})
}

// sampleAspectIntoTarget is sampleIntoTarget with the bind group layout's
// sample type and the shader given: it renders wgsl's fs_main, which reads
// view, into a new RGBA8 target and copies the target into the returned
// staging buffer.
func sampleAspectIntoTarget(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, view *wgpu.TextureView, sampleType gputypes.TextureSampleType, wgsl string) *wgpu.Buffer {
	t.Helper()
	target, targetView := newUsageTexture(t, device, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
		Binding:    0,
		Visibility: wgpu.ShaderStageFragment,
		Texture:    &gputypes.TextureBindingLayout{SampleType: sampleType, ViewDimension: gputypes.TextureViewDimension2D},
	})
	group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, TextureView: view})
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "sample-aspect", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout:      layout,
		Vertex:      wgpu.VertexState{Module: shader, EntryPoint: "vs_main"},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.DefaultMultisampleState(),
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: usageTargetSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
	}})
	return staging
}
