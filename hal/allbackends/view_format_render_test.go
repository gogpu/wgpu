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

// TestRenderThroughSrgbView renders into an RGBA8UnormSrgb view of an
// RGBA8Unorm texture that lists the sRGB format in ViewFormats, then reads the
// texture's bytes back. The pass must run in the view's format: the clear and
// the drawn color are sRGB-encoded (a linear 0.5 clear stores 188). Vulkan
// keyed its render pass and framebuffer on the texture's format, so the pass
// was incompatible with the sRGB pipeline (VUID-vkCmdDraw-renderPass-02684,
// VUID-VkFramebufferCreateInfo-pAttachments-00880; P1-VK-015).
func TestRenderThroughSrgbView(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsViewFormats, renderThroughSrgbView)
}

func renderThroughSrgbView(t *testing.T, device *wgpu.Device) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "unorm-with-srgb-view",
		Size:          wgpu.Extent3D{Width: sliceRigWidth, Height: sliceRigHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		ViewFormats:   []gputypes.TextureFormat{gputypes.TextureFormatRGBA8UnormSrgb},
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label: "srgb-view", Format: gputypes.TextureFormatRGBA8UnormSrgb,
		Dimension: gputypes.TextureViewDimension2D, MipLevelCount: 1, ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "left-half", WGSL: leftHalfShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "srgb-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "srgb-target",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8UnormSrgb, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	r := &targetRig{device: device, texture: tex, dimension: gputypes.TextureDimension2D, depth: 1, pipeline: pipeline}
	r.renderView(t, view, nil, gputypes.Color{R: 0.5, G: 0.5, B: 0.5, A: 1})

	staging := newPublicBuffer(t, device, uint64(sliceRigRowPitch*sliceRigHeight), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: sliceRigRowPitch, RowsPerImage: sliceRigHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         wgpu.Extent3D{Width: sliceRigWidth, Height: sliceRigHeight, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	words := readPublicBuffer(t, staging, int(sliceRigRowPitch/4*sliceRigHeight))

	// sRGB encode of linear 0.5 is 0.7354, stored as 188; the drawn green is
	// 0/1, which encodes to itself. Allow 1 for the encoder's rounding.
	wantClear := [4]byte{188, 188, 188, 255}
	for y := uint32(0); y < sliceRigHeight; y++ {
		for _, x := range []uint32{0, sliceRigWidth/2 - 1, sliceRigWidth / 2, sliceRigWidth - 1} {
			var px [4]byte
			binary.LittleEndian.PutUint32(px[:], words[y*sliceRigRowPitch/4+x])
			want := wantClear
			if x < sliceRigWidth/2 {
				want = sliceGreen
			}
			if !bytesNear(px[:], want[:], 1) {
				t.Errorf("texel (%d, %d) = %v, want %v (sRGB-encoded)", x, y, px, want)
			}
		}
	}
}
