// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// cullShader draws two triangles from vertex_index: 0-2 counter-clockwise in
// NDC (left half), 3-5 clockwise in NDC (right half), in green.
const cullShader = `
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(1.0, 1.0),
    );
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// TestCullModeAndFrontFace checks face culling on every backend against the
// WebGPU spec rule (facing from the polygon area in framebuffer coordinates;
// a triangle counter-clockwise in NDC has a positive area, so it is
// front-facing for FrontFaceCCW). The expected masks are the same for all
// backends, so the software backend is checked against the spec, not against
// another backend (P1-SW-014: software ignored CullMode and had the winding
// sign inverted).
func TestCullModeAndFrontFace(t *testing.T) {
	cases := []struct {
		name        string
		cull        gputypes.CullMode
		front       gputypes.FrontFace
		left, right bool // drawn: NDC-counter-clockwise, NDC-clockwise
	}{
		{"none", gputypes.CullModeNone, gputypes.FrontFaceCCW, true, true},
		{"back-ccw", gputypes.CullModeBack, gputypes.FrontFaceCCW, true, false},
		{"front-ccw", gputypes.CullModeFront, gputypes.FrontFaceCCW, false, true},
		{"back-cw", gputypes.CullModeBack, gputypes.FrontFaceCW, false, true},
		{"front-cw", gputypes.CullModeFront, gputypes.FrontFaceCW, true, false},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				left, right := drawCullTriangles(t, device, gputypes.PrimitiveState{
					Topology:  gputypes.PrimitiveTopologyTriangleList,
					FrontFace: c.front,
					CullMode:  c.cull,
				})
				if left != c.left || right != c.right {
					t.Errorf("drawn (NDC-ccw left, NDC-cw right) = (%v, %v), want (%v, %v)", left, right, c.left, c.right)
				}
			})
		}
	})
}

// drawCullTriangles renders cullShader into a 64x4 RGBA8 target cleared to
// red and reports whether the left and the right triangle were drawn (green).
func drawCullTriangles(t *testing.T, device *wgpu.Device, primitive gputypes.PrimitiveState) (left, right bool) {
	t.Helper()
	const width, height = 64, 4 // 256 bytes per row, the copy alignment
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "cull-target",
		Size:          wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "cull", WGSL: cullShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "cull-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "cull",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   primitive,
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	staging := newPublicBuffer(t, device, width*height*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: width * 4, RowsPerImage: height},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	words := readPublicBuffer(t, staging, width*height)
	green := func(x, y int) bool {
		var px [4]byte
		binary.LittleEndian.PutUint32(px[:], words[y*width+x])
		return px[0] < 128 && px[1] > 128
	}
	// Row 2 is at NDC y = -0.25; the left triangle covers x <= 20 there, the
	// right one x >= 44.
	return green(8, 2), green(56, 2)
}
