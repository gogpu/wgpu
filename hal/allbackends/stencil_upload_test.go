// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// stencilUploadFormats are the formats with a stencil aspect a buffer copy
// may write.
var stencilUploadFormats = []gputypes.TextureFormat{
	gputypes.TextureFormatStencil8,
	gputypes.TextureFormatDepth24PlusStencil8,
	gputypes.TextureFormatDepth32FloatStencil8,
}

// forEachStencilUpload runs fn for every backend, stencil format and upload
// path (WriteTexture or CopyBufferToTexture). formats limits the formats.
func forEachStencilUpload(t *testing.T, formats []gputypes.TextureFormat,
	fn func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viaWriteTexture bool)) {
	t.Helper()
	d32s8 := gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	forEachOverrideBackendFeatures(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, d32s8, func(t *testing.T, device *wgpu.Device) {
		software := strings.HasSuffix(t.Name(), "/software")
		for _, format := range formats {
			for _, path := range []string{"write-texture", "copy-buffer"} {
				t.Run(format.String()+"/"+path, func(t *testing.T) {
					if format == gputypes.TextureFormatDepth32FloatStencil8 &&
						!device.Features().Contains(gputypes.FeatureDepth32FloatStencil8) {
						t.Skip("the adapter has no depth32float-stencil8")
					}
					if software && format != gputypes.TextureFormatDepth32FloatStencil8 {
						t.Skip("software: stencil aspect is not stored, P1-SW-021")
					}
					fn(t, device, format, path == "write-texture")
				})
			}
		}
	})
}

// TestStencilUploadSubresource uploads stencil values into mip level 1 of
// array layers 1 and 2 of a 3-layer 2D array texture, after a background
// value was written to that level of all three layers, and reads every layer
// of the level back. A depth/stencil copy always covers whole mip levels
// (WebGPU "validating texture copy range"), so the region is the level's
// full extent. On the Intel Windows Vulkan driver the copy permuted the
// layers (Vulkan splitDepthStencilLayerCopies).
func TestStencilUploadSubresource(t *testing.T) {
	const background = 0xA5
	const texels = depthReadbackSize * depthReadbackSize
	forEachStencilUpload(t, stencilUploadFormats, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viaWriteTexture bool) {
		tex := newDepthArrayTexture(t, device, "stencil-subresource", format,
			wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst, 2, 3)
		level := wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: 3}
		uploadStencil(t, device, tex, 1, wgpu.Origin3D{}, level, bytes.Repeat([]byte{background}, texels*3), viaWriteTexture)

		pattern := make([]byte, texels*2)
		for i := range pattern {
			pattern[i] = byte(0x10 + i)
		}
		level.DepthOrArrayLayers = 2
		uploadStencil(t, device, tex, 1, wgpu.Origin3D{Z: 1}, level, pattern, viaWriteTexture)

		for layer := uint32(0); layer < 3; layer++ {
			got := readSubresource(t, device, tex, gputypes.TextureAspectStencilOnly, 1, layer)
			for i := 0; i < texels; i++ {
				want := byte(background)
				if layer > 0 {
					want = pattern[int(layer-1)*texels+i]
				}
				if v := got[(i/depthReadbackSize)*depthReadbackBytesPerRow+i%depthReadbackSize]; v != want {
					t.Fatalf("layer %d texel %d = %#x, want %#x", layer, i, v, want)
				}
			}
		}
	})
}

