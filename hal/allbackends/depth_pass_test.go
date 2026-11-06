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

// TestDepthAttachmentSubresource draws depth into mip level 1 and array layer
// 1 of a 2D array depth texture, with and without a color attachment, and
// reads that subresource back. GLES attached the depth texture of a pass
// with color attachments at level 0 through glFramebufferTexture2D, which
// takes no layer and is invalid for a 2D array texture, so nothing was
// attached and the depth went nowhere.
func TestDepthAttachmentSubresource(t *testing.T) {
	const format = gputypes.TextureFormatDepth32Float
	forEachOverrideBackend(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, func(t *testing.T, device *wgpu.Device) {
		if strings.HasSuffix(t.Name(), "/software") {
			t.Skip("software: a depth-only pass writes no depth, P1-SW-021")
		}
		for _, withColor := range []bool{true, false} {
			name := "depth-only"
			if withColor {
				name = "with-color"
			}
			t.Run(name, func(t *testing.T) {
				depth := newDepthArrayTexture(t, device, "depth-subresource", format,
					wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc, 2, 2)
				depthView := newSubresourceView(t, device, depth, 1, 1)
				var colorView *wgpu.TextureView
				if withColor {
					_, colorView = newReadbackTexture(t, device, "depth-subresource-color",
						gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageRenderAttachment)
				}
				pipeline := newDepthColorPipeline(t, device, format, depthReadbackDepth32, withColor)
				submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
					drawDepth(t, enc, pipeline, depthView, colorView, format)
				})
				data := readSubresource(t, device, depth, gputypes.TextureAspectDepthOnly, 1, 1)
				checkDepthReadback(t, format, gputypes.TextureAspectDepthOnly, data)
			})
		}
	})
}

// newDepthArrayTexture creates a 2D array texture of format whose mip level 1 is
// depthReadbackSize square.
func newDepthArrayTexture(t *testing.T, device *wgpu.Device, label string, format gputypes.TextureFormat,
	usage gputypes.TextureUsage, mips, layers uint32) *wgpu.Texture {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         label,
		Size:          wgpu.Extent3D{Width: 2 * depthReadbackSize, Height: 2 * depthReadbackSize, DepthOrArrayLayers: layers},
		MipLevelCount: mips, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: format, Usage: usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture %s: %v", label, err)
	}
	t.Cleanup(tex.Release)
	return tex
}

// newSubresourceView creates a 2D view of one mip level and array layer of tex.
func newSubresourceView(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, mip, layer uint32) *wgpu.TextureView {
	t.Helper()
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Dimension:    gputypes.TextureViewDimension2D,
		BaseMipLevel: mip, MipLevelCount: 1,
		BaseArrayLayer: layer, ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView mip %d layer %d: %v", mip, layer, err)
	}
	t.Cleanup(view.Release)
	return view
}

// readSubresource copies a depthReadbackSize square of aspect from mip level
// mip and array layer layer of tex into a buffer at depthReadbackBytesPerRow
// and returns the buffer's bytes.
func readSubresource(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, aspect gputypes.TextureAspect, mip, layer uint32) []byte {
	t.Helper()
	bufSize := uint64(depthReadbackBytesPerRow * depthReadbackSize)
	staging := newPublicBuffer(t, device, bufSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: depthReadbackSize},
			TextureBase: wgpu.ImageCopyTexture{
				Texture: tex, MipLevel: mip, Origin: wgpu.Origin3D{Z: layer}, Aspect: aspect,
			},
			Size: depthReadbackExtent,
		}})
	})
	return mapStaging(t, staging, bufSize)
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
	drawDepth(t, enc, pipeline, view, nil, format)
}

// drawDepth records a pass with depthView as its depth attachment and, when
// colorView is not nil, colorView as its color attachment, clears the depth to
// 1 and draws the full-screen triangle of pipeline.
func drawDepth(t *testing.T, enc *wgpu.CommandEncoder, pipeline *wgpu.RenderPipeline, depthView, colorView *wgpu.TextureView,
	format gputypes.TextureFormat) {
	t.Helper()
	desc := &wgpu.RenderPassDescriptor{DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
		View:        depthView,
		DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
	}}
	if format.HasStencil() {
		ds := desc.DepthStencilAttachment
		ds.StencilLoadOp, ds.StencilStoreOp = gputypes.LoadOpClear, gputypes.StoreOpStore
	}
	if colorView != nil {
		desc.ColorAttachments = []wgpu.RenderPassColorAttachment{{
			View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}}
	}
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

// newDepthWritePipeline creates a pipeline with no color targets whose
// full-screen triangle writes depth z into a format attachment. The fragment
// stage is empty, so the pipeline has only the depth output.
func newDepthWritePipeline(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, z float32) *wgpu.RenderPipeline {
	t.Helper()
	return newDepthColorPipeline(t, device, format, z, false)
}

// newDepthColorPipeline is newDepthWritePipeline with, when color is set, an
// RGBA8Unorm target that the triangle paints green.
func newDepthColorPipeline(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, z float32, color bool) *wgpu.RenderPipeline {
	t.Helper()
	fragment := &wgpu.FragmentState{EntryPoint: "fs"}
	if color {
		fragment.EntryPoint = "fs_color"
		fragment.Targets = []gputypes.ColorTargetState{
			{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
		}
	}
	wgsl := fmt.Sprintf(`
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
	return vec4<f32>(p[i], %v, 1.0);
}

@fragment
fn fs() {}

@fragment
fn fs_color() -> @location(0) vec4<f32> {
	return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`, z)
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "depth-write", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	fragment.Module = shader
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-write-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:    "depth-write",
		Layout:   layout,
		Vertex:   wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: fragment,
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
