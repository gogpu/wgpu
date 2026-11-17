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

// TestPipelineZeroValueDefaultsDraw is the regression test for P1-API-022:
// render pipeline fields whose Go zero is an Undefined member the WebGPU spec
// does not have must behave as the spec defaults on every backend.
//
//   - Zero stencil faces are compare "always", ops "keep": DX12 enabled the
//     stencil test with compare NEVER and drew nothing.
//   - An undefined depthCompare without depth writes passes every fragment:
//     DX12 mapped it to NEVER.
//   - A zero blend component is srcFactor "one", dstFactor "zero", operation
//     "add": Vulkan mapped the undefined dstFactor to ONE and added the
//     cleared red to the drawn green.
//
// Each case draws cullShader's left triangle (green) into a 64x4 RGBA8 target
// cleared to red and checks that row 2, x = 8 is pure green.
func TestPipelineZeroValueDefaultsDraw(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		t.Run("zero-stencil-faces", func(t *testing.T) {
			drawZeroDefaults(t, device, zeroDefaultsCase{
				depthFormat: gputypes.TextureFormatDepth24PlusStencil8,
				depthStencil: &wgpu.DepthStencilState{
					Format:            gputypes.TextureFormatDepth24PlusStencil8,
					DepthWriteEnabled: true,
					DepthCompare:      gputypes.CompareFunctionLess,
					StencilReadMask:   0xFF,
					StencilWriteMask:  0xFF,
				},
				depthClear: 1,
			})
		})
		t.Run("undefined-depth-compare", func(t *testing.T) {
			// Depth cleared to 0: an explicit Less (or NEVER) fails the
			// triangle at z = 0; an absent compare passes it.
			drawZeroDefaults(t, device, zeroDefaultsCase{
				depthFormat: gputypes.TextureFormatDepth24Plus,
				depthStencil: &wgpu.DepthStencilState{
					Format:       gputypes.TextureFormatDepth24Plus,
					StencilFront: keepStencil,
					StencilBack:  keepStencil,
				},
				depthClear: 0,
			})
		})
		t.Run("zero-blend-component", func(t *testing.T) {
			drawZeroDefaults(t, device, zeroDefaultsCase{blend: &gputypes.BlendState{}})
		})
	})
}

type zeroDefaultsCase struct {
	depthFormat  gputypes.TextureFormat // Undefined: no depth attachment
	depthStencil *wgpu.DepthStencilState
	depthClear   float32
	blend        *gputypes.BlendState
	writeMask    *gputypes.ColorWriteMask // nil: ColorWriteMaskAll
	stencilRef   uint32
	// wantCleared expects the cleared red at the probe: the draw must not
	// reach the target.
	wantCleared bool
}

func drawZeroDefaults(t *testing.T, device *wgpu.Device, c zeroDefaultsCase) {
	t.Helper()
	target, targetView := newZeroDefaultsTexture(t, device, gputypes.TextureFormatRGBA8Unorm,
		wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	pipeline := newZeroDefaultsPipeline(t, device, c)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	passDesc := &wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
	}
	if c.depthFormat != gputypes.TextureFormatUndefined {
		_, depthView := newZeroDefaultsTexture(t, device, c.depthFormat, wgpu.TextureUsageRenderAttachment)
		ds := &wgpu.RenderPassDepthStencilAttachment{
			View:        depthView,
			DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: c.depthClear,
		}
		if c.depthFormat == gputypes.TextureFormatDepth24PlusStencil8 {
			ds.StencilLoadOp, ds.StencilStoreOp = gputypes.LoadOpClear, gputypes.StoreOpStore
		}
		passDesc.DepthStencilAttachment = ds
	}
	pass, err := enc.BeginRenderPass(passDesc)
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	if c.stencilRef != 0 {
		pass.SetStencilReference(c.stencilRef)
	}
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	staging := newPublicBuffer(t, device, depthRigWidth*depthRigHeight*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthRigWidth * 4, RowsPerImage: depthRigHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
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
	if c.wantCleared {
		if px[0] < 128 || px[1] > 127 || px[2] > 127 {
			t.Fatalf("pixel (8, 2) = %v, want the cleared red: the draw reached the target", px)
		}
		return
	}
	if px[0] > 127 || px[1] < 128 || px[2] > 127 {
		t.Fatalf("pixel (8, 2) = %v, want green: the triangle was not drawn as the spec defaults say", px)
	}
}

func newZeroDefaultsTexture(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "zero-defaults", Size: wgpu.Extent3D{Width: depthRigWidth, Height: depthRigHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: format, Usage: usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture %v: %v", format, err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView %v: %v", format, err)
	}
	t.Cleanup(view.Release)
	return tex, view
}

func newZeroDefaultsPipeline(t *testing.T, device *wgpu.Device, c zeroDefaultsCase) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "zero-defaults", WGSL: cullShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "zero-defaults"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	mask := gputypes.ColorWriteMaskAll
	if c.writeMask != nil {
		mask = *c.writeMask
	}
	p, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "zero-defaults",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, Blend: c.blend, WriteMask: mask},
			},
		},
		Primitive:    gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		DepthStencil: c.depthStencil,
		Multisample:  gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(p.Release)
	return p
}
