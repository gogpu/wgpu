// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// staleColorSize is the side of mip level 0 of the color texture; its mip
// level 1 is depthReadbackSize square, the size of the other attachments.
const staleColorSize = 2 * depthReadbackSize

// TestRenderPassDropsAttachmentsOfEarlierPass renders into mip level 1 of a
// color texture with a second attachment (a depth texture, or a second color
// target), then renders into mip level 0 with the color attachment alone and
// checks that the whole level is drawn. GLES keeps one framebuffer per color
// texture and never detached what an earlier pass attached, so the second
// pass rendered with the first pass's smaller attachment still in place,
// which limits rendering to the area all attachments share. Mesa (llvmpipe)
// clips the draw that way; the Intel driver on Windows renders past it, so
// this test fails there only on Mesa.
func TestRenderPassDropsAttachmentsOfEarlierPass(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, func(t *testing.T, device *wgpu.Device) {
		for _, second := range []string{"depth", "color"} {
			t.Run(second, func(t *testing.T) {
				color := newDepthArrayTexture(t, device, "stale-color", gputypes.TextureFormatRGBA8Unorm,
					wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc, 2, 1)
				first := &wgpu.RenderPassDescriptor{ColorAttachments: []wgpu.RenderPassColorAttachment{
					staleColorAttachment(newSubresourceView(t, device, color, 1, 0)),
				}}
				firstPipeline := newStalePipeline(t, device, 1, second == "depth")
				if second == "depth" {
					_, depthView := newReadbackTexture(t, device, "stale-depth", gputypes.TextureFormatDepth32Float,
						wgpu.TextureUsageRenderAttachment)
					first.DepthStencilAttachment = &wgpu.RenderPassDepthStencilAttachment{
						View:        depthView,
						DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
					}
				} else {
					_, extraView := newReadbackTexture(t, device, "stale-color-1", gputypes.TextureFormatRGBA8Unorm,
						wgpu.TextureUsageRenderAttachment)
					first.ColorAttachments = append(first.ColorAttachments, staleColorAttachment(extraView))
					firstPipeline = newStalePipeline(t, device, 2, false)
				}
				full := &wgpu.RenderPassDescriptor{ColorAttachments: []wgpu.RenderPassColorAttachment{
					staleColorAttachment(newSubresourceView(t, device, color, 0, 0)),
				}}
				fullPipeline := newStalePipeline(t, device, 1, false)
				submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
					drawStalePass(t, enc, first, firstPipeline)
					drawStalePass(t, enc, full, fullPipeline)
				})
				got := readLayered(t, device, color, 0, 0,
					wgpu.Extent3D{Width: staleColorSize, Height: staleColorSize, DepthOrArrayLayers: 1})
				checkStaleGreen(t, got)
			})
		}
	})
}

// staleColorAttachment is a color attachment of view cleared to transparent
// black.
func staleColorAttachment(view *wgpu.TextureView) wgpu.RenderPassColorAttachment {
	return wgpu.RenderPassColorAttachment{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}
}

// drawStalePass records the pass desc drawing the full-screen triangle of
// pipeline.
func drawStalePass(t *testing.T, enc *wgpu.CommandEncoder, desc *wgpu.RenderPassDescriptor, pipeline *wgpu.RenderPipeline) {
	t.Helper()
	pass, err := enc.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
}

// checkStaleGreen checks that every texel of a staleColorSize square read at
// layeredRowStride is opaque green.
func checkStaleGreen(t *testing.T, got []byte) {
	t.Helper()
	want := []byte{0, 255, 0, 255}
	for y := 0; y < staleColorSize; y++ {
		for x := 0; x < staleColorSize; x++ {
			off := y*layeredRowStride + x*4
			if texel := got[off : off+4]; !bytes.Equal(texel, want) {
				t.Fatalf("mip 0 texel (%d, %d) = %v, want %v", x, y, texel, want)
			}
		}
	}
}

// newStalePipeline creates a pipeline whose full-screen triangle paints
// targets RGBA8Unorm color targets green and, when depth is set, has a
// Depth32Float depth state that writes depth 0.5.
func newStalePipeline(t *testing.T, device *wgpu.Device, targets int, depth bool) *wgpu.RenderPipeline {
	t.Helper()
	outputs := make([]string, 0, targets)
	values := make([]string, 0, targets)
	colorTargets := make([]gputypes.ColorTargetState, targets)
	for i := range colorTargets {
		colorTargets[i] = gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}
		outputs = append(outputs, fmt.Sprintf("@location(%d) c%d: vec4<f32>", i, i))
		values = append(values, "vec4<f32>(0.0, 1.0, 0.0, 1.0)")
	}
	wgsl := fmt.Sprintf(`
struct Out { %s }

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
	return vec4<f32>(p[i], 0.5, 1.0);
}

@fragment
fn fs() -> Out {
	return Out(%s);
}
`, strings.Join(outputs, ", "), strings.Join(values, ", "))
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "stale", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "stale-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	desc := &wgpu.RenderPipelineDescriptor{
		Label:       "stale",
		Layout:      layout,
		Vertex:      wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment:    &wgpu.FragmentState{Module: shader, EntryPoint: "fs", Targets: colorTargets},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	}
	if depth {
		desc.DepthStencil = &wgpu.DepthStencilState{
			Format:            gputypes.TextureFormatDepth32Float,
			DepthWriteEnabled: true,
			DepthCompare:      gputypes.CompareFunctionAlways,
			StencilFront:      wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
			StencilBack:       wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
		}
	}
	pipeline, err := device.CreateRenderPipeline(desc)
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}
