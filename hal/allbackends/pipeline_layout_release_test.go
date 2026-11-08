// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// A pipeline keeps its pipeline layout alive. WebGPU has no destroy() for a
// GPUPipelineLayout, so the only way an application lets one go is Release,
// which is Rust's drop (ADR-056). A pipeline created with that layout still
// needs it: every later SetBindGroup passes the layout to
// vkCmdBindDescriptorSets, and the pipeline was created from it. Rust
// wgpu-core holds an Arc<PipelineLayout> in the pipeline
// (pipeline.rs:496, :1155), so the HAL layout is destroyed only with the last
// pipeline. Without the same reference the released layout is destroyed at
// the next GPU completion and the following bind names a destroyed
// VkPipelineLayout (VUID-vkCmdBindDescriptorSets-layout-parameter), which the
// validation layer reports and this harness turns into a test failure
// (P2-CORE-022).

const pipelineLayoutReleaseComputeWGSL = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 4>;

@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    out[id.x] = id.x + 1u;
}
`

// TestReleasedPipelineLayoutComputePipeline creates a compute pipeline from a
// pipeline layout, releases the layout, lets the deferred destruction run, and
// only then dispatches through the pipeline.
func TestReleasedPipelineLayoutComputePipeline(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
		})
		out := newPublicBuffer(t, device, releaseSize*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc)
		staging := newPublicBuffer(t, device, releaseSize*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, Buffer: out, Size: releaseSize * 4})

		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: "pipeline-layout-release", WGSL: pipelineLayoutReleaseComputeWGSL,
		})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(shader.Release)
		layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
			Label:            "released",
			BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
		})
		if err != nil {
			t.Fatalf("CreatePipelineLayout: %v", err)
		}
		pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Label: "pipeline-of-a-released-layout", Layout: layout, Module: shader, EntryPoint: "main",
		})
		if err != nil {
			t.Fatalf("CreateComputePipeline: %v", err)
		}
		t.Cleanup(pipeline.Release)

		releasePipelineLayout(t, device, layout)

		enc := newEncoder(t, device)
		pass, err := enc.BeginComputePass(nil)
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, group, nil)
		pass.Dispatch(1, 1, 1)
		if err := pass.End(); err != nil {
			t.Fatalf("ComputePass.End: %v", err)
		}
		enc.CopyBufferToBuffer(out, 0, staging, 0, releaseSize*4)
		submit(t, device, enc)

		checkWords(t, "dispatch through a pipeline whose layout was released",
			readPublicBuffer(t, staging, releaseSize), []uint32{1, 2, 3, 4})
	})
}

const pipelineLayoutReleaseRenderWGSL = `
@group(0) @binding(0) var<uniform> color: vec4<f32>;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return color;
}
`

// TestReleasedPipelineLayoutRenderPipeline is the render pipeline's half of
// TestReleasedPipelineLayoutComputePipeline: the layout is released, the
// deferred destruction runs, and only then does a pass bind the pipeline and
// its bind group and draw.
func TestReleasedPipelineLayoutRenderPipeline(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageFragment,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
		})
		uniform := newPublicBuffer(t, device, 16, wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
		writeWords(t, device, uniform, mathFloatBits(0, 1, 0, 1)...) // green
		group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, Buffer: uniform, Size: 16})

		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: "pipeline-layout-release", WGSL: pipelineLayoutReleaseRenderWGSL,
		})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(shader.Release)
		layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
			Label:            "released",
			BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
		})
		if err != nil {
			t.Fatalf("CreatePipelineLayout: %v", err)
		}
		pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "pipeline-of-a-released-layout",
			Layout: layout,
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
			Fragment: &wgpu.FragmentState{
				Module:     shader,
				EntryPoint: "fs",
				Targets: []gputypes.ColorTargetState{
					{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
				},
			},
			Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline: %v", err)
		}
		t.Cleanup(pipeline.Release)

		releasePipelineLayout(t, device, layout)

		target := newPublicBufferTarget(t, device)
		staging := newPublicBuffer(t, device, pipelineLayoutTargetRowPitch*overrideTargetHeight,
			wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		enc := newEncoder(t, device)
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View:       target.view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
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
		enc.CopyTextureToBuffer(target.texture, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: pipelineLayoutTargetRowPitch, RowsPerImage: overrideTargetHeight},
			TextureBase:  wgpu.ImageCopyTexture{Texture: target.texture},
			Size:         wgpu.Extent3D{Width: overrideTargetWidth, Height: overrideTargetHeight, DepthOrArrayLayers: 1},
		}})
		submit(t, device, enc)

		// Texel (0, 0) of row 0 is the first word of the staging copy.
		pixel := readPublicBuffer(t, staging, overrideTargetWidth)[0]
		if want := uint32(0xFF00FF00); pixel != want {
			t.Errorf("texel (0,0) = %#08x, want %#08x (green)", pixel, want)
		}
	})
}

// TestReleasedPipelineLayoutCannotCreatePipeline: a released layout may
// already be queued for destruction, so a pipeline must not be created from
// it — the pipeline would name a dying HAL layout and revive its reference
// count, destroying it twice. A released sampler is rejected the same way by
// CreateBindGroup.
func TestReleasedPipelineLayoutCannotCreatePipeline(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: "pipeline-layout-released", WGSL: pipelineLayoutReleaseRenderWGSL,
		})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(shader.Release)
		layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "released"})
		if err != nil {
			t.Fatalf("CreatePipelineLayout: %v", err)
		}
		layout.Release()

		pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "of-a-released-layout",
			Layout: layout,
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
			Fragment: &wgpu.FragmentState{
				Module:     shader,
				EntryPoint: "fs",
				Targets: []gputypes.ColorTargetState{
					{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
				},
			},
			Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			if pipeline != nil {
				pipeline.Release()
			}
			t.Fatalf("CreateRenderPipeline with a released layout = %v, want ErrReleased", err)
		}
	})
}

const pipelineLayoutTargetRowPitch = 256 // overrideTargetWidth RGBA8 texels

type publicBufferTarget struct {
	texture *wgpu.Texture
	view    *wgpu.TextureView
}

// newPublicBufferTarget creates the 64x4 RGBA8 render target these tests draw
// into, with a copy source and a view of its only mip level.
func newPublicBufferTarget(t *testing.T, device *wgpu.Device) publicBufferTarget {
	t.Helper()
	texture, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "pipeline-layout-release",
		Size:          wgpu.Extent3D{Width: overrideTargetWidth, Height: overrideTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(texture.Release)
	view, err := device.CreateTextureView(texture, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return publicBufferTarget{texture: texture, view: view}
}

// releasePipelineLayout releases a pipeline layout and lets the deferred
// destruction run, so a layout that no longer has an owner is really destroyed
// before the work recorded afterwards.
func releasePipelineLayout(t *testing.T, device *wgpu.Device, layout *wgpu.PipelineLayout) {
	t.Helper()
	layout.Release()
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}
