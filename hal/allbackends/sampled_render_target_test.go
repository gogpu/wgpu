// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// An offscreen render target that is also sampled later (RenderAttachment |
// TextureBinding | CopySrc) is the common pattern of a consumer that renders
// into a texture and composites it. A render pass must leave such a target in
// the layout the texture tracker records for its usage, as Rust wgpu-hal does
// (vulkan/conv.rs derive_image_layout: COLOR_TARGET is
// COLOR_ATTACHMENT_OPTIMAL, the render pass's initial and final layout).
// Vulkan used to end the pass in GENERAL for every sampled render target while
// the tracker recorded COLOR_ATTACHMENT_OPTIMAL, so the next barrier named the
// wrong old layout (VUID-VkImageMemoryBarrier-oldLayout-01197) and a pass that
// loaded the target started from GENERAL.
//
// Each case draws into the target, then copies it, samples it into a second
// target, or both, and reads the pixels back. The Vulkan validation layer and
// the DX12 debug layer must stay silent.

// Colors of the target: the clear, the left-half draw and the right-half draw.
var (
	targetClear = [4]byte{0, 0, 255, 255}
	targetLeft  = [4]byte{10, 200, 30, 255}
	targetRight = [4]byte{250, 40, 90, 255}
)

func TestSampledRenderTarget(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, b overrideBackend, device *wgpu.Device)
	}{
		{"draw then copy", func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
			tex, view := newUsageTexture(t, device, sampledTargetUsage)
			enc := newEncoder(t, device)
			drawHalf(t, device, enc, view, gputypes.LoadOpClear, -1, 0, targetLeft)
			staging := copyTarget(t, device, enc, tex)
			submit(t, device, enc)
			checkRow(t, "target", staging, targetLeft, targetClear)
		}},
		{"load, sample and copy", func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
			tex, view := newUsageTexture(t, device, sampledTargetUsage)
			enc := newEncoder(t, device)
			drawHalf(t, device, enc, view, gputypes.LoadOpClear, -1, 0, targetLeft)
			drawHalf(t, device, enc, view, gputypes.LoadOpLoad, 0, 1, targetRight)
			_, sampled := sampleIntoTarget(t, device, enc, view)
			staging := copyTarget(t, device, enc, tex)
			submit(t, device, enc)
			checkRow(t, "sampled copy", sampled, targetLeft, targetRight)
			checkRow(t, "target", staging, targetLeft, targetRight)
		}},
		{"msaa resolve, sample and copy", func(t *testing.T, b overrideBackend, device *wgpu.Device) {
			if b.software {
				t.Skip("the software backend has no multisampled textures")
			}
			tex, view := newUsageTexture(t, device, sampledTargetUsage)
			enc := newEncoder(t, device)
			drawHalfMSAA(t, device, enc, view, -1, 0, targetLeft)
			drawHalf(t, device, enc, view, gputypes.LoadOpLoad, 0, 1, targetRight)
			_, sampled := sampleIntoTarget(t, device, enc, view)
			staging := copyTarget(t, device, enc, tex)
			submit(t, device, enc)
			checkRow(t, "sampled copy", sampled, targetLeft, targetRight)
			checkRow(t, "target", staging, targetLeft, targetRight)
		}},
		{"sample and copy in a later submission", func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
			tex, view := newUsageTexture(t, device, sampledTargetUsage)
			enc := newEncoder(t, device)
			drawHalf(t, device, enc, view, gputypes.LoadOpClear, -1, 0, targetLeft)
			submit(t, device, enc)

			enc = newEncoder(t, device)
			drawHalf(t, device, enc, view, gputypes.LoadOpLoad, 0, 1, targetRight)
			submit(t, device, enc)

			enc = newEncoder(t, device)
			_, sampled := sampleIntoTarget(t, device, enc, view)
			staging := copyTarget(t, device, enc, tex)
			submit(t, device, enc)
			checkRow(t, "sampled copy", sampled, targetLeft, targetRight)
			checkRow(t, "target", staging, targetLeft, targetRight)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			forEachDynamicOffsetDevice(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
				c.run(t, b, device)
			})
		})
	}
}

