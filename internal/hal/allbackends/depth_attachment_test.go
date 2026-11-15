// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDepthAttachmentStates is the regression test for P1-DX12-056 (reported
// by g3d). The device tracker records a depth attachment as depth-stencil
// write (read when the pass is read-only), and the HAL barrier gets that
// state as RenderAttachment (TextureBinding). The backends mapped both to
// their color/sampled states: DX12 transitioned a depth texture with only
// RenderAttachment usage (ALLOW_DEPTH_STENCIL, DENY_SHADER_RESOURCE) to
// RENDER_TARGET, the debug layer reported id 524 and Close failed with
// E_INVALIDARG on the first submit; Vulkan moved the image to
// COLOR_ATTACHMENT_OPTIMAL (VUID-VkImageMemoryBarrier-oldLayout-01208).
//
// Every case runs two frames, so the second submit transitions a texture the
// tracker already knows.
func TestDepthAttachmentStates(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		t.Run("write-render-attachment-only", func(t *testing.T) {
			// g3d's case: Depth24Plus with RenderAttachment only.
			r := newDepthRig(t, device, gputypes.TextureFormatDepth24Plus, wgpu.TextureUsageRenderAttachment)
			for frame := 0; frame < 2; frame++ {
				if !r.draw(t, depthPass{clear: true, clearValue: 1, write: true}) {
					t.Fatalf("frame %d: the triangle was not drawn", frame)
				}
			}
		})
		t.Run("read-only-after-write", func(t *testing.T) {
			r := newDepthRig(t, device, gputypes.TextureFormatDepth24Plus, wgpu.TextureUsageRenderAttachment)
			for frame := 0; frame < 2; frame++ {
				// Depth cleared to 0: the triangle at z = 0 fails Less.
				r.draw(t, depthPass{clear: true, clearValue: 0, write: true, noDraw: true})
				if r.draw(t, depthPass{readOnly: true}) {
					t.Fatalf("frame %d: drawn through a depth of 0 in a read-only pass", frame)
				}
			}
		})
	})
}

// TestDepthAttachmentWriteThenSampled: a depth texture written as an
// attachment, then read by a compute pass (sampleDepth), two frames; the
// compute pass needs DownlevelFlagsComputeShaders, which GL 4.1 lacks.
// Each frame also samples a depth cleared to 0.75: the software backend
// decoded sampled depth as RGBA8 bytes, which only a depth of 0 survives.
func TestDepthAttachmentWriteThenSampled(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		r := newDepthRig(t, device, gputypes.TextureFormatDepth32Float,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageTextureBinding)
		for frame := 0; frame < 2; frame++ {
			r.draw(t, depthPass{clear: true, clearValue: 0.75, write: true, noDraw: true})
			// GLES reads 0.75000006 back from its depth store.
			if got := r.sampleDepth(t); got < 0.75-1e-6 || got > 0.75+1e-6 {
				t.Fatalf("frame %d: sampled depth = %v, want the cleared 0.75", frame, got)
			}
			if !r.draw(t, depthPass{clear: true, clearValue: 1, write: true}) {
				t.Fatalf("frame %d: the triangle was not drawn", frame)
			}
			if got := r.sampleDepth(t); got != 0 {
				t.Fatalf("frame %d: sampled depth = %v, want 0 (the triangle's z)", frame, got)
			}
		}
	})
}

// keepStencil disables stencil testing: the rig's depth formats have no
// stencil aspect.
var keepStencil = wgpu.StencilFaceState{
	Compare:     gputypes.CompareFunctionAlways,
	FailOp:      gputypes.StencilOperationKeep,
	DepthFailOp: gputypes.StencilOperationKeep,
	PassOp:      gputypes.StencilOperationKeep,
}

const depthRigWidth, depthRigHeight = 64, 4 // 256 bytes per row, the copy alignment

// depthRig renders cullShader's left triangle (z = 0) with a depth test into a
// 64x4 RGBA8 target cleared to red, with a depth attachment.
type depthRig struct {
	device               *wgpu.Device
	target               *wgpu.Texture
	targetView           *wgpu.TextureView
	depthView            *wgpu.TextureView
	write, read          *wgpu.RenderPipeline
	shader, sampleShader *wgpu.ShaderModule
}

type depthPass struct {
	clear      bool
	clearValue float32
	write      bool // depth writes on; otherwise a read-only attachment
	readOnly   bool
	noDraw     bool
}

