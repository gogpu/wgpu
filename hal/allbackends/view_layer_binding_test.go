// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// layerColor is the texel every texel of array layer j is filled with: a
// distinct value per layer, so a view that reads the wrong layer shows.
func layerColor(j uint32) [4]byte {
	return [4]byte{byte(10 + 20*j), byte(250 - 20*j), byte(j), 255}
}

// newLayeredTexture creates a 4x4 RGBA8 2D texture with `layers` array
// layers and fills layer j with layerColor(j).
func newLayeredTexture(t *testing.T, device *wgpu.Device, layers uint32, usage gputypes.TextureUsage) *wgpu.Texture {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "layered",
		Size:          wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: layers},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         usage | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	texels := make([]byte, usageTargetSize*usageTargetSize*4)
	for j := uint32(0); j < layers; j++ {
		c := layerColor(j)
		for i := 0; i < len(texels); i += 4 {
			copy(texels[i:], c[:])
		}
		if err := device.Queue().WriteTexture(
			&wgpu.ImageCopyTexture{Texture: tex, Origin: wgpu.Origin3D{Z: j}}, texels,
			&wgpu.ImageDataLayout{BytesPerRow: usageTargetSize * 4, RowsPerImage: usageTargetSize},
			&wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("WriteTexture layer %d: %v", j, err)
		}
	}
	return tex
}

func newViewOf(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, dim gputypes.TextureViewDimension, base, count uint32) *wgpu.TextureView {
	t.Helper()
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label: "layer-view", Dimension: dim, MipLevelCount: 1, BaseArrayLayer: base, ArrayLayerCount: count,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return view
}

// TestSampleArrayLayer samples array layer k of a 4-layer texture through a
// 2d view whose baseArrayLayer is k: the shader must read layer k, not layer
// 0. Regression test for the DX12 SRV of a 2D view, which was a plain
// TEXTURE2D and so always addressed layer 0.
func TestSampleArrayLayer(t *testing.T) {
	const layers = 4
	forEachBackendAdapter(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		skipGLESViewDimension(t, b)
		tex := newLayeredTexture(t, device, layers, wgpu.TextureUsageTextureBinding)
		for _, k := range []uint32{0, 2, layers - 1} {
			t.Run(fmt.Sprintf("layer%d", k), func(t *testing.T) {
				skipSoftwareLayer(t, b, k)
				view := newViewOf(t, device, tex, gputypes.TextureViewDimension2D, k, 1)
				enc := newEncoder(t, device)
				target, staging := sampleIntoTarget(t, device, enc, view)
				submit(t, device, enc)
				checkTargetPixel(t, device, target, staging, layerColor(k))
			})
		}
	})
}

const sampleCubeWGSL = `
@group(0) @binding(0) var input: %s;
@group(0) @binding(1) var smp: sampler;

@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    let uv = vec2<f32>(f32((index << 1u) & 2u), f32(index & 2u));
    return vec4<f32>(uv * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    // +X is face 0 of a cube.
    return textureSampleLevel(input, smp, vec3<f32>(1.0, 0.0, 0.0), %s0.0);
}
`

// TestSampleCubeLayer samples face 0 (+X) of a cube view and of cube 0 of a
// cube-array view of a 12-layer texture, at base layer 0 and 6. The sample
// must come from the view's base layer. Regression test for the DX12 SRV of
// a cube view, which ignored its base layer, and of a cube-array view, whose
// First2DArrayFace (an array layer index) was set to the base layer / 6.
func TestSampleCubeLayer(t *testing.T) {
	const layers = 12
	forEachBackendAdapter(t, gputypes.DownlevelFlagsCubeArrayTextures, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		skipGLESViewDimension(t, b)
		tex := newLayeredTexture(t, device, layers, wgpu.TextureUsageTextureBinding)
		for _, tc := range []struct {
			dim   gputypes.TextureViewDimension
			count uint32
		}{
			{gputypes.TextureViewDimensionCube, 6},
			{gputypes.TextureViewDimensionCubeArray, 6},
		} {
			for _, base := range []uint32{0, 6} {
				t.Run(fmt.Sprintf("%s/base%d", tc.dim, base), func(t *testing.T) {
					view := newViewOf(t, device, tex, tc.dim, base, tc.count)
					enc := newEncoder(t, device)
					target, staging := sampleCubeIntoTarget(t, device, enc, view, tc.dim)
					submit(t, device, enc)
					checkTargetPixel(t, device, target, staging, layerColor(base))
				})
			}
		}
	})
}

