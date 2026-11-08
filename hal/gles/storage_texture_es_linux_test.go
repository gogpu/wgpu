// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && linux && !(js && wasm) && wgpu_gles_force_es

package gles

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// esStorageWriteWGSL writes green to a storage texture from compute.
const esStorageWriteWGSL = `
@group(0) @binding(0) var img: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    textureStore(img, vec2<i32>(id.xy), vec4<f32>(0.0, 1.0, 0.0, 1.0));
}`

// TestESStorageTextureIsWritten stores to a storage texture from compute and
// copies it back.
//
// It failed on OpenGL ES with GL_INVALID_OPERATION from glBindImageTexture: ES
// rejects an image binding of a texture with mutable (glTexImage2D) storage, so
// the image was never bound and the shader stored nowhere. Textures are now
// allocated with glTexStorage2D when the context has immutable storage, as Rust
// wgpu-hal does (gles/device.rs create_texture, the TEXTURE_STORAGE capability),
// and image stores and loads work.
func TestESStorageTextureIsWritten(t *testing.T) {
	_, dev, queue := openForcedESDevice(t)

	const size = 8
	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Label: "storage", Size: hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageStorageBinding | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(func() { dev.DestroyTexture(tex) })
	view, err := dev.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}

	module, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "store", Source: hal.ShaderSource{WGSL: esStorageWriteWGSL}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(func() { dev.DestroyShaderModule(module) })
	bgl, err := dev.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{Label: "store", Entries: []gputypes.BindGroupLayoutEntry{{
		Binding: 0, Visibility: gputypes.ShaderStageCompute,
		StorageTexture: &gputypes.StorageTextureBindingLayout{
			Access: gputypes.StorageTextureAccessWriteOnly, Format: gputypes.TextureFormatRGBA8Unorm,
			ViewDimension: gputypes.TextureViewDimension2D,
		},
	}}})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{Label: "store", Layout: bgl, Entries: []gputypes.BindGroupEntry{{
		Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()},
	}}})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	pl, err := dev.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{Label: "store", BindGroupLayouts: []hal.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	pipe, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{Label: "store", Layout: pl, Compute: hal.ComputeState{Module: module, EntryPoint: "main"}})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	staging, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: "staging", Size: 256 * 4, Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	t.Cleanup(func() { dev.DestroyBuffer(staging) })

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "store"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := enc.BeginEncoding("store"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "store"})
	pass.SetPipeline(pipe)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(2, 2, 1)
	pass.End()
	enc.CopyTextureToBuffer(tex, staging, []hal.BufferTextureCopy{{
		TextureBase:  hal.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectAll},
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		Size:         hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
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
	m, err := dev.MapBuffer(staging, 0, 256*4)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	out := make([]byte, 32)
	copy(out, unsafe.Slice((*byte)(m.Ptr), len(out)))
	if err := dev.UnmapBuffer(staging); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}
	if out[0] != 0 || out[1] != 255 || out[2] != 0 || out[3] != 255 {
		t.Fatalf("texel (0,0) = % x, want 00 ff 00 ff", out[:4])
	}
}