const sampledTargetUsage = wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopySrc

// fillHalfWGSL draws a full-height quad over [xLo, xHi] in NDC x in one color.
func fillHalfWGSL(xLo, xHi float32, color [4]byte) string {
	return fmt.Sprintf(`
@vertex
fn vs_main(@builtin(vertex_index) v: u32) -> @builtin(position) vec4<f32> {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    let c = quad[v %% 6u];
    return vec4<f32>(select(%f, %f, c.x > 0.0), c.y, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(%d.0, %d.0, %d.0, %d.0) / 255.0;
}
`, xLo, xHi, color[0], color[1], color[2], color[3])
}

// drawHalf records a render pass over view that loads or clears it to
// targetClear and draws the quad of fillHalfWGSL.
func drawHalf(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, view *wgpu.TextureView,
	load gputypes.LoadOp, xLo, xHi float32, color [4]byte) {
	t.Helper()
	drawHalfPass(t, device, enc, wgpu.RenderPassColorAttachment{View: view, LoadOp: load}, 1, xLo, xHi, color)
}

// drawHalfMSAA records a 4x multisampled pass, cleared to targetClear, that
// draws the quad of fillHalfWGSL and resolves into resolve.
func drawHalfMSAA(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, resolve *wgpu.TextureView,
	xLo, xHi float32, color [4]byte) {
	t.Helper()
	msaa, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "msaa",
		Size:          wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   4,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(msaa.Release)
	view, err := device.CreateTextureView(msaa, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	drawHalfPass(t, device, enc, wgpu.RenderPassColorAttachment{
		View: view, ResolveTarget: resolve, LoadOp: gputypes.LoadOpClear,
	}, 4, xLo, xHi, color)
}

// drawHalfPass records a render pass over the color attachment ca, whose
// clear value is targetClear and store op Store, with a pipeline of samples
// samples that draws the quad of fillHalfWGSL.
func drawHalfPass(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, ca wgpu.RenderPassColorAttachment,
	samples uint32, xLo, xHi float32, color [4]byte) {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "fill-half", WGSL: fillHalfWGSL(xLo, xHi, color)})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "fill-half"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:       "fill-half",
		Layout:      layout,
		Vertex:      wgpu.VertexState{Module: shader, EntryPoint: "vs_main"},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: samples, Mask: 0xFFFFFFFF},
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

	ca.StoreOp = gputypes.StoreOpStore
	ca.ClearValue = gputypes.Color{
		R: float64(targetClear[0]) / 255, G: float64(targetClear[1]) / 255,
		B: float64(targetClear[2]) / 255, A: float64(targetClear[3]) / 255,
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{ca},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
}

// copyTarget records a copy of the usageTargetSize-square tex into a new
// staging buffer, rows 256 bytes apart.
func copyTarget(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, tex *wgpu.Texture) *wgpu.Buffer {
	t.Helper()
	staging := newPublicBuffer(t, device, 256*usageTargetSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: usageTargetSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1},
	}})
	return staging
}

// checkRow checks texels (0, 1) and (usageTargetSize-1, 1) of a staging copy
// made by copyTarget or sampleIntoTarget.
func checkRow(t *testing.T, what string, staging *wgpu.Buffer, left, right [4]byte) {
	t.Helper()
	words := readPublicBuffer(t, staging, 256*usageTargetSize/4)
	texel := func(x int) [4]byte {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], words[256/4+x])
		return b
	}
	if got := texel(0); got != left {
		t.Errorf("%s: texel (0, 1) = %v, want %v", what, got, left)
	}
	if got := texel(usageTargetSize - 1); got != right {
		t.Errorf("%s: texel (%d, 1) = %v, want %v", what, usageTargetSize-1, got, right)
	}
}
