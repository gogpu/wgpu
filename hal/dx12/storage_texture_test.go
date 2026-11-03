// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

const storageTextureWriteWGSL = `
@group(0) @binding(0) var dst: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    textureStore(dst, vec2<i32>(id.xy), vec4<f32>(0.0, 1.0, 0.0, 1.0));
}
`

// TestStorageTextureWriteReadback writes every texel of a write-only storage
// texture from a dispatch and reads the texture back. The shader declares the
// binding as a UAV (u register), so the bind group must hold a UAV of the
// view: WARP reads an SRV descriptor in that slot as no resource and the
// texture stays zero, and GPU-based validation reports "Descriptor type
// doesn't match shader register type" (#939).
func TestStorageTextureWriteReadback(t *testing.T) {
	forEachDX12TestDevice(t, func(t *testing.T, d dx12TestDevice) {
		const size = 4
		const bytesPerRow = 256 // D3D12_TEXTURE_DATA_PITCH_ALIGNMENT
		dev, queue := d.device, d.queue

		tex, err := dev.CreateTexture(&hal.TextureDescriptor{
			Label:         "storage",
			Size:          hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         gputypes.TextureUsageStorageBinding | gputypes.TextureUsageCopySrc,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(func() { dev.DestroyTexture(tex) })
		view, err := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{Label: "storage"})
		if err != nil {
			t.Fatalf("CreateTextureView: %v", err)
		}
		t.Cleanup(func() { dev.DestroyTextureView(view) })

		staging, err := dev.CreateBuffer(&hal.BufferDescriptor{
			Label: "staging", Size: bytesPerRow * size,
			Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
		})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		t.Cleanup(func() { dev.DestroyBuffer(staging) })

		pipeline, group := createStorageTexturePipeline(t, dev, view)

		enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "storage-texture"})
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		if err := enc.BeginEncoding("storage-texture"); err != nil {
			t.Fatalf("BeginEncoding: %v", err)
		}
		enc.TransitionTextures([]hal.TextureBarrier{{
			Texture: tex,
			Usage:   hal.TextureUsageTransition{NewUsage: gputypes.TextureUsageStorageBinding},
		}})
		pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "storage-texture"})
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, group, nil)
		pass.Dispatch(size, size, 1)
		pass.End()
		enc.TransitionTextures([]hal.TextureBarrier{{
			Texture: tex,
			Usage: hal.TextureUsageTransition{
				OldUsage: gputypes.TextureUsageStorageBinding,
				NewUsage: gputypes.TextureUsageCopySrc,
			},
		}})
		enc.CopyTextureToBuffer(tex, staging, []hal.BufferTextureCopy{{
			BufferLayout: hal.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: size},
			TextureBase:  hal.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectAll},
			Size:         hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		}})
		cmd, err := enc.EndEncoding()
		if err != nil {
			t.Fatalf("EndEncoding: %v", err)
		}
		if _, err := queue.Submit([]hal.CommandBuffer{cmd}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if err := dev.WaitIdle(); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}

		m, err := dev.MapBuffer(staging, 0, bytesPerRow*size)
		if err != nil {
			t.Fatalf("MapBuffer: %v", err)
		}
		got := make([]byte, bytesPerRow*size)
		copy(got, unsafe.Slice((*byte)(m.Ptr), len(got)))
		if err := dev.UnmapBuffer(staging); err != nil {
			t.Fatalf("UnmapBuffer: %v", err)
		}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				texel := got[y*bytesPerRow+x*4:][:4]
				if [4]byte(texel) != [4]byte{0, 255, 0, 255} {
					t.Fatalf("texel (%d, %d) = %v, want [0 255 0 255]", x, y, texel)
				}
			}
		}
	})
}

func createStorageTexturePipeline(t *testing.T, dev hal.Device, view hal.TextureView) (hal.ComputePipeline, hal.BindGroup) {
	t.Helper()
	module, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "storage-texture", Source: hal.ShaderSource{WGSL: storageTextureWriteWGSL},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(func() { dev.DestroyShaderModule(module) })

	bgl, err := dev.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "storage-texture",
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: gputypes.ShaderStageCompute,
			StorageTexture: &gputypes.StorageTextureBindingLayout{
				Access:        gputypes.StorageTextureAccessWriteOnly,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				ViewDimension: gputypes.TextureViewDimension2D,
			},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(func() { dev.DestroyBindGroupLayout(bgl) })

	group, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:   "storage-texture",
		Layout:  bgl,
		Entries: []gputypes.BindGroupEntry{{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()}}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(func() { dev.DestroyBindGroup(group) })

	layout, err := dev.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "storage-texture", BindGroupLayouts: []hal.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(func() { dev.DestroyPipelineLayout(layout) })

	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "storage-texture", Layout: layout,
		Compute: hal.ComputeState{Module: module, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(func() { dev.DestroyComputePipeline(pipeline) })
	return pipeline, group
}
