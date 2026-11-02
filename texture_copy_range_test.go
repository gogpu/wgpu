//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// copyRig is a device with the textures and buffers the copy range tests use.
type copyRig struct {
	device *wgpu.Device
	// color is an 8x8x4 RGBA8 texture with 2 mip levels, srgb the same size as
	// RGBA8UnormSrgb, bgra the same size as BGRA8Unorm, wide a 128x2 RGBA8
	// texture whose row needs more than 256 bytes, depth an 8x8 Depth32Float.
	color, color2, srgb, bgra, wide, depth *wgpu.Texture
	buf                                    *wgpu.Buffer
}

func newCopyRig(t *testing.T) *copyRig {
	t.Helper()
	_, _, device := newDevice(t)
	t.Cleanup(device.Release)
	requireHAL(t, device)
	rig := &copyRig{device: device}
	const copyBoth = wgpu.TextureUsageCopySrc | wgpu.TextureUsageCopyDst
	newTexture := func(format gputypes.TextureFormat, w, h, layers, mips uint32) *wgpu.Texture {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Size:          wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: layers},
			MipLevelCount: mips,
			SampleCount:   1,
			Dimension:     wgpu.TextureDimension2D,
			Format:        format,
			Usage:         copyBoth,
		})
		if err != nil {
			t.Fatalf("CreateTexture(%v): %v", format, err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	rig.color = newTexture(gputypes.TextureFormatRGBA8Unorm, 8, 8, 4, 2)
	rig.color2 = newTexture(gputypes.TextureFormatRGBA8Unorm, 8, 8, 4, 2)
	rig.srgb = newTexture(gputypes.TextureFormatRGBA8UnormSrgb, 8, 8, 4, 2)
	rig.bgra = newTexture(gputypes.TextureFormatBGRA8Unorm, 8, 8, 4, 2)
	rig.wide = newTexture(gputypes.TextureFormatRGBA8Unorm, 128, 2, 1, 1)
	rig.depth = newTexture(gputypes.TextureFormatDepth32Float, 8, 8, 1, 1)
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  16384,
		Usage: wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	t.Cleanup(buf.Release)
	rig.buf = buf
	return rig
}

// finishKind finishes the encoder and returns the kind of the
// *core.TransferError it reports, or -1 when Finish succeeds.
func (r *copyRig) finishKind(t *testing.T, record func(enc *wgpu.CommandEncoder)) core.TransferErrorKind {
	t.Helper()
	enc, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	record(enc)
	cmd, err := enc.Finish()
	if err == nil {
		cmd.Release()
		return -1
	}
	var te *core.TransferError
	if !errors.As(err, &te) {
		t.Fatalf("Finish error = %v (%T), want *core.TransferError", err, err)
	}
	return te.Kind
}

func btCopy(mip uint32, origin wgpu.Origin3D, layout wgpu.ImageDataLayout, size wgpu.Extent3D) []wgpu.BufferTextureCopy {
	return []wgpu.BufferTextureCopy{{
		TextureBase:  wgpu.ImageCopyTexture{MipLevel: mip, Origin: origin, Aspect: gputypes.TextureAspectAll},
		BufferLayout: layout,
		Size:         size,
	}}
}

func extent(w, h, d uint32) wgpu.Extent3D {
	return wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: d}
}