// sampleCubeIntoTarget is sampleIntoTarget for a cube or cube-array view:
// the fragment shader samples face 0 (of cube 0) with a nearest sampler.
func sampleCubeIntoTarget(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, view *wgpu.TextureView, dim gputypes.TextureViewDimension) (*wgpu.Texture, *wgpu.Buffer) {
	t.Helper()
	target, targetView := newUsageTexture(t, device, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

	bgl := newBindGroupLayout(t, device,
		gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageFragment,
			Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: dim},
		},
		gputypes.BindGroupLayoutEntry{
			Binding: 1, Visibility: wgpu.ShaderStageFragment,
			Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
		})
	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "nearest",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeNearest,
		MinFilter:    gputypes.FilterModeNearest,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	t.Cleanup(sampler.Release)
	group := newBindGroup(t, device, bgl,
		wgpu.BindGroupEntry{Binding: 0, TextureView: view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: sampler})

	texType, arrayIndex := "texture_cube<f32>", ""
	if dim == gputypes.TextureViewDimensionCubeArray {
		texType, arrayIndex = "texture_cube_array<f32>", "0, "
	}
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "sample-cube", WGSL: fmt.Sprintf(sampleCubeWGSL, texType, arrayIndex),
	})
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
			ClearValue: gputypes.Color{R: 1, A: 1},
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
	return target, staging
}

// TestStorageWriteArrayLayer writes array layer k of a 4-layer texture from
// a compute shader through a 2d storage view whose baseArrayLayer is k:
// layer k must hold the write and every other layer its own contents.
func TestStorageWriteArrayLayer(t *testing.T) {
	const layers = 4
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding:    0,
			Visibility: wgpu.ShaderStageCompute,
			StorageTexture: &gputypes.StorageTextureBindingLayout{
				Access:        gputypes.StorageTextureAccessWriteOnly,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				ViewDimension: gputypes.TextureViewDimension2D,
			},
		})
		pipeline := newComputePipeline(t, device, storageWriteWGSL, bgl)
		for _, k := range []uint32{0, 2, layers - 1} {
			t.Run(fmt.Sprintf("layer%d", k), func(t *testing.T) {
				skipSoftwareLayer(t, b, k)
				tex := newLayeredTexture(t, device, layers, wgpu.TextureUsageStorageBinding|wgpu.TextureUsageCopySrc)
				view := newViewOf(t, device, tex, gputypes.TextureViewDimension2D, k, 1)
				group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, TextureView: view})
				enc := newEncoder(t, device)
				pass, err := enc.BeginComputePass(nil)
				if err != nil {
					t.Fatalf("BeginComputePass: %v", err)
				}
				pass.SetPipeline(pipeline)
				pass.SetBindGroup(0, group, nil)
				pass.Dispatch(1, 1, 1)
				if err := pass.End(); err != nil {
					t.Fatalf("End: %v", err)
				}
				submit(t, device, enc)
				for j := uint32(0); j < layers; j++ {
					want := layerColor(j)
					if j == k {
						want = [4]byte{0, 255, 0, 255}
					}
					checkLayerTexel(t, device, tex, j, want)
				}
			})
		}
	})
}

// checkLayerTexel copies array layer j of tex back and checks texel (1, 1).
func checkLayerTexel(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, j uint32, want [4]byte) {
	t.Helper()
	staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	enc := newEncoder(t, device)
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: usageTargetSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex, Origin: wgpu.Origin3D{Z: j}},
		Size:         wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
	}})
	submit(t, device, enc)
	words := readPublicBuffer(t, staging, 256*usageTargetSize/4)
	var got [4]byte
	binary.LittleEndian.PutUint32(got[:], words[(256+4)/4])
	if got != want {
		t.Errorf("layer %d texel (1, 1) = %v, want %v", j, got, want)
	}
}

// skipSoftwareLayer skips a binding of array layer k > 0 on the software
// backend, which reads and writes layer 0 for every view.
func skipSoftwareLayer(t *testing.T, b overrideBackend, k uint32) {
	t.Helper()
	if b.software && k > 0 {
		t.Skip("software: binding an array layer above 0 is P1-SW-026")
	}
}

// skipGLESViewDimension skips GLES, which has no texture views: a texture's
// GL target is fixed at creation from its size (a 2D texture with several
// layers is TEXTURE_2D_ARRAY), and a view whose dimension needs another
// target (a 2d view of one layer, a cube view of a 12-layer texture) cannot
// be sampled. Rust wgpu-hal gles has the same limit (gfx-rs/wgpu#1574).
func skipGLESViewDimension(t *testing.T, b overrideBackend) {
	t.Helper()
	if b.name == "gles" {
		t.Skip("gles: no texture views, a view dimension other than the texture's GL target cannot be sampled")
	}
}
