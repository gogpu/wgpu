// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestLayeredTextureCopyRoundTrip fills a whole mip level of a 3D texture and
// of a 2D array texture, with WriteTexture and with CopyBufferToTexture, and
// reads it back whole and one layer at a time from a layer above 0. Every
// texel encodes its x, y and layer, so a copy that lands on the wrong level,
// the wrong layer or with the wrong layer stride is seen. GLES allocated no
// storage for 3D and 2D array textures and copied only layer 0 of level 0.
func TestLayeredTextureCopyRoundTrip(t *testing.T) {
	dims := []struct {
		name string
		dim  gputypes.TextureDimension
	}{
		{"3d", gputypes.TextureDimension3D},
		{"array", gputypes.TextureDimension2D},
	}
	uploads := []string{"WriteTexture", "CopyBufferToTexture"}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, d := range dims {
			for mip := uint32(0); mip < 2; mip++ {
				for _, upload := range uploads {
					t.Run(fmt.Sprintf("%s/mip%d/%s", d.name, mip, upload), func(t *testing.T) {
						layeredRoundTrip(t, device, d.dim, mip, upload)
					})
				}
			}
		}
	})
}

const (
	layeredBaseSize   = 8
	layeredBaseLayers = 4
	layeredRowStride  = 256
)

// layeredTexel is the RGBA8 value stored at (x, y, z) of mip level mip.
func layeredTexel(x, y, z, mip uint32) []byte {
	return []byte{byte(x*16 + y), byte(z + 1), byte(10 + mip), 255}
}

// layeredLevel returns the width, height and depth (3D) or layer count
// (array) of mip level mip.
func layeredLevel(dim gputypes.TextureDimension, mip uint32) (w, h, d uint32) {
	w, h, d = layeredBaseSize>>mip, layeredBaseSize>>mip, layeredBaseLayers
	if dim == gputypes.TextureDimension3D {
		d = layeredBaseLayers >> mip
	}
	return w, h, d
}

// layeredImage lays out a level as rows layeredRowStride bytes apart and
// layers h rows apart.
func layeredImage(w, h, d, mip uint32) []byte {
	img := make([]byte, layeredRowStride*h*d)
	for z := uint32(0); z < d; z++ {
		for y := uint32(0); y < h; y++ {
			for x := uint32(0); x < w; x++ {
				copy(img[(z*h+y)*layeredRowStride+x*4:], layeredTexel(x, y, z, mip))
			}
		}
	}
	return img
}

func layeredRoundTrip(t *testing.T, device *wgpu.Device, dim gputypes.TextureDimension, mip uint32, upload string) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "layered-roundtrip",
		Size:          wgpu.Extent3D{Width: layeredBaseSize, Height: layeredBaseSize, DepthOrArrayLayers: layeredBaseLayers},
		MipLevelCount: 2,
		SampleCount:   1,
		Dimension:     dim,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	w, h, d := layeredLevel(dim, mip)
	img := layeredImage(w, h, d, mip)
	size := wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: d}
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

	// The whole level.
	got := readLayered(t, device, tex, mip, 0, size)
	compareLayered(t, got, w, h, d, 0, mip)

	// One layer above 0, read on its own: its origin Z selects it.
	k := d - 1
	got = readLayered(t, device, tex, mip, k, wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1})
	compareLayered(t, got, w, h, 1, k, mip)
}

// readLayered reads layers [z, z+size.DepthOrArrayLayers) of mip level mip,
// rows layeredRowStride bytes apart and layers size.Height rows apart.
func readLayered(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, mip, z uint32, size wgpu.Extent3D) []byte {
	t.Helper()
	n := uint64(layeredRowStride) * uint64(size.Height) * uint64(size.DepthOrArrayLayers)
	staging := newPublicBuffer(t, device, n, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: layeredRowStride, RowsPerImage: size.Height},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex, MipLevel: mip, Origin: wgpu.Origin3D{Z: z}},
			Size:         size,
		}})
	})
	out := make([]byte, 0, n)
	for _, word := range readPublicBuffer(t, staging, int(n/4)) {
		out = binary.LittleEndian.AppendUint32(out, word)
	}
	return out
}

// compareLayered checks d layers of got, the first of which is layer z0.
func compareLayered(t *testing.T, got []byte, w, h, d, z0, mip uint32) {
	t.Helper()
	for z := uint32(0); z < d; z++ {
		for y := uint32(0); y < h; y++ {
			for x := uint32(0); x < w; x++ {
				off := (z*h+y)*layeredRowStride + x*4
				want := layeredTexel(x, y, z0+z, mip)
				if texel := got[off : off+4]; !bytes.Equal(texel, want) {
					t.Fatalf("mip %d layer %d texel (%d, %d) = %v, want %v", mip, z0+z, x, y, texel, want)
				}
			}
		}
	}
}
