// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestCubeTextureCopyRoundTrip fills a whole mip level of a six-layer 2D
// texture whose view formats name its own format, with WriteTexture and with
// CopyBufferToTexture, and reads it back whole and one face at a time. GLES
// stores such a texture as a cube map (TEXTURE_CUBE_MAP), whose faces GL
// addresses only through the per-face targets TEXTURE_CUBE_MAP_POSITIVE_X + n:
// WriteTexture wrote nothing to it, CopyBufferToTexture passed TEXTURE_CUBE_MAP
// itself to glTexSubImage2D, and a readback attached TEXTURE_CUBE_MAP to the
// framebuffer. Rust wgpu-hal gles/queue.rs get_2d_target picks the face.
func TestCubeTextureCopyRoundTrip(t *testing.T) {
	uploads := []string{"WriteTexture", "CopyBufferToTexture"}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for mip := uint32(0); mip < 2; mip++ {
			for _, upload := range uploads {
				t.Run(fmt.Sprintf("mip%d/%s", mip, upload), func(t *testing.T) {
					cubeRoundTrip(t, device, mip, upload)
				})
			}
		}
	})
}

const cubeFaces = 6

func cubeRoundTrip(t *testing.T, device *wgpu.Device, mip uint32, upload string) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "cube-roundtrip",
		Size:          wgpu.Extent3D{Width: layeredBaseSize, Height: layeredBaseSize, DepthOrArrayLayers: cubeFaces},
		MipLevelCount: 2,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc | wgpu.TextureUsageTextureBinding,
		ViewFormats:   []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm},
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	w, h := uint32(layeredBaseSize>>mip), uint32(layeredBaseSize>>mip)
	img := layeredImage(w, h, cubeFaces, mip)
	size := wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: cubeFaces}
	layout := wgpu.ImageDataLayout{BytesPerRow: layeredRowStride, RowsPerImage: h}
	dst := wgpu.ImageCopyTexture{Texture: tex, MipLevel: mip}
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

	got := readLayered(t, device, tex, mip, 0, size)
	compareLayered(t, got, w, h, cubeFaces, 0, mip)

	// The last face, read on its own: its origin Z selects it.
	got = readLayered(t, device, tex, mip, cubeFaces-1, wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1})
	compareLayered(t, got, w, h, 1, cubeFaces-1, mip)
}
