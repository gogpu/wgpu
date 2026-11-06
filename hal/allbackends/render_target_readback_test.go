// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal/software"
)

// leftHalfShader fills the left half of the target with green. The quad is
// six vertices from vertex 0, so the draw does not depend on firstVertex.
const leftHalfShader = `
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(0.0, -1.0), vec2<f32>(0.0, 1.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// The mip-0 extent of the targets. 64 texels of RGBA8 are 256 bytes, the
// copy row alignment, so every mip level is read back with one row pitch.
const sliceRigWidth, sliceRigHeight, sliceRigDepth = 64, 4, 4

const sliceRigRowPitch = 256

var (
	sliceRed   = gputypes.Color{R: 1, G: 0, B: 0, A: 1}
	sliceBlue  = gputypes.Color{R: 0, G: 0, B: 1, A: 1}
	sliceGreen = [4]byte{0, 255, 0, 255}
)

// TestRenderToMipLevel renders into mip level 1 of a 2D texture: the pass
// must cover the 32x2 level, not the 64x4 texture. Regression test for the
// Vulkan view extent, which was the texture's mip-0 size for every view, so
// the framebuffer outgrew the view (VUID-VkFramebufferCreateInfo-flags-04533)
// and the render area covered texels the level does not have.
func TestRenderToMipLevel(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		skipSoftwareMip(t, device, 1)
		r := newTargetRig(t, device, gputypes.TextureDimension2D, 1)
		r.render(t, 1, nil, sliceRed)
		r.check(t, 1, 0, sliceRed)
	})
}

// TestRenderToMipLevelLeavesMip0 renders mip level 0, then a pass on a view
// of mip level 1: level 0 must keep its own contents. Regression test for
// the software backend, which rendered every view into the only level it
// stores (mip 0), and for GLES, whose offscreen framebuffer attached mip 0
// for every view.
func TestRenderToMipLevelLeavesMip0(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		r := newTargetRig(t, device, gputypes.TextureDimension2D, 1)
		r.render(t, 0, nil, sliceRed)
		r.render(t, 1, nil, sliceBlue)
		r.check(t, 0, 0, sliceRed)
	})
}

// skipSoftwareMip skips a test of a mip level above 0 on the software
// backend, which stores only mip level 0.
func skipSoftwareMip(t *testing.T, device *wgpu.Device, mip uint32) {
	t.Helper()
	if _, sw := device.HalDevice().(*software.Device); sw && mip > 0 {
		t.Skip("software: mip > 0 storage is P1-SW-026")
	}
}

type targetRig struct {
	device    *wgpu.Device
	texture   *wgpu.Texture
	dimension gputypes.TextureDimension
	depth     uint32
	pipeline  *wgpu.RenderPipeline
}

func newTargetRig(t *testing.T, device *wgpu.Device, dim gputypes.TextureDimension, depth uint32) *targetRig {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "slice-target",
		Size:          wgpu.Extent3D{Width: sliceRigWidth, Height: sliceRigHeight, DepthOrArrayLayers: depth},
		MipLevelCount: 2,
		SampleCount:   1,
		Dimension:     dim,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "left-half", WGSL: leftHalfShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "left-half-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "left-half",
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
	return &targetRig{device: device, texture: tex, dimension: dim, depth: depth, pipeline: pipeline}
}

// render clears one mip level (and, for a 3D texture, one depth slice) to
// clearColor and draws the green left half into it.
func (r *targetRig) render(t *testing.T, mip uint32, slice *uint32, clearColor gputypes.Color) {
	t.Helper()
	view := r.newView(t, mip)
	t.Cleanup(view.Release)
	r.renderView(t, view, slice, clearColor)
}

// newView creates a view of one mip level, 3D for a 3D texture.
func (r *targetRig) newView(t *testing.T, mip uint32) *wgpu.TextureView {
	t.Helper()
	viewDim := gputypes.TextureViewDimension2D
	if r.dimension == gputypes.TextureDimension3D {
		viewDim = gputypes.TextureViewDimension3D
	}
	view, err := r.device.CreateTextureView(r.texture, &wgpu.TextureViewDescriptor{
		Label: "target-view", Dimension: viewDim, BaseMipLevel: mip, MipLevelCount: 1, ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	return view
}

// newLayerView creates a view of one array layer of one mip level of a 2D
// texture, with dimension dim (2d or 2d-array).
func (r *targetRig) newLayerView(t *testing.T, dim gputypes.TextureViewDimension, mip, layer uint32) *wgpu.TextureView {
	t.Helper()
	view, err := r.device.CreateTextureView(r.texture, &wgpu.TextureViewDescriptor{
		Label: "layer-view", Dimension: dim, BaseMipLevel: mip, MipLevelCount: 1,
		BaseArrayLayer: layer, ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return view
}

// renderView clears view (one depth slice of it, for a 3D view) to clearColor,
// draws the green left half and submits the pass.
func (r *targetRig) renderView(t *testing.T, view *wgpu.TextureView, slice *uint32, clearColor gputypes.Color) {
	t.Helper()
	enc, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			DepthSlice: slice,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: clearColor,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(r.pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := r.device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

// check reads back one mip level and verifies that depth slice z (the array
// layer of a 2D texture) holds a green left half and clearColor on the right.
func (r *targetRig) check(t *testing.T, mip, z uint32, clearColor gputypes.Color) {
	t.Helper()
	r.verify(t, mip, z, clearColor, true)
}

// checkSolid verifies that depth slice z of one mip level holds clearColor only.
func (r *targetRig) checkSolid(t *testing.T, mip, z uint32, clearColor gputypes.Color) {
	t.Helper()
	r.verify(t, mip, z, clearColor, false)
}

func (r *targetRig) verify(t *testing.T, mip, z uint32, clearColor gputypes.Color, greenLeft bool) {
	t.Helper()
	w := max(uint32(sliceRigWidth)>>mip, 1)
	h := max(uint32(sliceRigHeight)>>mip, 1)
	// Every depth slice of a 3D level, every array layer of a 2D one.
	d := r.depth
	if r.dimension == gputypes.TextureDimension3D {
		d = max(r.depth>>mip, 1)
	}
	staging := newPublicBuffer(t, r.device, uint64(sliceRigRowPitch*h*d), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(r.texture, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: sliceRigRowPitch, RowsPerImage: h},
		TextureBase:  wgpu.ImageCopyTexture{Texture: r.texture, MipLevel: mip},
		Size:         wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: d},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := r.device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	words := readPublicBuffer(t, staging, int(sliceRigRowPitch/4*h*d))
	want := [4]byte{byte(clearColor.R * 255), byte(clearColor.G * 255), byte(clearColor.B * 255), byte(clearColor.A * 255)}
	for y := uint32(0); y < h; y++ {
		for _, x := range []uint32{0, w/2 - 1, w / 2, w - 1} {
			var px [4]byte
			binary.LittleEndian.PutUint32(px[:], words[(z*h+y)*sliceRigRowPitch/4+x])
			exp := want
			if greenLeft && x < w/2 {
				exp = sliceGreen
			}
			if px != exp {
				t.Errorf("mip %d slice %d texel (%d, %d) = %v, want %v", mip, z, x, y, px, exp)
			}
		}
	}
}
