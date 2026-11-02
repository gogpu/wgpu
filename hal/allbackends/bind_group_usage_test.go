// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package allbackends_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// These tests read back resources a shader reaches only through a bind group,
// after a queue write or an earlier pass of the same encoder. The resource gets
// its barrier only if the bind group's usage enters the pass usage scope, as
// Rust wgpu-core's UsageScope::merge_bind_group does (P1-CORE-010). On Vulkan
// the synchronization validation layer runs and every error fails the test.

const doubleWGSL = `
@group(0) @binding(0) var<storage, read> input: array<u32, 64>;
@group(0) @binding(1) var<storage, read_write> output: array<u32, 64>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    output[id.x] = input[id.x] * 2u + 1u;
}
`

// TestWriteBufferThenDispatchReadsIt: Queue.WriteBuffer fills a storage
// buffer that a dispatch then reads through a read-only storage binding. The
// write's COPY_DST must become STORAGE_READ before the dispatch.
func TestWriteBufferThenDispatchReadsIt(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		const n = 64
		input := newPublicBuffer(t, device, n*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst)
		output := newPublicBuffer(t, device, n*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc)
		staging := newPublicBuffer(t, device, n*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		)
		pipeline := newComputePipeline(t, device, doubleWGSL, bgl)
		group := newBindGroup(t, device, bgl,
			wgpu.BindGroupEntry{Binding: 0, Buffer: input, Size: n * 4},
			wgpu.BindGroupEntry{Binding: 1, Buffer: output, Size: n * 4},
		)

		data := make([]byte, n*4)
		for i := range n {
			binary.LittleEndian.PutUint32(data[i*4:], uint32(i+10))
		}
		if err := device.Queue().WriteBuffer(input, 0, data); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}

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
		enc.CopyBufferToBuffer(output, 0, staging, 0, n*4)
		submit(t, device, enc)

		for i, got := range readPublicBuffer(t, staging, n) {
			if want := uint32(i+10)*2 + 1; got != want {
				t.Fatalf("output[%d] = %d, want %d", i, got, want)
			}
		}
	})
}

const sampleWGSL = `
@group(0) @binding(0) var input: texture_2d<f32>;

@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    let uv = vec2<f32>(f32((index << 1u) & 2u), f32(index & 2u));
    return vec4<f32>(uv * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    return textureLoad(input, vec2<i32>(position.xy), 0);
}
`

const storageWriteWGSL = `
@group(0) @binding(0) var output: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(4, 4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    textureStore(output, vec2<i32>(id.xy), vec4<f32>(0.0, 1.0, 0.0, 1.0));
}
`

const usageTargetSize = 4

// TestWriteTextureThenSample: Queue.WriteTexture fills a texture that a
// render pass then samples through a bind group. The write's COPY_DST must
// become RESOURCE before the draw, from the bind group's usage rather than an
// eager barrier at the end of the pending writes.
func TestWriteTextureThenSample(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		tex, view := newUsageTexture(t, device, wgpu.TextureUsageTextureBinding|wgpu.TextureUsageCopyDst)
		texels := make([]byte, usageTargetSize*usageTargetSize*4)
		for i := 0; i < len(texels); i += 4 {
			copy(texels[i:], []byte{10, 200, 30, 255})
		}
		if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, texels,
			&wgpu.ImageDataLayout{BytesPerRow: usageTargetSize * 4, RowsPerImage: usageTargetSize},
			&wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("WriteTexture: %v", err)
		}

		enc := newEncoder(t, device)
		target, staging := sampleIntoTarget(t, device, enc, view)
		submit(t, device, enc)
		checkTargetPixel(t, device, target, staging, [4]byte{10, 200, 30, 255})
	})
}

// TestStorageTextureWriteThenSample: a dispatch writes a storage texture and
// a later render pass of the same encoder samples it. The texture must leave
// UNDEFINED before the dispatch and move from STORAGE_WRITE to RESOURCE
// before the draw, inside the encoder.
func TestStorageTextureWriteThenSample(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		skipKnownGap(t, "gles", "P1-GLES-017", "textureStore to a storage texture reads back zeros")
		skipKnownGap(t, "software", "P1-SW-018", "textureStore to a storage texture reads back zeros")
		_, view := newUsageTexture(t, device, wgpu.TextureUsageStorageBinding|wgpu.TextureUsageTextureBinding)
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
		target, staging := sampleIntoTarget(t, device, enc, view)
		submit(t, device, enc)
		checkTargetPixel(t, device, target, staging, [4]byte{0, 255, 0, 255})
	})
}

// sampleIntoTarget records a render pass that copies view into a new target
// texture with textureLoad, then a copy of the target into a staging buffer.
func sampleIntoTarget(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, view *wgpu.TextureView) (*wgpu.Texture, *wgpu.Buffer) {
	t.Helper()
	target, targetView := newUsageTexture(t, device, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

	bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
		Binding:    0,
		Visibility: wgpu.ShaderStageFragment,
		Texture: &gputypes.TextureBindingLayout{
			SampleType:    gputypes.TextureSampleTypeFloat,
			ViewDimension: gputypes.TextureViewDimension2D,
		},
	})
	group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, TextureView: view})
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "sample", WGSL: sampleWGSL})
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

