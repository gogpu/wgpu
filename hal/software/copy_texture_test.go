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

// copyTextures records one CopyTextureToTexture of region and submits it.
func copyTextures(t *testing.T, dev *Device, src, dst hal.Texture, region hal.TextureCopy) {
	t.Helper()
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	region.SrcBase.Texture, region.DstBase.Texture = src, dst
	enc.CopyTextureToTexture(src, dst, []hal.TextureCopy{region})
	submit(t, enc)
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

// checkLevel0Sentinel checks that every texel of mip level 0 of a w x h
// newCopyTexture made with sentinel set is still copySentinel.
func checkLevel0Sentinel(t *testing.T, dev *Device, tex *Texture, w, h uint32) {
	t.Helper()
	data := readImage(t, dev, tex, 0, 0, w, h, 4)
	for y := range h {
		for x := range w {
			checkCopiedTexel(t, data, w, h, x, y, 0, copySentinel)
		}
	}
}

// TestCopyTextureToTextureOtherMipKeepsLevel0 copies into mip level 1: the
// texels must land in level 1 and level 0 must stay unchanged. Before mip
// chains were stored the copy wrote the start of level 0, and then it was
// skipped.
func TestCopyTextureToTextureOtherMipKeepsLevel0(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 4, 4, 1, false)
	dst := newCopyTexture(t, dev, 4, 4, 1, true)

	copyTextures(t, dev, src, dst, hal.TextureCopy{
		DstBase: hal.ImageCopyTexture{MipLevel: 1},
		Size:    hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})

	checkLevel0Sentinel(t, dev, dst, 4, 4)
	level1 := readImage(t, dev, dst, 1, 0, 2, 2, 4)
	for y := range uint32(2) {
		for x := range uint32(2) {
			checkCopiedTexel(t, level1, 2, 2, x, y, 0, copyTexel(x, y, 0))
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
	submit(t, enc)

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

// copyFill is the byte of the data the other-mip transfer tests write.
const copyFill = 0x5A

// TestCopyBufferToTextureOtherMipKeepsLevel0 uploads one row into mip level
// 1: it must land in level 1 row 0 and leave level 0 unchanged (the copy
// once wrote the data into level 0).
func TestCopyBufferToTextureOtherMipKeepsLevel0(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	dst := newCopyTexture(t, dev, 4, 4, 1, true)
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 256, Usage: gputypes.BufferUsageCopySrc})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	fill := make([]byte, 256)
	for i := range fill {
		fill[i] = copyFill
	}
	bufHal.(*Buffer).WriteData(0, fill)

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToTexture(bufHal, dst, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 256},
		TextureBase:  hal.ImageCopyTexture{Texture: dst, MipLevel: 1},
		Size:         hal.Extent3D{Width: 2, Height: 1, DepthOrArrayLayers: 1},
	}})
	submit(t, enc)

	checkLevel0Sentinel(t, dev, dst, 4, 4)
	level1 := readImage(t, dev, dst, 1, 0, 2, 2, 4)
	for y := range uint32(2) {
		for x := range uint32(2) {
			want := [4]byte{} // row 1 was never written
			if y == 0 {
				want = [4]byte{copyFill, copyFill, copyFill, copyFill}
			}
			checkCopiedTexel(t, level1, 2, 2, x, y, 0, want)
		}
	}
}

// TestWriteTextureOtherMipKeepsLevel0 writes all of mip level 1: it must
// land there and leave level 0 unchanged (the write once landed in level 0).
func TestWriteTextureOtherMipKeepsLevel0(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	dst := newCopyTexture(t, dev, 4, 4, 1, true)

	fill := make([]byte, 2*2*4)
	for i := range fill {
		fill[i] = copyFill
	}
	err := q.WriteTexture(&hal.ImageCopyTexture{Texture: dst, MipLevel: 1}, fill,
		&hal.ImageDataLayout{BytesPerRow: 8, RowsPerImage: 2}, &hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1})
	if err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	checkLevel0Sentinel(t, dev, dst, 4, 4)
	level1 := readImage(t, dev, dst, 1, 0, 2, 2, 4)
	for y := range uint32(2) {
		for x := range uint32(2) {
			checkCopiedTexel(t, level1, 2, 2, x, y, 0, [4]byte{copyFill, copyFill, copyFill, copyFill})
		}
	}
}

// TestCopyFromOtherMipReadsThatLevel copies from mip level 1 of a texture,
// into a texture and into a buffer: both must get level 1's texels, not
// level 0's (the copy once read level 0, and then it was skipped).
func TestCopyFromOtherMipReadsThatLevel(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	src := newCopyTexture(t, dev, 4, 4, 1, false)
	dst := newCopyTexture(t, dev, 4, 4, 1, true)
	level1 := make([]byte, 0, 2*2*4)
	for y := range uint32(2) {
		for x := range uint32(2) {
			px := copyTexel(x, y, 9)
			level1 = append(level1, px[:]...)
		}
	}
	err := q.WriteTexture(&hal.ImageCopyTexture{Texture: src, MipLevel: 1}, level1,
		&hal.ImageDataLayout{BytesPerRow: 8, RowsPerImage: 2}, &hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1})
	if err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	bufHal, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 16, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	buf := bufHal.(*Buffer)

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
	submit(t, enc)

	data := readImage(t, dev, dst, 0, 0, 4, 4, 4)
	for y := range uint32(4) {
		for x := range uint32(4) {
			want := copySentinel
			if x < 2 && y < 2 {
				want = copyTexel(x, y, 9)
			}
			checkCopiedTexel(t, data, 4, 4, x, y, 0, want)
		}
	}
	got := buf.GetData()
	for y := range uint32(2) {
		for x := range uint32(2) {
			checkCopiedTexel(t, got, 2, 2, x, y, 0, copyTexel(x, y, 9))
		}
	}
}