// TestStencilReadbackLayers writes mip level 1 of each layer of a 4-layer
// stencil array texture with its own copy, then reads layers 1..3 back with
// one copy, the texture-to-buffer direction of the multi-layer copy that
// TestStencilUploadSubresource writes. Both directions go through Vulkan's
// splitDepthStencilLayerCopies: the Intel Windows Vulkan driver permuted the
// layers of a multi-layer stencil copy at mip level > 0 and base layer > 0.
func TestStencilReadbackLayers(t *testing.T) {
	const texels = depthReadbackSize * depthReadbackSize
	forEachStencilUpload(t, stencilUploadFormats, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viaWriteTexture bool) {
		tex := newDepthArrayTexture(t, device, "stencil-readback-layers", format,
			wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst, 2, 4)
		level := wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: 1}
		for layer := uint32(0); layer < 4; layer++ {
			uploadStencil(t, device, tex, 1, wgpu.Origin3D{Z: layer}, level,
				bytes.Repeat([]byte{byte(0x10 * (layer + 1))}, texels), viaWriteTexture)
		}
		const layers = 3
		bufSize := uint64(depthReadbackBytesPerRow * depthReadbackSize * layers)
		staging := newPublicBuffer(t, device, bufSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
				BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: depthReadbackSize},
				TextureBase: wgpu.ImageCopyTexture{
					Texture: tex, MipLevel: 1, Origin: wgpu.Origin3D{Z: 1}, Aspect: gputypes.TextureAspectStencilOnly,
				},
				Size: wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: layers},
			}})
		})
		got := mapStaging(t, staging, bufSize)
		for k := 0; k < layers; k++ {
			want := byte(0x10 * (k + 2))
			for i := 0; i < texels; i++ {
				off := (k*depthReadbackSize+i/depthReadbackSize)*depthReadbackBytesPerRow + i%depthReadbackSize
				if got[off] != want {
					t.Fatalf("layer %d texel %d = %#x, want %#x", k+1, i, got[off], want)
				}
			}
		}
	})
}

// TestStencilUploadOrder writes the stencil of a texture twice with no
// submit between: the second upload must land after the first.
func TestStencilUploadOrder(t *testing.T) {
	const texels = depthReadbackSize * depthReadbackSize
	forEachStencilUpload(t, stencilUploadFormats, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viaWriteTexture bool) {
		tex, _ := newReadbackTexture(t, device, "stencil-order", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
		second := make([]byte, texels)
		for i := range second {
			second[i] = byte(0x40 + i)
		}
		uploadStencil(t, device, tex, 0, wgpu.Origin3D{}, depthReadbackExtent, bytes.Repeat([]byte{0x11}, texels), viaWriteTexture)
		uploadStencil(t, device, tex, 0, wgpu.Origin3D{}, depthReadbackExtent, second, viaWriteTexture)
		got := readAspect(t, device, tex, gputypes.TextureAspectStencilOnly)
		for i := 0; i < texels; i++ {
			if v := got[(i/depthReadbackSize)*depthReadbackBytesPerRow+i%depthReadbackSize]; v != second[i] {
				t.Fatalf("texel %d = %#x, want %#x", i, v, second[i])
			}
		}
	})
}

// TestStencilUploadKeepsDepth clears the depth of a combined depth/stencil
// texture to 0.5 in a render pass, uploads stencil values, and draws a green
// triangle at depth 0.25 with depth test Less and stencil test Equal 1. Only
// the texels whose uploaded stencil is 1 may turn green, and every one of
// them must: the stencil upload must keep the depth (a draw-based upload that
// writes its triangle's depth would fail the depth test).
func TestStencilUploadKeepsDepth(t *testing.T) {
	formats := []gputypes.TextureFormat{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32FloatStencil8}
	forEachStencilUpload(t, formats, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viaWriteTexture bool) {
		depth, depthView := newReadbackTexture(t, device, "stencil-keeps-depth", format,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
		color, colorView := newReadbackTexture(t, device, "stencil-keeps-depth-color", gputypes.TextureFormatRGBA8Unorm,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		pipeline := newStencilEqualPipeline(t, device, format)

		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
				DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
					View:        depthView,
					DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 0.5,
					StencilLoadOp: gputypes.LoadOpClear, StencilStoreOp: gputypes.StoreOpStore,
				},
			})
			if err != nil {
				t.Fatalf("BeginRenderPass: %v", err)
			}
			if err := pass.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
		})

		stencil := make([]byte, depthReadbackSize*depthReadbackSize)
		for i := range stencil {
			stencil[i] = byte(i % 3)
		}
		uploadStencil(t, device, depth, 0, wgpu.Origin3D{}, depthReadbackExtent, stencil, viaWriteTexture)

		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
				ColorAttachments: []wgpu.RenderPassColorAttachment{{
					View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					ClearValue: gputypes.Color{A: 1},
				}},
				DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
					View:        depthView,
					DepthLoadOp: gputypes.LoadOpLoad, DepthStoreOp: gputypes.StoreOpStore,
					StencilLoadOp: gputypes.LoadOpLoad, StencilStoreOp: gputypes.StoreOpStore,
				},
			})
			if err != nil {
				t.Fatalf("BeginRenderPass: %v", err)
			}
			pass.SetPipeline(pipeline)
			pass.SetStencilReference(1)
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
			if err := pass.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
		})

		got := readAspect(t, device, color, gputypes.TextureAspectAll)
		for i := 0; i < depthReadbackSize*depthReadbackSize; i++ {
			px := got[(i/depthReadbackSize)*depthReadbackBytesPerRow+(i%depthReadbackSize)*4:][:4]
			if green := px[1] > 128; green != (stencil[i] == 1) {
				t.Fatalf("texel %d (stencil %d) = %v, want green %v", i, stencil[i], px, stencil[i] == 1)
			}
		}
		if format == gputypes.TextureFormatDepth32FloatStencil8 {
			data := readAspect(t, device, depth, gputypes.TextureAspectDepthOnly)
			for i := 0; i < depthReadbackSize*depthReadbackSize; i++ {
				off := (i/depthReadbackSize)*depthReadbackBytesPerRow + (i%depthReadbackSize)*4
				if v := math.Float32frombits(binary.LittleEndian.Uint32(data[off:])); v != 0.5 {
					t.Fatalf("depth texel %d = %v, want 0.5", i, v)
				}
			}
		}
	})
}