// checkTargetPixel reads texel (1, 1) of the staging copy of target.
func checkTargetPixel(t *testing.T, _ *wgpu.Device, _ *wgpu.Texture, staging *wgpu.Buffer, want [4]byte) {
	t.Helper()
	words := readPublicBuffer(t, staging, 256*usageTargetSize/4)
	var got [4]byte
	binary.LittleEndian.PutUint32(got[:], words[(256+4)/4])
	if got != want {
		t.Errorf("texel (1, 1) = %v, want %v", got, want)
	}
}

func newUsageTexture(t *testing.T, device *wgpu.Device, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return tex, view
}

func newBindGroupLayout(t *testing.T, device *wgpu.Device, entries ...gputypes.BindGroupLayoutEntry) *wgpu.BindGroupLayout {
	t.Helper()
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(bgl.Release)
	return bgl
}

func newBindGroup(t *testing.T, device *wgpu.Device, bgl *wgpu.BindGroupLayout, entries ...wgpu.BindGroupEntry) *wgpu.BindGroup {
	t.Helper()
	group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl, Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(group.Release)
	return group
}

func newComputePipeline(t *testing.T, device *wgpu.Device, wgsl string, bgl *wgpu.BindGroupLayout) *wgpu.ComputePipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Layout: layout, Module: shader, EntryPoint: "main"})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

func newEncoder(t *testing.T, device *wgpu.Device) *wgpu.CommandEncoder {
	t.Helper()
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	return enc
}

func submit(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder) {
	t.Helper()
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

const drawFromBuffersWGSL = `
struct Color { value: vec4<f32> }
@group(0) @binding(0) var<uniform> color: Color;

@vertex
fn vs_main(@location(0) position: vec2<f32>) -> @builtin(position) vec4<f32> {
    return vec4<f32>(position, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return color.value;
}
`

// TestWriteBufferThenDrawReadsIt: Queue.WriteBuffer fills a vertex buffer, a
// uniform buffer and an indirect argument buffer that one indirect draw then
// reads. Each write's COPY_DST must become VERTEX, UNIFORM and INDIRECT before
// the render pass, from the pass usage scope rather than an eager barrier at
// the end of the pending writes.
func TestWriteBufferThenDrawReadsIt(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsIndirectExecution, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		skipKnownGap(t, "gles", "P1-GLES-016", "DrawIndirect draws nothing")
		vertices := newPublicBuffer(t, device, 6*4, wgpu.BufferUsageVertex|wgpu.BufferUsageCopyDst)
		uniform := newPublicBuffer(t, device, 16, wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
		indirect := newPublicBuffer(t, device, 16, wgpu.BufferUsageIndirect|wgpu.BufferUsageCopyDst)
		writeWords(t, device, vertices, mathFloatBits(-1, -1, 3, -1, -1, 3)...)
		writeWords(t, device, uniform, mathFloatBits(0.2, 0.4, 0.6, 1)...)
		writeWords(t, device, indirect, 3, 1, 0, 0)

		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageFragment,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
		})
		group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, Buffer: uniform, Size: 16})
		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: drawFromBuffersWGSL})
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
			Layout: layout,
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs_main", Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: 8,
				StepMode:    gputypes.VertexStepModeVertex,
				Attributes:  []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2}},
			}}},
			Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			Multisample: gputypes.DefaultMultisampleState(),
			Fragment: &wgpu.FragmentState{
				Module: shader, EntryPoint: "fs_main",
				Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
			},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline: %v", err)
		}
		t.Cleanup(pipeline.Release)

		target, targetView := newUsageTexture(t, device, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		enc := newEncoder(t, device)
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{A: 1},
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, group, nil)
		pass.SetVertexBuffer(0, vertices, 0)
		pass.DrawIndirect(indirect, 0)
		if err := pass.End(); err != nil {
			t.Fatalf("RenderPass.End: %v", err)
		}
		enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: usageTargetSize},
			TextureBase:  wgpu.ImageCopyTexture{Texture: target},
			Size:         wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
		}})
		submit(t, device, enc)
		// 0.2, 0.4, 0.6 in unorm8: 51, 102, 153.
		checkTargetPixel(t, device, target, staging, [4]byte{51, 102, 153, 255})
	})
}

func writeWords(t *testing.T, device *wgpu.Device, buffer *wgpu.Buffer, words ...uint32) {
	t.Helper()
	data := make([]byte, len(words)*4)
	for i, w := range words {
		binary.LittleEndian.PutUint32(data[i*4:], w)
	}
	if err := device.Queue().WriteBuffer(buffer, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
}

func mathFloatBits(values ...float32) []uint32 {
	words := make([]uint32, len(values))
	for i, v := range values {
		words[i] = math.Float32bits(v)
	}
	return words
}
