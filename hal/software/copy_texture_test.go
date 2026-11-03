//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// copyTexel is the 4 bytes of texel (x, y) of layer z of a pattern texture.
func copyTexel(x, y, z uint32) [4]byte {
	return [4]byte{byte(x), byte(y), byte(z), 0xA0}
}

// copySentinel is the 4 bytes of a texel no copy may change.
var copySentinel = [4]byte{0xEE, 0xEE, 0xEE, 0xEE}

// newCopyTexture creates a w x h x layers RGBA8 texture filled with the
// copyTexel pattern, or with copySentinel if sentinel is set.
func newCopyTexture(t *testing.T, dev *Device, w, h, layers uint32, sentinel bool) *Texture {
	t.Helper()
	ht, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: layers},
		Format:        gputypes.TextureFormatRGBA8Unorm,
		MipLevelCount: 2,
		Usage:         gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	tex := ht.(*Texture)
	data := make([]byte, 0, w*h*layers*4)
	for z := range layers {
		for y := range h {
			for x := range w {
				px := copyTexel(x, y, z)
				if sentinel {
					px = copySentinel
				}
				data = append(data, px[:]...)
			}
		}
	}
	tex.WriteData(0, data)
	return tex
}

// copyTextures records one CopyTextureToTexture of region.
func copyTextures(t *testing.T, dev *Device, src, dst hal.Texture, region hal.TextureCopy) {
	t.Helper()
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	region.SrcBase.Texture, region.DstBase.Texture = src, dst
	enc.CopyTextureToTexture(src, dst, []hal.TextureCopy{region})
}

// checkCopiedTexel compares texel (x, y) of layer z of data, a w x h image
// per layer, with want.
func checkCopiedTexel(t *testing.T, data []byte, w, h, x, y, z uint32, want [4]byte) {
	t.Helper()
	i := ((z*h+y)*w + x) * 4
	if got := [4]byte(data[i : i+4]); got != want {
		t.Errorf("texel (%d, %d) layer %d = %v, want %v", x, y, z, got, want)
	}
}

// TestCopyTextureToTextureSubRect copies a 3x2 sub-rectangle from origin
// (2, 1) of an 8-wide texture to origin (1, 2) of a 6-wide one: rows at
// each texture's own pitch, both origins honored, every other texel kept.
func TestCopyTextureToTextureSubRect(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 8, 4, 1, false)
	dst := newCopyTexture(t, dev, 6, 5, 1, true)

	copyTextures(t, dev, src, dst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{Origin: hal.Origin3D{X: 2, Y: 1}},
		DstBase: hal.ImageCopyTexture{Origin: hal.Origin3D{X: 1, Y: 2}},
		Size:    hal.Extent3D{Width: 3, Height: 2, DepthOrArrayLayers: 1},
	})

	data := dst.GetData()
	for y := range uint32(5) {
		for x := range uint32(6) {
			want := copySentinel
			if x >= 1 && x < 4 && y >= 2 && y < 4 {
				want = copyTexel(x+1, y-1, 0) // dst (x, y) is src (x-1+2, y-2+1)
			}
			checkCopiedTexel(t, data, 6, 5, x, y, 0, want)
		}
	}
}

// TestCopyTextureToTextureArrayLayer copies layer 1 of a 2-layer texture to
// layer 0 of another and layer 0 to layer 1: Origin.Z selects the layer.
func TestCopyTextureToTextureArrayLayer(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 4, 3, 2, false)
	dst := newCopyTexture(t, dev, 4, 3, 2, true)

	copyTextures(t, dev, src, dst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 0}},
		DstBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 1}},
		Size:    hal.Extent3D{Width: 4, Height: 3, DepthOrArrayLayers: 1},
	})
	copyTextures(t, dev, src, dst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{Origin: hal.Origin3D{X: 1, Y: 1, Z: 1}},
		DstBase: hal.ImageCopyTexture{Origin: hal.Origin3D{X: 0, Y: 0, Z: 0}},
		Size:    hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})

	data := dst.GetData()
	for y := range uint32(3) {
		for x := range uint32(4) {
			checkCopiedTexel(t, data, 4, 3, x, y, 1, copyTexel(x, y, 0))
			want := copySentinel
			if x < 2 && y < 2 {
				want = copyTexel(x+1, y+1, 1)
			}
			checkCopiedTexel(t, data, 4, 3, x, y, 0, want)
		}
	}
}

// TestCopyTextureToTextureOtherMipCopiesNothing copies into mip level 1.
// The software backend stores mip level 0 only, so the copy must leave the
// texture alone; the old copy wrote the start of mip 0 instead.
func TestCopyTextureToTextureOtherMipCopiesNothing(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 4, 4, 1, false)
	dst := newCopyTexture(t, dev, 4, 4, 1, true)

	copyTextures(t, dev, src, dst, hal.TextureCopy{
		DstBase: hal.ImageCopyTexture{MipLevel: 1},
		Size:    hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})

	data := dst.GetData()
	for y := range uint32(4) {
		for x := range uint32(4) {
			checkCopiedTexel(t, data, 4, 4, x, y, 0, copySentinel)
		}
	}
}

