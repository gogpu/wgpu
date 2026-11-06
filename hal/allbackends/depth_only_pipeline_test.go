// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// depthOnlyShader draws a full-target triangle at the depth the entry point
// names: vs_near (0.25) for the depth-only pipeline, vs_far (0.5) for the
// color pipeline drawn behind it.
const depthOnlyShader = `
fn tri(i: u32, z: f32) -> vec4<f32> {
    var p = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
    return vec4<f32>(p[i], z, 1.0);
}

@vertex
fn vs_near(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    return tri(i, 0.25);
}

@vertex
fn vs_far(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    return tri(i, 0.5);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}

@fragment
fn fs_empty() {}
`

// depthPassMode selects the first pass of drawBehindDepthOnly.
type depthPassMode int

const (
	noDepthPass            depthPassMode = iota // no first pass: the control
	depthPassNoFragment                         // Fragment nil
	depthPassEmptyFragment                      // a fragment stage with no outputs
)

// TestDepthOnlyPipeline creates a render pipeline without a fragment stage
// (WebGPU allows it for a depth-only pass) and draws with it on every
// backend. A first pass writes depth 0.25 with that pipeline; a second pass
// draws green at depth 0.5 with depth test Less, which must fail everywhere
// the first pass drew. GLES needs an empty fragment shader linked into the
// program: OpenGL ES refuses to link a program without one. The depth format
// is Depth24Plus, which OpenGL ES 3.0 has as DEPTH_COMPONENT24.
func TestDepthOnlyPipeline(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, tt := range []struct {
			name string
			mode depthPassMode
			want [4]byte
		}{
			{name: "no fragment stage occludes", mode: depthPassNoFragment, want: [4]byte{255, 0, 0, 255}},
			{name: "empty fragment stage occludes", mode: depthPassEmptyFragment, want: [4]byte{255, 0, 0, 255}},
			{name: "control without a depth pass", mode: noDepthPass, want: [4]byte{0, 255, 0, 255}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				if tt.mode != noDepthPass && strings.Contains(t.Name(), "/software/") {
					t.Skip("software: a depth-only pass writes no depth, P1-SW-021")
				}
				if px := drawBehindDepthOnly(t, device, tt.mode); px != tt.want {
					t.Errorf("pixel (0,0) = %v, want %v", px, tt.want)
				}
			})
		}
	})
}

// drawBehindDepthOnly runs the two passes of TestDepthOnlyPipeline, the
// depth-only one as mode selects, and returns pixel (0,0) of the color
// target, cleared to red.
func drawBehindDepthOnly(t *testing.T, device *wgpu.Device, mode depthPassMode) [4]byte {
	t.Helper()
	size := wgpu.Extent3D{Width: overrideTargetWidth, Height: overrideTargetHeight, DepthOrArrayLayers: 1}
	color := newAttachment(t, device, size, gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	depth := newAttachment(t, device, size, gputypes.TextureFormatDepth24Plus, wgpu.TextureUsageRenderAttachment)
	colorView := newAttachmentView(t, device, color)
	depthView := newAttachmentView(t, device, depth)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "depth-only", WGSL: depthOnlyShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-only-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	depthLoad := gputypes.LoadOpClear
	if mode != noDepthPass {
		var fragment *wgpu.FragmentState
		if mode == depthPassEmptyFragment {
			fragment = &wgpu.FragmentState{Module: shader, EntryPoint: "fs_empty"}
		}
		near := newDepthPipeline(t, device, layout, shader, "vs_near", fragment)
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
				View: depthView, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
			},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass (depth only): %v", err)
		}
		pass.SetPipeline(near)
		pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		if err := pass.End(); err != nil {
			t.Fatalf("End (depth only): %v", err)
		}
		depthLoad = gputypes.LoadOpLoad
	}

	far := newDepthPipeline(t, device, layout, shader, "vs_far", &wgpu.FragmentState{
		Module: shader, EntryPoint: "fs",
		Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
	})
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View: depthView, DepthLoadOp: depthLoad, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass (color): %v", err)
	}
	pass.SetPipeline(far)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End (color): %v", err)
	}

	const words = overrideTargetWidth * overrideTargetHeight
	staging := newPublicBuffer(t, device, words*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(color, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: overrideTargetWidth * 4, RowsPerImage: overrideTargetHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: color},
		Size:         size,
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	var px [4]byte
	binary.LittleEndian.PutUint32(px[:], readPublicBuffer(t, staging, words)[0])
	return px
}

func newAttachment(t *testing.T, device *wgpu.Device, size wgpu.Extent3D, format gputypes.TextureFormat, usage gputypes.TextureUsage) *wgpu.Texture {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size: size, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: format, Usage: usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture %v: %v", format, err)
	}
	t.Cleanup(tex.Release)
	return tex
}

func newAttachmentView(t *testing.T, device *wgpu.Device, tex *wgpu.Texture) *wgpu.TextureView {
	t.Helper()
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return view
}

// newDepthPipeline creates a pipeline that writes Depth24Plus with test
// Less, with fragment stage fragment (nil: a depth-only pipeline).
func newDepthPipeline(t *testing.T, device *wgpu.Device, layout *wgpu.PipelineLayout, shader *wgpu.ShaderModule, vs string, fragment *wgpu.FragmentState) *wgpu.RenderPipeline {
	t.Helper()
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:    vs,
		Layout:   layout,
		Vertex:   wgpu.VertexState{Module: shader, EntryPoint: vs},
		Fragment: fragment,
		DepthStencil: &wgpu.DepthStencilState{
			Format: gputypes.TextureFormatDepth24Plus, DepthWriteEnabled: true, DepthCompare: gputypes.CompareFunctionLess,
			StencilFront: wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
			StencilBack:  wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline %s: %v", vs, err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}
