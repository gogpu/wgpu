// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestTextureCopyRoundTrip uploads a 2x2 RGBA8 texture with four different
// colors, with WriteTexture and with CopyBufferToTexture from rows 256 bytes
// apart, and reads it back with a 256-byte bytesPerRow, as multi-row
// buffer/texture copies require. Every channel and every row must come back
// where it was written. GLES used to read with GL_BGRA (R and B swapped),
// flip the rows, ignore bytesPerRow in both directions and return zeros when
// the copy did not fill the whole buffer; a single-color or green-only check
// cannot see any of that.
func TestTextureCopyRoundTrip(t *testing.T) {
	const (
		width, height = 2, 2
		tightRowBytes = width * 4
		paddedRow     = 256
		bufferSize    = paddedRow * height
	)
	rows := [height][]byte{
		{255, 0, 0, 255 /* red */, 0, 255, 0, 255 /* green */},
		{0, 0, 255, 255 /* blue */, 10, 20, 30, 40},
	}
	size := wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1}
	padded := make([]byte, bufferSize)
	for y, r := range rows {
		copy(padded[y*paddedRow:], r)
	}

	uploads := []struct {
		name   string
		upload func(t *testing.T, device *wgpu.Device, tex *wgpu.Texture)
	}{
		{"WriteTexture", func(t *testing.T, device *wgpu.Device, tex *wgpu.Texture) {
			tight := append(append([]byte{}, rows[0]...), rows[1]...)
			if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, tight,
				&wgpu.ImageDataLayout{BytesPerRow: tightRowBytes, RowsPerImage: height}, &size); err != nil {
				t.Fatalf("WriteTexture: %v", err)
			}
		}},
		{"CopyBufferToTexture", func(t *testing.T, device *wgpu.Device, tex *wgpu.Texture) {
			src := newPublicBuffer(t, device, bufferSize, wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
			if err := device.Queue().WriteBuffer(src, 0, padded); err != nil {
				t.Fatalf("WriteBuffer: %v", err)
			}
			submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
				enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
					BufferLayout: wgpu.ImageDataLayout{BytesPerRow: paddedRow, RowsPerImage: height},
					TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
					Size:         size,
				}})
			})
		}},
	}

	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, u := range uploads {
			t.Run(u.name, func(t *testing.T) {
				tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
					Label:         "copy-roundtrip",
					Size:          size,
					MipLevelCount: 1,
					SampleCount:   1,
					Dimension:     gputypes.TextureDimension2D,
					Format:        gputypes.TextureFormatRGBA8Unorm,
					Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc | wgpu.TextureUsageRenderAttachment,
				})
				if err != nil {
					t.Fatalf("CreateTexture: %v", err)
				}
				t.Cleanup(tex.Release)

				u.upload(t, device, tex)

				const words = bufferSize / 4
				staging := newPublicBuffer(t, device, bufferSize, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
				submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
					enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
						BufferLayout: wgpu.ImageDataLayout{BytesPerRow: paddedRow, RowsPerImage: height},
						TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
						Size:         size,
					}})
				})

				out := make([]byte, 0, bufferSize)
				for _, w := range readPublicBuffer(t, staging, words) {
					out = binary.LittleEndian.AppendUint32(out, w)
				}
				for y, want := range rows {
					if got := out[y*paddedRow : y*paddedRow+tightRowBytes]; !bytes.Equal(got, want) {
						t.Errorf("row %d = %v, want %v", y, got, want)
					}
				}
			})
		}
	})
}

func submitCopy(t *testing.T, device *wgpu.Device, record func(enc *wgpu.CommandEncoder)) {
	t.Helper()
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	record(enc)
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}