// TestCopyTextureToSurfaceTexture copies a BGRA8 texture into the acquired
// texture of a software surface, at an origin, and checks the framebuffer.
// The surface texture is a *SurfaceTexture, which the copy used to drop.
func TestCopyTextureToSurfaceTexture(t *testing.T) {
	const w, h = 6, 4
	surf, dev, _ := createDamageTestSurfaceBGRA(t, w, h)
	acquired, err := surf.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	srcHal, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 3, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatBGRA8Unorm,
		Usage:  gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	src := srcHal.(*Texture)
	pattern := make([]byte, 0, 3*2*4)
	for y := range uint32(2) {
		for x := range uint32(3) {
			px := copyTexel(x, y, 0)
			pattern = append(pattern, px[:]...)
		}
	}
	src.WriteData(0, pattern)

	copyTextures(t, dev, src, acquired.Texture, hal.TextureCopy{
		DstBase: hal.ImageCopyTexture{Origin: hal.Origin3D{X: 2, Y: 1}},
		Size:    hal.Extent3D{Width: 3, Height: 2, DepthOrArrayLayers: 1},
	})

	surf.mu.RLock()
	fb := append([]byte(nil), surf.framebuffer...)
	surf.mu.RUnlock()
	for y := range uint32(h) {
		for x := range uint32(w) {
			i := (y*w + x) * 4
			got := [4]byte(fb[i : i+4])
			inside := x >= 2 && x < 5 && y >= 1 && y < 3
			if inside && got != copyTexel(x-2, y-1, 0) {
				t.Errorf("framebuffer texel (%d, %d) = %v, want %v", x, y, got, copyTexel(x-2, y-1, 0))
			}
			if !inside && got == copyTexel(x, y, 0) {
				t.Errorf("framebuffer texel (%d, %d) outside the region was written", x, y)
			}
		}
	}
}

// TestCopyTextureToBufferLayerAndOrigin copies a 2x2 region from origin
// (1, 1) of layer 1 into a buffer with a 256-byte row pitch.
func TestCopyTextureToBufferLayerAndOrigin(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 4, 3, 2, false)
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 512, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	buf := bufHal.(*Buffer)

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(src, buf, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 2},
		TextureBase:  hal.ImageCopyTexture{Texture: src, Origin: hal.Origin3D{X: 1, Y: 1, Z: 1}},
		Size:         hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	}})

	data := buf.GetData()
	for row := range uint32(2) {
		for x := range uint32(2) {
			i := row*256 + x*4
			if got, want := [4]byte(data[i:i+4]), copyTexel(x+1, row+1, 1); got != want {
				t.Errorf("buffer row %d texel %d = %v, want %v", row, x, got, want)
			}
		}
	}
}

// TestCopyBufferToTextureOtherMipCopiesNothing uploads into mip level 1,
// which has no storage: the texture must stay unchanged (the old copy
// wrote the data into mip level 0).
func TestCopyBufferToTextureOtherMipCopiesNothing(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	dst := newCopyTexture(t, dev, 4, 4, 1, true)
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 256, Usage: gputypes.BufferUsageCopySrc})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	bufHal.(*Buffer).WriteData(0, make([]byte, 256))

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToTexture(bufHal, dst, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 256},
		TextureBase:  hal.ImageCopyTexture{Texture: dst, MipLevel: 1},
		Size:         hal.Extent3D{Width: 2, Height: 1, DepthOrArrayLayers: 1},
	}})

	data := dst.GetData()
	for x := range uint32(4) {
		checkCopiedTexel(t, data, 4, 4, x, 0, 0, copySentinel)
	}
}

// TestWriteTextureOtherMipWritesNothing writes into mip level 1, which has
// no storage: the texture must stay unchanged (the old write landed in mip
// level 0).
func TestWriteTextureOtherMipWritesNothing(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	dst := newCopyTexture(t, dev, 4, 4, 1, true)

	err := q.WriteTexture(&hal.ImageCopyTexture{Texture: dst, MipLevel: 1}, make([]byte, 2*2*4),
		&hal.ImageDataLayout{BytesPerRow: 8, RowsPerImage: 2}, &hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1})
	if err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	data := dst.GetData()
	for y := range uint32(4) {
		for x := range uint32(4) {
			checkCopiedTexel(t, data, 4, 4, x, y, 0, copySentinel)
		}
	}
}

// TestCopyFromOtherMipWritesNothing copies from mip level 1 of a texture,
// into a texture and into a buffer: there is no level 1 data to read, and
// copying level 0 in its place would be the wrong data, so neither
// destination may change.
func TestCopyFromOtherMipWritesNothing(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 4, 4, 1, false)
	dst := newCopyTexture(t, dev, 4, 4, 1, true)
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	buf := bufHal.(*Buffer)
	marker := make([]byte, 64)
	for i := range marker {
		marker[i] = 0xEE
	}
	buf.WriteData(0, marker)

	copyTextures(t, dev, src, dst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{MipLevel: 1},
		Size:    hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(src, buf, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 8, RowsPerImage: 2},
		TextureBase:  hal.ImageCopyTexture{Texture: src, MipLevel: 1},
		Size:         hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	}})

	data := dst.GetData()
	for y := range uint32(4) {
		for x := range uint32(4) {
			checkCopiedTexel(t, data, 4, 4, x, y, 0, copySentinel)
		}
	}
	for i, b := range buf.GetData() {
		if b != 0xEE {
			t.Fatalf("buffer byte %d = %#x, want 0xee: the copy read mip level 0", i, b)
		}
	}
}
