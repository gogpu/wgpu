// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && linux && !(js && wasm) && wgpu_gles_force_es

package gles

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// openForcedESDevice opens a device on the forced-ES instance context.
func openForcedESDevice(t *testing.T) (*Adapter, hal.Device, hal.Queue) {
	t.Helper()
	inst, err := Backend{}.CreateInstance(nil)
	if err != nil {
		skipUnlessGLESRequired(t, "no EGL: "+err.Error())
	}
	t.Cleanup(inst.Destroy)
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		skipUnlessGLESRequired(t, "no adapter from the instance context")
	}
	adapter, ok := adapters[0].Adapter.(*Adapter)
	if !ok || adapter.ctx == nil {
		skipUnlessGLESRequired(t, "no instance context (Wayland, or EGL without a surfaceless or pbuffer path)")
	}
	dev, err := adapter.Open(gputypes.Features(0), gputypes.Limits{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return adapter, dev.Device, dev.Queue
}

// TestMSAAIsRenderbufferOnES checks that a multisampled render target on an
// OpenGL ES context is a renderbuffer and renders and resolves. ES has no
// glTexImage2DMultisample (desktop only) and ES 3.0 no multisample texture
// images, so CreateTexture used to fail with GL_INVALID_OPERATION 0x502 and
// every pass that resolved MSAA could not be created. Rust wgpu-hal creates
// renderbuffers for render targets (gles/device.rs create_texture).
func TestMSAAIsRenderbufferOnES(t *testing.T) {
	adapter, dev, queue := openForcedESDevice(t)
	if !adapter.caps.IsES {
		t.Fatalf("context version %q is not OpenGL ES under wgpu_gles_force_es", adapter.caps.Version)
	}

	const width, height = 8, 8
	msaa, err := dev.CreateTexture(&hal.TextureDescriptor{
		Label:         "msaa",
		Size:          hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   4,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture(msaa 4x): %v", err)
	}
	t.Cleanup(func() { dev.DestroyTexture(msaa) })
	msaaTex, ok := msaa.(*Texture)
	if !ok {
		t.Fatalf("CreateTexture returned %T, want *Texture", msaa)
	}
	if !msaaTex.renderBuffer {
		t.Errorf("MSAA texture on ES is not a renderbuffer: an ES context has no multisample texture images")
	}
	if msaaTex.sampleCount != 4 {
		t.Errorf("sampleCount = %d, want 4", msaaTex.sampleCount)
	}

	resolve, err := dev.CreateTexture(&hal.TextureDescriptor{
		Label:         "resolve",
		Size:          hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture(resolve): %v", err)
	}
	t.Cleanup(func() { dev.DestroyTexture(resolve) })

	msaaView, err := dev.CreateTextureView(msaa, nil)
	if err != nil {
		t.Fatalf("CreateTextureView(msaa): %v", err)
	}
	resolveView, err := dev.CreateTextureView(resolve, nil)
	if err != nil {
		t.Fatalf("CreateTextureView(resolve): %v", err)
	}

	const bytesPerRow = 256
	staging, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label: "staging",
		Size:  bytesPerRow * height,
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(staging): %v", err)
	}
	t.Cleanup(func() { dev.DestroyBuffer(staging) })

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "msaa"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := enc.BeginEncoding("msaa"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		Label: "msaa",
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View:          msaaView,
			ResolveTarget: resolveView,
			LoadOp:        gputypes.LoadOpClear,
			StoreOp:       gputypes.StoreOpStore,
			ClearValue:    gputypes.Color{R: 1, G: 0.5, B: 0, A: 1},
		}},
	})
	pass.End()
	enc.CopyTextureToBuffer(resolve, staging, []hal.BufferTextureCopy{{
		TextureBase:  hal.ImageCopyTexture{Texture: resolve, MipLevel: 0, Aspect: gputypes.TextureAspectAll},
		BufferLayout: hal.ImageDataLayout{Offset: 0, BytesPerRow: bytesPerRow, RowsPerImage: height},
		Size:         hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := queue.Submit([]hal.CommandBuffer{cmd}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := dev.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}

	m, err := dev.MapBuffer(staging, 0, bytesPerRow*height)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	out := make([]byte, bytesPerRow*height)
	copy(out, unsafe.Slice((*byte)(m.Ptr), len(out)))
	if err := dev.UnmapBuffer(staging); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}
	// The clear value 1, 0.5, 0, 1 in RGBA8.
	want := [4]byte{255, 128, 0, 255}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			i := y*bytesPerRow + x*4
			got := [4]byte{out[i], out[i+1], out[i+2], out[i+3]}
			if got != want {
				t.Fatalf("resolved texel (%d,%d) = %v, want %v: the MSAA renderbuffer pass did not resolve", x, y, got, want)
			}
		}
	}
}