// newStencilEqualPipeline draws a viewport-covering green triangle at depth
// 0.25 with depth test Less, no depth write, and stencil test Equal.
func newStencilEqualPipeline(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "stencil-equal", WGSL: `
@vertex fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
    let uv = vec2f(f32((i << 1u) & 2u), f32(i & 2u));
    return vec4f(uv * 2.0 - 1.0, 0.25, 1.0);
}
@fragment fn fs() -> @location(0) vec4f { return vec4f(0.0, 1.0, 0.0, 1.0); }
`})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "stencil-equal"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	equal := wgpu.StencilFaceState{
		Compare: gputypes.CompareFunctionEqual,
		FailOp:  gputypes.StencilOperationKeep, DepthFailOp: gputypes.StencilOperationKeep, PassOp: gputypes.StencilOperationKeep,
	}
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "stencil-equal",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
		Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		DepthStencil: &wgpu.DepthStencilState{
			Format:           format,
			DepthCompare:     gputypes.CompareFunctionLess,
			StencilFront:     equal,
			StencilBack:      equal,
			StencilReadMask:  0xFF,
			StencilWriteMask: 0,
		},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// uploadStencil writes tight (rows of size.Width bytes, size.Height rows per
// layer) into the stencil aspect of mip level mip of tex at origin, through
// WriteTexture or through CopyBufferToTexture at 256-byte rows.
func uploadStencil(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, mip uint32, origin wgpu.Origin3D,
	size wgpu.Extent3D, tight []byte, viaWriteTexture bool) {
	t.Helper()
	dst := wgpu.ImageCopyTexture{Texture: tex, MipLevel: mip, Origin: origin, Aspect: gputypes.TextureAspectStencilOnly}
	if viaWriteTexture {
		layout := wgpu.ImageDataLayout{BytesPerRow: size.Width, RowsPerImage: size.Height}
		if err := device.Queue().WriteTexture(&dst, tight, &layout, &size); err != nil {
			t.Fatalf("WriteTexture: %v", err)
		}
		return
	}
	rows := int(size.Height * size.DepthOrArrayLayers)
	padded := make([]byte, depthReadbackBytesPerRow*rows)
	for r := 0; r < rows; r++ {
		copy(padded[r*depthReadbackBytesPerRow:], tight[r*int(size.Width):(r+1)*int(size.Width)])
	}
	src := newPublicBuffer(t, device, uint64(len(padded)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(src, 0, padded); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: size.Height},
			TextureBase:  dst,
			Size:         size,
		}})
	})
}
