// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestEncoderCommandInsideOpenPass calls each command encoder command while a
// render or compute pass of the encoder is open. The WebGPU spec locks the
// encoder while a pass is open, and a command of the locked encoder
// invalidates it, so Finish must fail (Rust wgpu-core: EncoderStateError::
// Locked). The command used to be recorded into the backend's open pass,
// where a Vulkan transfer command is invalid and a GLES copy leaves the pass's
// framebuffer bound.
func TestEncoderCommandInsideOpenPass(t *testing.T) {
	var other *wgpu.Buffer
	commands := []struct {
		name string
		call func(enc *wgpu.CommandEncoder, buf *wgpu.Buffer, tex *wgpu.Texture)
	}{
		{"copy-buffer-to-buffer", func(enc *wgpu.CommandEncoder, buf *wgpu.Buffer, _ *wgpu.Texture) {
			enc.CopyBufferToBuffer(buf, 0, other, 0, 256)
		}},
		{"copy-buffer-to-texture", func(enc *wgpu.CommandEncoder, buf *wgpu.Buffer, tex *wgpu.Texture) {
			enc.CopyBufferToTexture(buf, tex, []wgpu.BufferTextureCopy{lockedTestRegion(tex)})
		}},
		{"copy-texture-to-buffer", func(enc *wgpu.CommandEncoder, buf *wgpu.Buffer, tex *wgpu.Texture) {
			enc.CopyTextureToBuffer(tex, buf, []wgpu.BufferTextureCopy{lockedTestRegion(tex)})
		}},
		{"clear-buffer", func(enc *wgpu.CommandEncoder, buf *wgpu.Buffer, _ *wgpu.Texture) {
			enc.ClearBuffer(buf, 0, 256)
		}},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		buf := newPublicBuffer(t, device, 4096, wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		other = newPublicBuffer(t, device, 4096, wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		tex, _ := newReadbackTexture(t, device, "locked-copy", gputypes.TextureFormatRGBA8Unorm,
			wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
		target := newRenderTargetView(t, device, wgpu.TextureUsageRenderAttachment)
		for _, c := range commands {
			// The command itself is valid: outside a pass Finish accepts it.
			t.Run(c.name+"/outside", func(t *testing.T) {
				enc := newEncoder(t, device)
				c.call(enc, buf, tex)
				cb, err := enc.Finish()
				if err != nil {
					t.Fatalf("Finish: %v", err)
				}
				cb.Release()
			})
			t.Run(c.name+"/render", func(t *testing.T) {
				enc := newEncoder(t, device)
				pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
					ColorAttachments: []wgpu.RenderPassColorAttachment{{
						View: target, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					}},
				})
				if err != nil {
					t.Fatalf("BeginRenderPass: %v", err)
				}
				c.call(enc, buf, tex)
				_ = pass.End()
				expectFinishFails(t, enc)
			})
			t.Run(c.name+"/compute", func(t *testing.T) {
				enc := newEncoder(t, device)
				pass, err := enc.BeginComputePass(nil)
				if err != nil {
					t.Fatalf("BeginComputePass: %v", err)
				}
				c.call(enc, buf, tex)
				_ = pass.End()
				expectFinishFails(t, enc)
			})
		}
	})
}

func lockedTestRegion(tex *wgpu.Texture) wgpu.BufferTextureCopy {
	return wgpu.BufferTextureCopy{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: depthReadbackSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         depthReadbackExtent,
	}
}

func expectFinishFails(t *testing.T, enc *wgpu.CommandEncoder) {
	t.Helper()
	cb, err := enc.Finish()
	if err == nil {
		cb.Release()
		t.Fatal("Finish accepted a command recorded while a pass was open")
	}
}
