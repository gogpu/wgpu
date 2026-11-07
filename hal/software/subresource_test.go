//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// These tests address mip levels, array layers and 3D depth slices through
// HAL operations only (WriteTexture, copies, render passes, shaders), so they
// hold whatever byte layout the backend stores a texture in (P1-SW-026).

// subTexel is the RGBA8 texel (x, y) of depth slice or array layer z of mip
// level level of a pattern texture: unique per subresource and position.
func subTexel(level, z, x, y uint32) [4]byte {
	return [4]byte{byte(x + 16*level), byte(y + 16*z), byte(0x40 + level), byte(0x80 + z)}
}

// subSentinel is a texel no operation of a test may change.
var subSentinel = [4]byte{0xEE, 0xEE, 0xEE, 0xEE}

// subExtent is the size of mip level level of a texture whose level 0 is
// w x h x d: each dimension halved per level, at least 1. d shrinks only
// for a 3D texture; for a 2D one it is the array layer count.
func subExtent(dim gputypes.TextureDimension, w, h, d, level uint32) (lw, lh, ld uint32) {
	lw, lh, ld = max(w>>level, 1), max(h>>level, 1), d
	if dim == gputypes.TextureDimension3D {
		ld = max(d>>level, 1)
	}
	return lw, lh, ld
}

func newSubTexture(t *testing.T, dev *Device, dim gputypes.TextureDimension, w, h, d, mips uint32,
	format gputypes.TextureFormat) hal.Texture {
	t.Helper()
	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: d},
		MipLevelCount: mips,
		SampleCount:   1,
		Dimension:     dim,
		Format:        format,
		Usage: gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding |
			gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageStorageBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	return tex
}

func newSubView(t *testing.T, dev *Device, tex hal.Texture, desc hal.TextureViewDescriptor) hal.TextureView {
	t.Helper()
	view, err := dev.CreateTextureView(tex, &desc)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	return view
}

// fillLevel writes n consecutive depth slices or layers, from z0, of mip
// level level with texel(x, y, z) in one WriteTexture.
func fillLevel(t *testing.T, q hal.Queue, tex hal.Texture, level, z0, w, h, n uint32, texel func(x, y, z uint32) [4]byte) {
	t.Helper()
	data := make([]byte, 0, w*h*n*4)
	for z := range n {
		for y := range h {
			for x := range w {
				px := texel(x, y, z0+z)
				data = append(data, px[:]...)
			}
		}
	}
	err := q.WriteTexture(&hal.ImageCopyTexture{Texture: tex, MipLevel: level, Origin: hal.Origin3D{Z: z0}}, data,
		&hal.ImageDataLayout{BytesPerRow: w * 4, RowsPerImage: h}, &hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: n})
	if err != nil {
		t.Fatalf("WriteTexture level %d: %v", level, err)
	}
}

// readImage reads back depth slice or layer z of mip level level, w x h
// texels of bpp bytes, through CopyTextureToBuffer.
func readImage(t *testing.T, dev *Device, tex hal.Texture, level, z, w, h, bpp uint32) []byte {
	t.Helper()
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(w * h * bpp), Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(tex, bufHal, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: w * bpp, RowsPerImage: h},
		TextureBase:  hal.ImageCopyTexture{Texture: tex, MipLevel: level, Origin: hal.Origin3D{Z: z}},
		Size:         hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	}})
	submit(t, enc)
	return bufHal.(*Buffer).GetData()
}

// checkImage compares a w x h RGBA8 image read back from level level, slice
// z with want(x, y).
func checkImage(t *testing.T, data []byte, level, z, w, h uint32, want func(x, y uint32) [4]byte) {
	t.Helper()
	bad := 0
	for y := range h {
		for x := range w {
			i := (y*w + x) * 4
			if got, exp := [4]byte(data[i:i+4]), want(x, y); got != exp {
				if bad < 4 {
					t.Errorf("level %d slice %d texel (%d, %d) = %v, want %v", level, z, x, y, got, exp)
				}
				bad++
			}
		}
	}
	if bad > 4 {
		t.Errorf("level %d slice %d: %d texels differ in all", level, z, bad)
	}
}

