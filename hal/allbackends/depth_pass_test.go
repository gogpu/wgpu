// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDepthOnlyRenderPass renders a shadow-map style pass: a depth attachment
// and no color attachment, cleared to 1 and covered by a triangle that writes
// its depth, then reads the depth back. GLES bound a framebuffer and set the
// viewport only for a pass with color attachments, so a depth-only pass
// cleared and drew into whatever framebuffer was bound before it.
func TestDepthOnlyRenderPass(t *testing.T) {
	formats := []gputypes.TextureFormat{
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8,
	}
	d32s8 := gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	forEachOverrideBackendFeatures(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, d32s8, func(t *testing.T, device *wgpu.Device) {
		software := strings.HasSuffix(t.Name(), "/software")
		for _, format := range formats {
			t.Run(format.String(), func(t *testing.T) {
				if format == gputypes.TextureFormatDepth32FloatStencil8 &&
					!device.Features().Contains(gputypes.FeatureDepth32FloatStencil8) {
					t.Skip("the adapter has no depth32float-stencil8")
				}
				if software {
					t.Skip("software: a depth-only pass writes no depth, P1-SW-021")
				}
				depth, view := newReadbackTexture(t, device, "depth-only", format,
					wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
				pipeline := newDepthWritePipeline(t, device, format, depthPassValue(format))
				submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
					drawDepthOnly(t, enc, pipeline, view, format)
				})
				data := readAspect(t, device, depth, gputypes.TextureAspectDepthOnly)
				checkDepthReadback(t, format, gputypes.TextureAspectDepthOnly, data)
			})
		}
	})
}

// depthPassValue is the depth the triangle writes: the value
// checkDepthReadback expects for format.
func depthPassValue(format gputypes.TextureFormat) float32 {
	if format == gputypes.TextureFormatDepth16Unorm {
		return depthReadbackDepth16
	}
	return depthReadbackDepth32
}

// drawDepthOnly records a pass with only view as its depth attachment: the
// depth is cleared to 1 and the full-screen triangle of pipeline is drawn.
func drawDepthOnly(t *testing.T, enc *wgpu.CommandEncoder, pipeline *wgpu.RenderPipeline, view *wgpu.TextureView,
	format gputypes.TextureFormat) {
	t.Helper()
	ds := &wgpu.RenderPassDepthStencilAttachment{
		View:        view,
		DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
	}
	if format.HasStencil() {
		ds.StencilLoadOp, ds.StencilStoreOp = gputypes.LoadOpClear, gputypes.StoreOpStore
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{DepthStencilAttachment: ds})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
}

// newDepthWritePipeline creates a pipeline with no color targets whose
// full-screen triangle writes depth z into a format attachment. The fragment
// stage is empty, so the pipeline has only the depth output.
func newDepthWritePipeline(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, z float32) *wgpu.RenderPipeline {
	t.Helper()
	wgsl := fmt.Sprintf(`
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
	return vec4<f32>(p[i], %v, 1.0);
}

@fragment
fn fs() {}
`, z)
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "depth-write", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-write-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:    "depth-write",
		Layout:   layout,
		Vertex:   wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{Module: shader, EntryPoint: "fs"},
		DepthStencil: &wgpu.DepthStencilState{
			Format:            format,
			DepthWriteEnabled: true,
			DepthCompare:      gputypes.CompareFunctionAlways,
			StencilFront:      wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
			StencilBack:       wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}
