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

const writeRegionSize = 8

// writeRegionCase is one WriteTexture of a 3x2 rectangle at (3, 4) into an
// 8x8 texture of layers layers: images (depth slices or array layers) from
// z, rows bytesPerRow apart and images rowsPerImage rows apart.
type writeRegionCase struct {
	name         string
	dimension    gputypes.TextureDimension
	layers       uint32
	bytesPerRow  uint32
	rowsPerImage uint32
	z, depth     uint32
}

// TestWriteTextureRegion fills a texture with one color, then writes a 3x2
// rectangle at origin (3, 4) from data that starts at a layout offset, whose
// rows are padded and whose length is the WebGPU minimum: it ends at the last
// byte of the last row of the last image. Only the rectangle may change, and
// it must hold the rows the layout describes.
//
// Regression test for two defects. The GLES 2D upload wrote at (0, 0), read
// rows as tightly packed and ignored the offset. The staging copy of the
// batching backends dropped a last row shorter than bytesPerRow, and when the
// rows needed no padding it ignored the offset and staged the whole data.
func TestWriteTextureRegion(t *testing.T) {
	d2, d3 := gputypes.TextureDimension2D, gputypes.TextureDimension3D
	cases := []writeRegionCase{
		{name: "2d/bytesPerRow20", dimension: d2, layers: 1, bytesPerRow: 20, rowsPerImage: 2, depth: 1},
		// 18 is not a multiple of the 4-byte texel.
		{name: "2d/bytesPerRow18", dimension: d2, layers: 1, bytesPerRow: 18, rowsPerImage: 2, depth: 1},
		// 256 needs no padding in staging.
		{name: "2d/bytesPerRow256", dimension: d2, layers: 1, bytesPerRow: 256, rowsPerImage: 2, depth: 1},
		{name: "array/bytesPerRow20", dimension: d2, layers: 4, bytesPerRow: 20, rowsPerImage: 3, z: 1, depth: 2},
		{name: "array/bytesPerRow256", dimension: d2, layers: 4, bytesPerRow: 256, rowsPerImage: 3, z: 1, depth: 2},
		{name: "3d/bytesPerRow20", dimension: d3, layers: 4, bytesPerRow: 20, rowsPerImage: 3, z: 1, depth: 3},
		{name: "3d/bytesPerRow256", dimension: d3, layers: 4, bytesPerRow: 256, rowsPerImage: 3, z: 1, depth: 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
				checkWriteTextureRegion(t, device, c)
			})
		})
	}
}

func checkWriteTextureRegion(t *testing.T, device *wgpu.Device, c writeRegionCase) {
	t.Helper()
	const (
		ox, oy = 3, 4
		w, h   = 3, 2
		offset = 4
	)
	background := [4]byte{9, 9, 9, 255}
	regionTexel := func(x, y, z uint32) [4]byte { return [4]byte{byte(100 + x), byte(200 + y), byte(10 + z), 255} }
	extent := wgpu.Extent3D{Width: writeRegionSize, Height: writeRegionSize, DepthOrArrayLayers: c.layers}

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "write-region",
		Size:          extent,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     c.dimension,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	full := make([]byte, writeRegionSize*writeRegionSize*4*c.layers)
	for i := 0; i < len(full); i += 4 {
		copy(full[i:], background[:])
	}
	if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, full,
		&wgpu.ImageDataLayout{BytesPerRow: writeRegionSize * 4, RowsPerImage: writeRegionSize}, &extent); err != nil {
		t.Fatalf("WriteTexture (background): %v", err)
	}

	// The padding and the bytes before the offset hold a value no texel has.
	imageBytes := c.bytesPerRow * c.rowsPerImage
	region := make([]byte, offset+imageBytes*(c.depth-1)+c.bytesPerRow*(h-1)+w*4)
	for i := range region {
		region[i] = 0xEE
	}
	for z := uint32(0); z < c.depth; z++ {
		for y := uint32(0); y < h; y++ {
			for x := uint32(0); x < w; x++ {
				texel := regionTexel(x, y, z)
				copy(region[offset+z*imageBytes+y*c.bytesPerRow+x*4:], texel[:])
			}
		}
	}
	if err := device.Queue().WriteTexture(
		&wgpu.ImageCopyTexture{Texture: tex, Origin: wgpu.Origin3D{X: ox, Y: oy, Z: c.z}}, region,
		&wgpu.ImageDataLayout{Offset: offset, BytesPerRow: c.bytesPerRow, RowsPerImage: c.rowsPerImage},
		&wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: c.depth}); err != nil {
		t.Fatalf("WriteTexture (region): %v", err)
	}

	const pitch = 256
	staging := newPublicBuffer(t, device, uint64(pitch*writeRegionSize*c.layers), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc := newEncoder(t, device)
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: pitch, RowsPerImage: writeRegionSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         extent,
	}})
	submit(t, device, enc)
	words := readPublicBuffer(t, staging, int(pitch*writeRegionSize*c.layers/4))
	inRegion := func(x, y, z uint32) bool {
		return x >= ox && x < ox+w && y >= oy && y < oy+h && z >= c.z && z < c.z+c.depth
	}
	for z := uint32(0); z < c.layers; z++ {
		for y := uint32(0); y < writeRegionSize; y++ {
			for x := uint32(0); x < writeRegionSize; x++ {
				want := background
				if inRegion(x, y, z) {
					want = regionTexel(x-ox, y-oy, z-c.z)
				}
				var got [4]byte
				binary.LittleEndian.PutUint32(got[:], words[(z*writeRegionSize+y)*pitch/4+x])
				if got != want {
					t.Errorf("texel (%d, %d, %d) = %v, want %v", x, y, z, got, want)
				}
			}
		}
	}
}
