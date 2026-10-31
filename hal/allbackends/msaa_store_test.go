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

// halvesShader draws one triangle in green: vs_left covers the left half of
// the target, vs_right the right half. Each is drawn from vertex 0, so the
// test does not depend on firstVertex (DX12 ignores it in vertex_index,
// P1-DX12-054).
const halvesShader = `
@vertex
fn vs_left(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(-1.0, 1.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@vertex
fn vs_right(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 3>(vec2<f32>(1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(1.0, 1.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// TestMSAAStoreWithResolveIsLoadable checks that a color attachment stored by
// one pass is loaded by the next, with and without MSAA. WebGPU: storeOp
// "store" on a multisampled attachment keeps its samples even when the
// attachment has a resolveTarget. Pass 1 clears to red, draws the left half
// in green and stores (and resolves); pass 2 loads, draws the right half and
// resolves again, so it must hold both halves.
//
// Regression test for P1-GLES-013: the first resolve into a new resolve
// texture blitted that texture onto itself, so pass 1 read back zeros. Also
// covers P1-VK-011 (the MSAA attachment's Store forced to DontCare) on
// drivers that discard DontCare contents; Intel and NVIDIA keep them, so
// TestBuildRenderPassLayoutMSAAStoreOp in hal/vulkan is the check there.
func TestMSAAStoreWithResolveIsLoadable(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, samples := range []uint32{1, 4} {
			t.Run(map[uint32]string{1: "single-sample", 4: "msaa4"}[samples], func(t *testing.T) {
				if samples > 1 && strings.Contains(t.Name(), "/software/") {
					t.Skip("the software backend has no MSAA and refuses SampleCount > 1 (BUG-SW-007)")
				}
				r := newMSAARig(t, device, samples)
				for frame := 0; frame < 2; frame++ {
					left, right := r.pass(t, gputypes.LoadOpClear, r.left)
					if !left || right {
						t.Fatalf("frame %d, pass 1: drawn (left, right) = (%v, %v), want (true, false)", frame, left, right)
					}
					left, right = r.pass(t, gputypes.LoadOpLoad, r.right)
					if !left || !right {
						t.Fatalf("frame %d, pass 2: drawn (left, right) = (%v, %v), want (true, true)", frame, left, right)
					}
				}
			})
		}
	})
}

const msaaRigWidth, msaaRigHeight = 64, 4 // 256 bytes per row, the copy alignment

type msaaRig struct {
	device      *wgpu.Device
	readback    *wgpu.Texture     // the resolve target, or the color target without MSAA
	color       *wgpu.TextureView // the attachment drawn into
	resolve     *wgpu.TextureView // nil without MSAA
	left, right *wgpu.RenderPipeline
}

func newMSAARig(t *testing.T, device *wgpu.Device, samples uint32) *msaaRig {
	t.Helper()
	size := wgpu.Extent3D{Width: msaaRigWidth, Height: msaaRigHeight, DepthOrArrayLayers: 1}
	newTexture := func(label string, samples uint32, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label: label, Size: size, MipLevelCount: 1, SampleCount: samples,
			Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm, Usage: usage,
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
	r := &msaaRig{device: device}
	if samples == 1 {
		r.readback, r.color = newTexture("color", 1, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	} else {
		_, r.color = newTexture("msaa-color", samples, wgpu.TextureUsageRenderAttachment)
		r.readback, r.resolve = newTexture("msaa-resolve", 1, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "halves", WGSL: halvesShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "halves-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	newPipeline := func(vs string) *wgpu.RenderPipeline {
		p, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  vs,
			Layout: layout,
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: vs},
			Fragment: &wgpu.FragmentState{
				Module:     shader,
				EntryPoint: "fs",
				Targets: []gputypes.ColorTargetState{
					{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
				},
			},
			Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			Multisample: gputypes.MultisampleState{Count: samples, Mask: 0xFFFFFFFF},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline %s: %v", vs, err)
		}
		t.Cleanup(p.Release)
		return p
	}
	r.left, r.right = newPipeline("vs_left"), newPipeline("vs_right")
	return r
}

// pass draws pipeline's triangle into the color attachment with Store (and a
// resolve with MSAA), reads the resolved (or single-sample) target back and
// reports whether the left and the right half are green.
func (r *msaaRig) pass(t *testing.T, load gputypes.LoadOp, pipeline *wgpu.RenderPipeline) (left, right bool) {
	t.Helper()
	staging := newPublicBuffer(t, r.device, msaaRigWidth*msaaRigHeight*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	ca := wgpu.RenderPassColorAttachment{
		View:       r.color,
		LoadOp:     load,
		StoreOp:    gputypes.StoreOpStore,
		ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
	}
	if r.resolve != nil {
		ca.ResolveTarget = r.resolve
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{ca},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyTextureToBuffer(r.readback, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: msaaRigWidth * 4, RowsPerImage: msaaRigHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: r.readback},
		Size:         wgpu.Extent3D{Width: msaaRigWidth, Height: msaaRigHeight, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := r.device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	words := readPublicBuffer(t, staging, msaaRigWidth*msaaRigHeight)
	green := func(x, y int) bool {
		var px [4]byte
		binary.LittleEndian.PutUint32(px[:], words[y*msaaRigWidth+x])
		return px[0] < 128 && px[1] > 128
	}
	// Row 2 is at NDC y = -0.25; the left triangle covers x <= 20 there, the
	// right one x >= 44 (as in drawCullTriangles).
	return green(8, 2), green(56, 2)
}
