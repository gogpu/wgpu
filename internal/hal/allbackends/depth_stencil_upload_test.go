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

// TestDepthUploadPaddedRows uploads a Depth16Unorm texture whose rows are 256
// bytes apart in the source, twice the 128 bytes a 64-texel row holds, with
// WriteTexture and with CopyBufferToTexture, and samples row 2 in a compute
// shader. Every row holds its own depth, so rows read tightly packed land one
// row off. GLES set UNPACK_ROW_LENGTH from the size of a color pixel, which
// is 0 for DEPTH_COMPONENT, so GL read the padded rows as tightly packed.
func TestDepthUploadPaddedRows(t *testing.T) {
	required := gputypes.DownlevelFlagsComputeShaders | gputypes.DownlevelFlagsDepthTextureAndBufferCopies
	forEachOverrideBackend(t, required, func(t *testing.T, device *wgpu.Device) {
		for _, upload := range []string{"WriteTexture", "CopyBufferToTexture"} {
			t.Run(upload, func(t *testing.T) {
				depthUploadPadded(t, device, upload)
			})
		}
	})
}

// depthRowValue is the Depth16Unorm value stored in row y.
func depthRowValue(y uint32) uint16 { return uint16((y + 1) * 0x1000) }

func depthUploadPadded(t *testing.T, device *wgpu.Device, upload string) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "depth-upload",
		Size:          wgpu.Extent3D{Width: depthRigWidth, Height: depthRigHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatDepth16Unorm,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)

	const stride = 256
	img := make([]byte, stride*depthRigHeight)
	for y := uint32(0); y < depthRigHeight; y++ {
		for x := uint32(0); x < depthRigWidth; x++ {
			binary.LittleEndian.PutUint16(img[y*stride+x*2:], depthRowValue(y))
		}
	}
	size := wgpu.Extent3D{Width: depthRigWidth, Height: depthRigHeight, DepthOrArrayLayers: 1}
	layout := wgpu.ImageDataLayout{BytesPerRow: stride, RowsPerImage: depthRigHeight}
	dst := wgpu.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectAll}
	if upload == "WriteTexture" {
		if err := device.Queue().WriteTexture(&dst, img, &layout, &size); err != nil {
			t.Fatalf("WriteTexture: %v", err)
		}
	} else {
		src := newPublicBuffer(t, device, uint64(len(img)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		if err := device.Queue().WriteBuffer(src, 0, img); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{BufferLayout: layout, TextureBase: dst, Size: size}})
		})
	}

	// sampleDepth reads texel (8, 2).
	r := &depthRig{device: device, depthView: view}
	got := r.sampleDepth(t)
	want := float32(depthRowValue(2)) / 65535
	if diff := got - want; diff > 1e-4 || diff < -1e-4 {
		t.Fatalf("depth at row 2 = %v, want %v (row 1 holds %v)", got, want, float32(depthRowValue(1))/65535)
	}
}

// TestStencil8Attachment draws cullShader's left triangle into a pass whose
// depth/stencil attachment is a Stencil8 texture. GLES had no mapping for
// Stencil8 and created an RGBA8 texture, which made the framebuffer
// incomplete at STENCIL_ATTACHMENT, so the pass drew nothing into its color
// target. Rust wgpu-hal gles/conv.rs maps Stencil8 to STENCIL_INDEX8.
func TestStencil8Attachment(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		size := wgpu.Extent3D{Width: depthRigWidth, Height: depthRigHeight, DepthOrArrayLayers: 1}
		newView := func(label string, format gputypes.TextureFormat, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
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
		target, targetView := newView("stencil8-target", gputypes.TextureFormatRGBA8Unorm,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		_, stencilView := newView("stencil8", gputypes.TextureFormatStencil8, wgpu.TextureUsageRenderAttachment)

		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "stencil8", WGSL: cullShader})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(shader.Release)
		layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "stencil8"})
		if err != nil {
			t.Fatalf("CreatePipelineLayout: %v", err)
		}
		t.Cleanup(layout.Release)
		pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "stencil8",
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
				Format:       gputypes.TextureFormatStencil8,
				DepthCompare: gputypes.CompareFunctionAlways,
				StencilFront: keepStencil,
				StencilBack:  keepStencil,
			},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline: %v", err)
		}
		t.Cleanup(pipeline.Release)

		staging := newPublicBuffer(t, device, depthRigWidth*depthRigHeight*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
				ColorAttachments: []wgpu.RenderPassColorAttachment{{
					View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
				}},
				DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
					View: stencilView, StencilLoadOp: gputypes.LoadOpClear, StencilStoreOp: gputypes.StoreOpStore,
				},
			})
			if err != nil {
				t.Fatalf("BeginRenderPass: %v", err)
			}
			pass.SetPipeline(pipeline)
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
			if err := pass.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
			enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
				BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthRigWidth * 4, RowsPerImage: depthRigHeight},
				TextureBase:  wgpu.ImageCopyTexture{Texture: target},
				Size:         size,
			}})
		})
		words := readPublicBuffer(t, staging, depthRigWidth*depthRigHeight)
		var px [4]byte
		binary.LittleEndian.PutUint32(px[:], words[2*depthRigWidth+8])
		if px[0] >= 128 || px[1] <= 128 {
			t.Fatalf("pixel (8, 2) = %v, want the triangle's green", px)
		}
	})
}