// TestWriteTextureSubresourcesRoundTrip writes every mip level of a 2D
// texture, a 2D array and a 3D texture, all layers or depth slices of a
// level in one WriteTexture, and reads every slice back: each level and
// layer must hold its own data. Non-power-of-two sizes check the per-level
// pitch, and the 3D texture's depth shrinks per level (4, 2, 1). The old
// backend stored mip level 0 only and skipped the other levels.
func TestWriteTextureSubresourcesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dim     gputypes.TextureDimension
		w, h, d uint32
	}{
		{"2d", gputypes.TextureDimension2D, 7, 5, 1},
		{"2d-array", gputypes.TextureDimension2D, 5, 3, 3},
		{"3d", gputypes.TextureDimension3D, 6, 4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev, q, cleanup := createSoftwareDevice(t)
			defer cleanup()
			const mips = 3
			tex := newSubTexture(t, dev, tc.dim, tc.w, tc.h, tc.d, mips, gputypes.TextureFormatRGBA8Unorm)
			for level := range uint32(mips) {
				w, h, n := subExtent(tc.dim, tc.w, tc.h, tc.d, level)
				fillLevel(t, q, tex, level, 0, w, h, n, func(x, y, z uint32) [4]byte { return subTexel(level, z, x, y) })
			}
			for level := range uint32(mips) {
				w, h, n := subExtent(tc.dim, tc.w, tc.h, tc.d, level)
				for z := range n {
					checkImage(t, readImage(t, dev, tex, level, z, w, h, 4), level, z, w, h,
						func(x, y uint32) [4]byte { return subTexel(level, z, x, y) })
				}
			}
		})
	}
}

// TestCopyTextureToTextureBetweenLevels copies a region of mip level 1 of
// one texture to mip level 0 of another, and level 0 to level 1 of a
// layer, at origins: each copy reads and writes its own level at that
// level's pitch, and every other texel keeps its sentinel. Also uploads the
// sentinels with CopyBufferToTexture at each level.
func TestCopyTextureToTextureBetweenLevels(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newSubTexture(t, dev, gputypes.TextureDimension2D, 9, 6, 2, 2, gputypes.TextureFormatRGBA8Unorm)
	dst := newSubTexture(t, dev, gputypes.TextureDimension2D, 9, 6, 2, 2, gputypes.TextureFormatRGBA8Unorm)
	for level := range uint32(2) {
		w, h, n := subExtent(gputypes.TextureDimension2D, 9, 6, 2, level)
		fillLevel(t, q, src, level, 0, w, h, n, func(x, y, z uint32) [4]byte { return subTexel(level, z, x, y) })
		uploadSentinel(t, dev, dst, level, w, h, n)
	}

	copyTextures(t, dev, src, dst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{MipLevel: 1, Origin: hal.Origin3D{X: 1, Y: 1, Z: 1}},
		DstBase: hal.ImageCopyTexture{MipLevel: 0, Origin: hal.Origin3D{X: 5, Y: 2, Z: 0}},
		Size:    hal.Extent3D{Width: 3, Height: 2, DepthOrArrayLayers: 1},
	})
	copyTextures(t, dev, src, dst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{MipLevel: 0, Origin: hal.Origin3D{X: 4, Y: 3, Z: 0}},
		DstBase: hal.ImageCopyTexture{MipLevel: 1, Origin: hal.Origin3D{X: 2, Y: 1, Z: 1}},
		Size:    hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})

	checkImage(t, readImage(t, dev, dst, 0, 0, 9, 6, 4), 0, 0, 9, 6, func(x, y uint32) [4]byte {
		if x >= 5 && x < 8 && y >= 2 && y < 4 {
			return subTexel(1, 1, x-5+1, y-2+1)
		}
		return subSentinel
	})
	checkImage(t, readImage(t, dev, dst, 1, 1, 4, 3, 4), 1, 1, 4, 3, func(x, y uint32) [4]byte {
		if x >= 2 && y >= 1 {
			return subTexel(0, 0, x-2+4, y-1+3)
		}
		return subSentinel
	})
	for _, s := range [][2]uint32{{0, 1}, {1, 0}} {
		w, h, _ := subExtent(gputypes.TextureDimension2D, 9, 6, 2, s[0])
		checkImage(t, readImage(t, dev, dst, s[0], s[1], w, h, 4), s[0], s[1], w, h,
			func(_, _ uint32) [4]byte { return subSentinel })
	}
}

// uploadSentinel fills n layers of mip level level with subSentinel through
// CopyBufferToTexture, rows 256 bytes apart.
func uploadSentinel(t *testing.T, dev *Device, tex hal.Texture, level, w, h, n uint32) {
	t.Helper()
	const pitch = 256
	data := make([]byte, pitch*h*n)
	for i := range data {
		data[i] = 0xEE
	}
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(data)), Usage: gputypes.BufferUsageCopySrc})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	bufHal.(*Buffer).WriteData(0, data)
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToTexture(bufHal, tex, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: pitch, RowsPerImage: h},
		TextureBase:  hal.ImageCopyTexture{Texture: tex, MipLevel: level},
		Size:         hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: n},
	}})
	submit(t, enc)
}