// TestTextureCopyRangeValidation: the region of a texture copy must lie
// inside its texture (Rust wgpu-core validate_texture_copy_range). The three
// copy commands report the same TransferError kinds.
func TestTextureCopyRangeValidation(t *testing.T) {
	rig := newCopyRig(t)
	layout := wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 8}
	origin := func(x, y, z uint32) wgpu.Origin3D { return wgpu.Origin3D{X: x, Y: y, Z: z} }

	tests := []struct {
		name string
		mip  uint32
		at   wgpu.Origin3D
		size wgpu.Extent3D
		want core.TransferErrorKind
	}{
		{name: "whole mip 0", size: extent(8, 8, 4), want: -1},
		{name: "inner region", at: origin(2, 2, 1), size: extent(4, 4, 2), want: -1},
		{name: "whole mip 1", mip: 1, size: extent(4, 4, 4), want: -1},
		{name: "width overruns", at: origin(5, 0, 0), size: extent(4, 1, 1), want: core.TransferErrorTextureOverrun},
		{name: "height overruns", at: origin(0, 6, 0), size: extent(1, 3, 1), want: core.TransferErrorTextureOverrun},
		{name: "layers overrun", at: origin(0, 0, 3), size: extent(1, 1, 2), want: core.TransferErrorTextureOverrun},
		{name: "mip 1 is half the size", mip: 1, size: extent(5, 1, 1), want: core.TransferErrorTextureOverrun},
	}
	for _, tc := range tests {
		t.Run("texture to buffer, "+tc.name, func(t *testing.T) {
			got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
				enc.CopyTextureToBuffer(rig.color, rig.buf, btCopy(tc.mip, tc.at, layout, tc.size))
			})
			if got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
		t.Run("buffer to texture, "+tc.name, func(t *testing.T) {
			got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
				enc.CopyBufferToTexture(rig.buf, rig.color, btCopy(tc.mip, tc.at, layout, tc.size))
			})
			if got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
		t.Run("texture to texture, source, "+tc.name, func(t *testing.T) {
			got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
				enc.CopyTextureToTexture(rig.color, rig.srgb, []wgpu.TextureCopy{{
					Source:      wgpu.ImageCopyTexture{Texture: rig.color, MipLevel: tc.mip, Origin: tc.at},
					Destination: wgpu.ImageCopyTexture{Texture: rig.srgb, MipLevel: tc.mip},
					Size:        tc.size,
				}})
			})
			if got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
		t.Run("texture to texture, destination, "+tc.name, func(t *testing.T) {
			got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
				enc.CopyTextureToTexture(rig.srgb, rig.color, []wgpu.TextureCopy{{
					Source:      wgpu.ImageCopyTexture{Texture: rig.srgb, MipLevel: tc.mip},
					Destination: wgpu.ImageCopyTexture{Texture: rig.color, MipLevel: tc.mip, Origin: tc.at},
					Size:        tc.size,
				}})
			})
			if got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("depth copied whole", func(t *testing.T) {
		got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(rig.depth, rig.buf, btCopy(0, origin(0, 0, 0), wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 8}, extent(8, 8, 1)))
		})
		if got != -1 {
			t.Fatalf("error kind = %d, want none", got)
		}
	})
	t.Run("depth partial copy", func(t *testing.T) {
		got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(rig.depth, rig.buf, btCopy(0, origin(0, 0, 0), wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 8}, extent(4, 8, 1)))
		})
		if got != core.TransferErrorUnsupportedPartialTransfer {
			t.Fatalf("error kind = %d, want %d", got, core.TransferErrorUnsupportedPartialTransfer)
		}
	})
}

// TestTextureToTextureFormatCompatibility: the formats of a texture-to-texture
// copy must be equal except for the sRGB suffix (Rust wgpu-core
// copy_texture_to_texture, W3C "copy-compatible").
func TestTextureToTextureFormatCompatibility(t *testing.T) {
	rig := newCopyRig(t)
	copyOne := func(src, dst *wgpu.Texture) func(enc *wgpu.CommandEncoder) {
		return func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(src, dst, []wgpu.TextureCopy{{
				Source:      wgpu.ImageCopyTexture{Texture: src},
				Destination: wgpu.ImageCopyTexture{Texture: dst},
				Size:        extent(8, 8, 1),
			}})
		}
	}
	tests := []struct {
		name     string
		src, dst *wgpu.Texture
		want     core.TransferErrorKind
	}{
		{"same format", rig.color, rig.color2, -1},
		{"srgb destination", rig.color, rig.srgb, -1},
		{"srgb source", rig.srgb, rig.color, -1},
		{"different channel order", rig.color, rig.bgra, core.TransferErrorTextureFormatsNotCopyCompatible},
		{"different format size", rig.color, rig.depth, core.TransferErrorTextureFormatsNotCopyCompatible},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rig.finishKind(t, copyOne(tc.src, tc.dst)); got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
	}
}
