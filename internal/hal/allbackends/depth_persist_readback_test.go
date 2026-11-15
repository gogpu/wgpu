// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// depthPersistShader passes position and color through from the vertex
// buffer (depthPersistVertices).
const depthPersistShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) color: vec4f,
}

@vertex
fn vs(@location(0) p: vec3f, @location(1) color: vec4f) -> VOut {
    return VOut(vec4f(p, 1.0), color);
}

@fragment
fn fs(in: VOut) -> @location(0) vec4f {
    return in.color;
}
`

// depthPersistVertices returns a full-target quad at z = 0.5 in red
// (vertices 0..5), then a quad at z = 0.75 in green over the left half
// (6..11) and one at z = 0.25 in green over the right half (12..17), as
// position xyz and color rgba.
func depthPersistVertices() []float32 {
	quad := func(x0, x1, z float32, rgba [4]float32) []float32 {
		out := make([]float32, 0, 6*7)
		for _, xy := range [6][2]float32{{x0, -1}, {x1, -1}, {x0, 1}, {x1, -1}, {x1, 1}, {x0, 1}} {
			out = append(out, xy[0], xy[1], z, rgba[0], rgba[1], rgba[2], rgba[3])
		}
		return out
	}
	red, green := [4]float32{1, 0, 0, 1}, [4]float32{0, 1, 0, 1}
	out := quad(-1, 1, 0.5, red)
	out = append(out, quad(-1, 0, 0.75, green)...)
	return append(out, quad(0, 1, 0.25, green)...)
}

// depthPersistStride is the size of one depthPersistVertices vertex.
const depthPersistStride = 7 * 4

// TestDepthPersistsAcrossDraws checks that depth written by one draw tests the
// next one, within a pass and across two passes with LoadOpLoad, for each
// depth format: the red quad at z = 0.5 hides the green one at 0.75 on the
// left, and the green one at 0.25 covers it on the right. The software
// backend tested every draw against a fresh depth buffer and never stored
// depth, so both halves turned green (P1-SW-015).
func TestDepthPersistsAcrossDraws(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		for _, format := range []gputypes.TextureFormat{
			gputypes.TextureFormatDepth32Float,
			gputypes.TextureFormatDepth24Plus,
			gputypes.TextureFormatDepth16Unorm,
		} {
			for _, twoPasses := range []bool{false, true} {
				name := format.String() + "/one-pass"
				if twoPasses {
					name = format.String() + "/two-passes"
				}
				t.Run(name, func(t *testing.T) {
					checkDepthPersists(t, device, format, twoPasses)
				})
			}
		}
	})
}

func checkDepthPersists(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, twoPasses bool) {
	const width, height = 64, 4 // 256 bytes per row, the copy alignment
	size := wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1}
	newView := func(label string, f gputypes.TextureFormat, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label: label, Size: size, MipLevelCount: 1, SampleCount: 1,
			Dimension: gputypes.TextureDimension2D, Format: f, Usage: usage,
		})
		if err != nil {
			t.Fatalf("CreateTexture %s: %v", label, err)
		}
		t.Cleanup(tex.Release)
		view, err := device.CreateTextureView(tex, nil)
		if err != nil {
			t.Fatalf("CreateTextureView %s: %v", label, err)
		}
		t.Cleanup(view.Release)
		return tex, view
	}
	target, targetView := newView("depth-persist-target", gputypes.TextureFormatRGBA8Unorm,
		wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	_, depthView := newView("depth-persist-depth", format, wgpu.TextureUsageRenderAttachment)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "depth-persist", WGSL: depthPersistShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-persist"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "depth-persist",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs", Buffers: []gputypes.VertexBufferLayout{{
			ArrayStride: depthPersistStride, StepMode: gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{
				{Format: gputypes.VertexFormatFloat32x3, ShaderLocation: 0},
				{Format: gputypes.VertexFormatFloat32x4, Offset: 12, ShaderLocation: 1},
			},
		}}},
		Fragment: &wgpu.FragmentState{Module: shader, EntryPoint: "fs", Targets: []gputypes.ColorTargetState{
			{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
		}},
		Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		DepthStencil: &wgpu.DepthStencilState{
			Format: format, DepthWriteEnabled: true, DepthCompare: gputypes.CompareFunctionLess,
			StencilFront: keepStencil, StencilBack: keepStencil,
		},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	beginPass := func(first bool) *wgpu.RenderPassEncoder {
		color := wgpu.RenderPassColorAttachment{View: targetView, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
		ds := &wgpu.RenderPassDepthStencilAttachment{View: depthView, DepthLoadOp: gputypes.LoadOpLoad, DepthStoreOp: gputypes.StoreOpStore}
		if first {
			color.LoadOp, color.ClearValue = gputypes.LoadOpClear, gputypes.Color{A: 1}
			ds.DepthLoadOp, ds.DepthClearValue = gputypes.LoadOpClear, 1
		}
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments:       []wgpu.RenderPassColorAttachment{color},
			DepthStencilAttachment: ds,
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		pass.SetPipeline(pipeline)
		return pass
	}
	endPass := func(pass *wgpu.RenderPassEncoder) {
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	}
	vertices := newVertexBuffer(t, device, depthPersistVertices())
	pass := beginPass(true)
	pass.SetVertexBuffer(0, vertices, 0)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	if twoPasses {
		endPass(pass)
		pass = beginPass(false)
	}
	// The green quads, through a buffer offset rather than firstVertex.
	pass.SetVertexBuffer(0, vertices, 6*depthPersistStride)
	pass.Draw(gputypes.DrawArgs{VertexCount: 12, InstanceCount: 1})
	endPass(pass)

	staging := newPublicBuffer(t, device, width*height*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: width * 4, RowsPerImage: height},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         size,
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	const red, green = 0xFF0000FF, 0xFF00FF00
	row := readPublicBuffer(t, staging, width*height)[width : 2*width]
	for x, px := range row {
		want := uint32(red)
		if x >= width/2 {
			want = green
		}
		if px != want {
			t.Errorf("pixel %d = %#08x, want %#08x (red: the left quad hidden; green: the right one in front)", x, px, want)
			break
		}
	}
}
