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

// twoSidedStencilShader draws cullShader's two triangles: vertices 0-2 are
// counter-clockwise in NDC (the left half), vertices 3-5 clockwise (the right
// half). fs_mark paints blue, fs_probe green.
const twoSidedStencilShader = `
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(1.0, 1.0),
    );
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs_mark() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 0.0, 1.0, 1.0);
}

@fragment
fn fs_probe() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// Probe pixels in row 2 of the 64x4 target: inside the left (NDC
// counter-clockwise) and the right (NDC clockwise) triangle.
const twoSidedLeftX, twoSidedRightX = 8, 55

// TestStencilTwoSided is the regression test for P2-SW-030: a back-facing
// triangle uses stencilBack and a front-facing one stencilFront, with facing
// decided by primitive.frontFace (index.bs "stencil test" and "front-facing").
// The software rasterizer applied stencilFront to every triangle and enabled
// the stencil test only when stencilFront was not the default.
//
// The mark pass draws both triangles with the faces under test into a stencil
// cleared to 0. Each triangle is then drawn again with compare "equal" against
// the value it should hold; only a match paints the probe green.
func TestStencilTwoSided(t *testing.T) {
	inc := wgpu.StencilFaceState{
		Compare: gputypes.CompareFunctionAlways, FailOp: wgpu.StencilOperationKeep,
		DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationIncrementClamp,
	}
	dec := inc
	dec.PassOp = wgpu.StencilOperationDecrementWrap
	replace := inc
	replace.PassOp = wgpu.StencilOperationReplace
	keep := wgpu.StencilFaceState{
		Compare: gputypes.CompareFunctionAlways, FailOp: wgpu.StencilOperationKeep,
		DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationKeep,
	}
	cases := []struct {
		name        string
		frontFace   gputypes.FrontFace
		front, back wgpu.StencilFaceState
		ref         uint32
		// wantLeft, wantRight: the stencil the mark pass leaves under the
		// left and the right triangle.
		wantLeft, wantRight uint32
	}{
		// CCW: the left triangle is front-facing (incremented to 1), the
		// right one back-facing (0 decremented with wrap to 255).
		{"front-inc-back-dec-ccw", gputypes.FrontFaceCCW, inc, dec, 0, 1, 255},
		// CW swaps the facing of the two triangles.
		{"front-inc-back-dec-cw", gputypes.FrontFaceCW, inc, dec, 0, 255, 1},
		// Only the back face has a non-default state: the stencil test must
		// still run, and only the back-facing (right) triangle writes.
		{"back-only", gputypes.FrontFaceCCW, keep, replace, 7, 0, 7},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				left, right := drawTwoSidedStencil(t, device, c.frontFace, c.front, c.back, c.ref, c.wantLeft, c.wantRight)
				if !isGreen(left) || !isGreen(right) {
					t.Fatalf("left (%d, 2) = %v, right (%d, 2) = %v; want both green: stencil should be "+
						"left %d, right %d after the mark pass (blue = the probe's equal test failed)",
						twoSidedLeftX, left, twoSidedRightX, right, c.wantLeft, c.wantRight)
				}
			})
		}
	})
}

func isGreen(px [4]byte) bool { return px[0] < 128 && px[1] >= 128 && px[2] < 128 }

// drawTwoSidedStencil runs the mark pass with front/back and reference ref,
// then probes the left triangle for wantLeft and the right one for wantRight,
// and returns the two probe pixels.
func drawTwoSidedStencil(t *testing.T, device *wgpu.Device, frontFace gputypes.FrontFace,
	front, back wgpu.StencilFaceState, ref, wantLeft, wantRight uint32) (left, right [4]byte) {
	t.Helper()
	const format = gputypes.TextureFormatDepth24PlusStencil8
	target, targetView := newZeroDefaultsTexture(t, device, gputypes.TextureFormatRGBA8Unorm,
		wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	_, dsView := newZeroDefaultsTexture(t, device, format, wgpu.TextureUsageRenderAttachment)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "two-sided-stencil", WGSL: twoSidedStencilShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "two-sided-stencil"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	newPipeline := func(entry string, ff gputypes.FrontFace, f, b wgpu.StencilFaceState) *wgpu.RenderPipeline {
		p, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "two-sided-stencil-" + entry,
			Layout: layout,
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
			Fragment: &wgpu.FragmentState{
				Module: shader, EntryPoint: entry,
				Targets: []gputypes.ColorTargetState{
					{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
				},
			},
			Primitive: gputypes.PrimitiveState{
				Topology: gputypes.PrimitiveTopologyTriangleList, FrontFace: ff, CullMode: gputypes.CullModeNone,
			},
			DepthStencil: &wgpu.DepthStencilState{
				Format: format, DepthCompare: gputypes.CompareFunctionAlways,
				StencilFront: f, StencilBack: b, StencilReadMask: 0xFF, StencilWriteMask: 0xFF,
			},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline %s: %v", entry, err)
		}
		t.Cleanup(p.Release)
		return p
	}
	mark := newPipeline("fs_mark", frontFace, front, back)
	equal := wgpu.StencilFaceState{
		Compare: gputypes.CompareFunctionEqual, FailOp: wgpu.StencilOperationKeep,
		DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationKeep,
	}
	probe := newPipeline("fs_probe", gputypes.FrontFaceCCW, equal, equal)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:        dsView,
			DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
			StencilLoadOp: gputypes.LoadOpClear, StencilStoreOp: gputypes.StoreOpStore,
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(mark)
	pass.SetStencilReference(ref)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	pass.SetPipeline(probe)
	pass.SetStencilReference(wantLeft)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	pass.SetStencilReference(wantRight)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1, FirstVertex: 3})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	staging := newPublicBuffer(t, device, depthRigWidth*depthRigHeight*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthRigWidth * 4, RowsPerImage: depthRigHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: depthRigWidth, Height: depthRigHeight, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	words := readPublicBuffer(t, staging, depthRigWidth*depthRigHeight)
	binary.LittleEndian.PutUint32(left[:], words[2*depthRigWidth+twoSidedLeftX])
	binary.LittleEndian.PutUint32(right[:], words[2*depthRigWidth+twoSidedRightX])
	return left, right
}