func newDepthRig(t *testing.T, device *wgpu.Device, depthFormat gputypes.TextureFormat, depthUsage gputypes.TextureUsage) *depthRig {
	t.Helper()
	size := wgpu.Extent3D{Width: depthRigWidth, Height: depthRigHeight, DepthOrArrayLayers: 1}
	newTexture := func(label string, format gputypes.TextureFormat, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label: label, Size: size, MipLevelCount: 1, SampleCount: 1,
			Dimension: gputypes.TextureDimension2D, Format: format, Usage: usage,
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
	r := &depthRig{device: device}
	r.target, r.targetView = newTexture("depth-rig-target", gputypes.TextureFormatRGBA8Unorm,
		wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	_, r.depthView = newTexture("depth-rig-depth", depthFormat, depthUsage)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "depth-rig", WGSL: cullShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	r.shader = shader
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-rig-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	newPipeline := func(depthWrite bool) *wgpu.RenderPipeline {
		p, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "depth-rig",
			Layout: layout,
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
			Fragment: &wgpu.FragmentState{
				Module: shader, EntryPoint: "fs",
				Targets: []gputypes.ColorTargetState{
					{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
				},
			},
			Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			DepthStencil: &wgpu.DepthStencilState{
				Format:            depthFormat,
				DepthWriteEnabled: depthWrite,
				DepthCompare:      gputypes.CompareFunctionLess,
				StencilFront:      keepStencil,
				StencilBack:       keepStencil,
			},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline: %v", err)
		}
		t.Cleanup(p.Release)
		return p
	}
	r.write, r.read = newPipeline(true), newPipeline(false)
	return r
}

// draw records one pass and reports whether the triangle was drawn (green at
// row 2, x = 8, inside the left triangle; see drawCullTriangles).
func (r *depthRig) draw(t *testing.T, p depthPass) bool {
	t.Helper()
	device := r.device
	staging := newPublicBuffer(t, device, depthRigWidth*depthRigHeight*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	ds := &wgpu.RenderPassDepthStencilAttachment{View: r.depthView}
	switch {
	case p.readOnly:
		// A read-only aspect takes no load or store op.
		ds.DepthReadOnly, ds.StencilReadOnly = true, true
	case p.clear:
		ds.DepthLoadOp, ds.DepthStoreOp, ds.DepthClearValue = gputypes.LoadOpClear, gputypes.StoreOpStore, p.clearValue
	default:
		ds.DepthLoadOp, ds.DepthStoreOp = gputypes.LoadOpLoad, gputypes.StoreOpStore
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: r.targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
		DepthStencilAttachment: ds,
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if !p.noDraw {
		if p.write {
			pass.SetPipeline(r.write)
		} else {
			pass.SetPipeline(r.read)
		}
		pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyTextureToBuffer(r.target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthRigWidth * 4, RowsPerImage: depthRigHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: r.target},
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
	var px [4]byte
	binary.LittleEndian.PutUint32(px[:], words[2*depthRigWidth+8])
	return px[0] < 128 && px[1] > 128
}

// sampleDepthShader reads the depth texture at (8, 2) with textureLoad.
const sampleDepthShader = `
@group(0) @binding(0) var depth: texture_depth_2d;
@group(0) @binding(1) var<storage, read_write> out: array<f32, 1>;

@compute @workgroup_size(1)
fn main() {
    out[0] = textureLoad(depth, vec2<i32>(8, 2), 0);
}
`

// sampleDepth reads the depth texture in a compute shader.
func (r *depthRig) sampleDepth(t *testing.T) float32 {
	t.Helper()
	device := r.device
	if r.sampleShader == nil {
		s, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "depth-sample", WGSL: sampleDepthShader})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(s.Release)
		r.sampleShader = s
	}
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "depth-sample",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeDepth, ViewDimension: gputypes.TextureViewDimension2D,
			}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeStorage,
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(bgl.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-sample", BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "depth-sample", Layout: layout, Module: r.sampleShader, EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	out := newPublicBuffer(t, device, 4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc)
	staging := newPublicBuffer(t, device, 4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "depth-sample", Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, TextureView: r.depthView},
			{Binding: 1, Buffer: out, Size: 4},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(bg.Release)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyBufferToBuffer(out, 0, staging, 0, 4)
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	words := readPublicBuffer(t, staging, 1)
	return math.Float32frombits(words[0])
}
