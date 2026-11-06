// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestIntegerTargetClearReadback clears an integer color target and reads the
// texels back: each channel holds the clear value, in the format's channel
// width. A copy round trip cannot see a clear that writes the float bits of
// the clear value into an integer target, or a format stored as another one
// of the same texel size; this can.
func TestIntegerTargetClearReadback(t *testing.T) {
	unsigned := gputypes.Color{R: 5, G: 6, B: 7, A: 2}
	signed := gputypes.Color{R: 5, G: -6, B: 7, A: -2}
	formats := []struct {
		format gputypes.TextureFormat
		color  gputypes.Color
	}{
		{gputypes.TextureFormatR8Uint, unsigned}, {gputypes.TextureFormatR8Sint, signed},
		{gputypes.TextureFormatR16Uint, unsigned}, {gputypes.TextureFormatR16Sint, signed},
		{gputypes.TextureFormatR32Uint, unsigned}, {gputypes.TextureFormatR32Sint, signed},
		{gputypes.TextureFormatRG8Uint, unsigned}, {gputypes.TextureFormatRG8Sint, signed},
		{gputypes.TextureFormatRG16Uint, unsigned}, {gputypes.TextureFormatRG16Sint, signed},
		{gputypes.TextureFormatRG32Uint, unsigned}, {gputypes.TextureFormatRG32Sint, signed},
		{gputypes.TextureFormatRGBA8Uint, unsigned}, {gputypes.TextureFormatRGBA8Sint, signed},
		{gputypes.TextureFormatRGBA16Uint, unsigned}, {gputypes.TextureFormatRGBA16Sint, signed},
		{gputypes.TextureFormatRGBA32Uint, unsigned}, {gputypes.TextureFormatRGBA32Sint, signed},
		{gputypes.TextureFormatRGB10A2Uint, unsigned},
	}
	const width, height, rowStride = 4, 2, 256
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		if strings.HasSuffix(t.Name(), "/software") {
			t.Skip("software: an integer target is cleared to the float bits, P1-SW-021")
		}
		if strings.HasSuffix(t.Name(), "/gles") {
			t.Skip("gles: an integer target is cleared to the float bits, P1-GLES-022")
		}
		for _, f := range formats {
			t.Run(f.format.String(), func(t *testing.T) {
				texel := integerTexel(f.format, f.color)
				out := clearTargetReadback(t, device, f.format, f.color, width, height, rowStride)
				want := bytes.Repeat(texel, width)
				for y := range height {
					if got := out[y*rowStride : y*rowStride+len(want)]; !bytes.Equal(got, want) {
						t.Errorf("row %d = % x, want % x", y, got, want)
					}
				}
			})
		}
	})
}

// integerTexel encodes c as one texel of an integer format: little-endian
// channels of the format's width, or the packed 10-10-10-2 word.
func integerTexel(format gputypes.TextureFormat, c gputypes.Color) []byte {
	channels := []int64{int64(c.R), int64(c.G), int64(c.B), int64(c.A)}
	if format == gputypes.TextureFormatRGB10A2Uint {
		word := uint32(channels[0]) | uint32(channels[1])<<10 | uint32(channels[2])<<20 | uint32(channels[3])<<30
		return binary.LittleEndian.AppendUint32(nil, word)
	}
	n := int(format.Components())
	width := int(format.BlockCopySize()) / n
	texel := make([]byte, 0, width*n)
	for _, v := range channels[:n] {
		for b := range width {
			texel = append(texel, byte(uint64(v)>>(8*b)))
		}
	}
	return texel
}

func clearTargetReadback(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, color gputypes.Color, width, height, rowStride uint32) []byte {
	t.Helper()
	size := wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1}
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "integer-target",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
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

	staging := newPublicBuffer(t, device, uint64(rowStride*height), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore, ClearValue: color,
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: rowStride, RowsPerImage: height},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
			Size:         size,
		}})
	})
	out := make([]byte, 0, rowStride*height)
	for _, w := range readPublicBuffer(t, staging, int(rowStride*height/4)) {
		out = binary.LittleEndian.AppendUint32(out, w)
	}
	return out
}
