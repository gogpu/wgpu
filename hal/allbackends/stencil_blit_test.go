// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// combinedStencilFormats are the depth/stencil formats whose stencil aspect
// GLES cannot write from a buffer: the frontend writes it with a draw
// (StencilCopyNeedsBlit). Every other backend copies it.
var combinedStencilFormats = []gputypes.TextureFormat{
	gputypes.TextureFormatDepth24PlusStencil8,
	gputypes.TextureFormatDepth32FloatStencil8,
}

// TestStencilUploadLayerRange writes a background stencil value into mip
// levels 0 and 1 of all four layers of a 2D array texture, then a pattern
// into layers 1 and 2 of mip level 1 with one upload, and reads every layer
// of both levels back: the upload must reach exactly its layers and its mip
// level, and leave the others as they were.
func TestStencilUploadLayerRange(t *testing.T) {
	const (
		layers     = 4
		background = 0x5A
		level0     = 0x3C
		texels     = depthReadbackSize * depthReadbackSize
	)
	forEachStencilUpload(t, combinedStencilFormats, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viaWriteTexture bool) {
		tex := newDepthArrayTexture(t, device, "stencil-layer-range", format,
			wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst, 2, layers)
		full0 := wgpu.Extent3D{Width: 2 * depthReadbackSize, Height: 2 * depthReadbackSize, DepthOrArrayLayers: layers}
		uploadStencil(t, device, tex, 0, wgpu.Origin3D{}, full0, bytes.Repeat([]byte{level0}, 4*texels*layers), viaWriteTexture)
		full1 := wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: layers}
		uploadStencil(t, device, tex, 1, wgpu.Origin3D{}, full1, bytes.Repeat([]byte{background}, texels*layers), viaWriteTexture)

		pattern := make([]byte, texels*2)
		for i := range pattern {
			pattern[i] = byte(0x80 + i)
		}
		part := wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: 2}
		uploadStencil(t, device, tex, 1, wgpu.Origin3D{Z: 1}, part, pattern, viaWriteTexture)

		for layer := uint32(0); layer < layers; layer++ {
			got := readStencilLevel(t, device, tex, 0, layer, 2*depthReadbackSize)
			for i, v := range got {
				if v != level0 {
					t.Fatalf("mip 0 layer %d texel %d = %#x, want %#x", layer, i, v, level0)
				}
			}
			got = readStencilLevel(t, device, tex, 1, layer, depthReadbackSize)
			for i, v := range got {
				want := byte(background)
				if layer == 1 || layer == 2 {
					want = pattern[int(layer-1)*texels+i]
				}
				if v != want {
					t.Fatalf("mip 1 layer %d texel %d = %#x, want %#x", layer, i, v, want)
				}
			}
		}
	})
}

// TestStencilUploadRegions writes layers 0 and 1 of a stencil aspect with two
// regions of one CopyBufferToTexture, the second from a later part of the
// buffer: each region must land in its own layer.
func TestStencilUploadRegions(t *testing.T) {
	const texels = depthReadbackSize * depthReadbackSize
	forEachStencilFormat(t, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat) {
		tex := newDepthArrayTexture(t, device, "stencil-regions", format,
			wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst, 2, 2)
		layerBytes := depthReadbackBytesPerRow * depthReadbackSize
		data := make([]byte, 2*layerBytes)
		for layer := 0; layer < 2; layer++ {
			for i := 0; i < texels; i++ {
				data[layer*layerBytes+(i/depthReadbackSize)*depthReadbackBytesPerRow+i%depthReadbackSize] = byte(0x20*(layer+1) + i)
			}
		}
		src := newPublicBuffer(t, device, uint64(len(data)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		if err := device.Queue().WriteBuffer(src, 0, data); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		size := wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: 1}
		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			regions := make([]wgpu.BufferTextureCopy, 0, 2)
			for layer := uint32(0); layer < 2; layer++ {
				regions = append(regions, wgpu.BufferTextureCopy{
					BufferLayout: wgpu.ImageDataLayout{
						Offset: uint64(layer) * uint64(layerBytes), BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: depthReadbackSize,
					},
					TextureBase: wgpu.ImageCopyTexture{
						Texture: tex, MipLevel: 1, Origin: wgpu.Origin3D{Z: layer}, Aspect: gputypes.TextureAspectStencilOnly,
					},
					Size: size,
				})
			}
			enc.CopyBufferToTexture(src, tex, regions)
		})
		for layer := uint32(0); layer < 2; layer++ {
			got := readStencilLevel(t, device, tex, 1, layer, depthReadbackSize)
			for i, v := range got {
				if want := byte(0x20*(int(layer)+1) + i); v != want {
					t.Fatalf("layer %d texel %d = %#x, want %#x", layer, i, v, want)
				}
			}
		}
	})
}

// TestStencilUploadQueueOrder mixes the two upload paths on one queue. A
// WriteTexture after a submitted copy must win. A copy recorded before a
// WriteTexture but submitted after it must win too: the encoder's commands
// run when it is submitted, not when they were recorded.
func TestStencilUploadQueueOrder(t *testing.T) {
	const texels = depthReadbackSize * depthReadbackSize
	forEachStencilFormat(t, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat) {
		copied, written := bytes.Repeat([]byte{0x33}, texels), bytes.Repeat([]byte{0xC4}, texels)
		check := func(t *testing.T, tex *wgpu.Texture, want byte) {
			t.Helper()
			for i, v := range readStencilLevel(t, device, tex, 0, 0, depthReadbackSize) {
				if v != want {
					t.Fatalf("texel %d = %#x, want %#x", i, v, want)
				}
			}
		}

		t.Run("copy-then-write", func(t *testing.T) {
			tex, _ := newReadbackTexture(t, device, "stencil-order-a", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
			uploadStencil(t, device, tex, 0, wgpu.Origin3D{}, depthReadbackExtent, copied, false)
			uploadStencil(t, device, tex, 0, wgpu.Origin3D{}, depthReadbackExtent, written, true)
			check(t, tex, written[0])
		})
		t.Run("record-write-submit", func(t *testing.T) {
			tex, _ := newReadbackTexture(t, device, "stencil-order-b", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
			src := newStencilSource(t, device, copied)
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{stencilCopyRegion(tex)})
			uploadStencil(t, device, tex, 0, wgpu.Origin3D{}, depthReadbackExtent, written, true)
			cb, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			if _, err := device.Queue().Submit(cb); err != nil {
				t.Fatalf("Submit: %v", err)
			}
			check(t, tex, copied[0])
		})
	})
}

// TestStencilUploadPartialRejected checks that a stencil upload that does
// not cover the whole mip level in x and y is refused on every backend,
// GLES's blit path included: a depth/stencil copy always covers whole levels
// (WebGPU "validating texture copy range").
func TestStencilUploadPartialRejected(t *testing.T) {
	forEachStencilFormat(t, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat) {
		tex, _ := newReadbackTexture(t, device, "stencil-partial", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
		dst := wgpu.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectStencilOnly}
		half := wgpu.Extent3D{Width: depthReadbackSize / 2, Height: depthReadbackSize / 2, DepthOrArrayLayers: 1}
		checkPartial := func(t *testing.T, err error) {
			t.Helper()
			var te *core.TransferError
			if !errors.As(err, &te) || te.Kind != core.TransferErrorUnsupportedPartialTransfer {
				t.Fatalf("error %v, want TransferErrorUnsupportedPartialTransfer", err)
			}
		}

		t.Run("write-texture", func(t *testing.T) {
			layout := wgpu.ImageDataLayout{BytesPerRow: half.Width, RowsPerImage: half.Height}
			checkPartial(t, device.Queue().WriteTexture(&dst, make([]byte, half.Width*half.Height), &layout, &half))
		})
		t.Run("copy-buffer", func(t *testing.T) {
			src := newStencilSource(t, device, make([]byte, depthReadbackSize*depthReadbackSize))
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
				BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: half.Height},
				TextureBase:  dst,
				Size:         half,
			}})
			_, err = enc.Finish()
			checkPartial(t, err)
		})
	})
}

// TestStencilUploadInsidePassRejected records a stencil copy while a render
// pass of the encoder is open. The encoder is locked, so the copy must be
// refused (and Finish must fail) before the blit could begin a pass of its
// own.
func TestStencilUploadInsidePassRejected(t *testing.T) {
	forEachStencilFormat(t, func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat) {
		tex, _ := newReadbackTexture(t, device, "stencil-in-pass", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
		_, colorView := newReadbackTexture(t, device, "stencil-in-pass-color", gputypes.TextureFormatRGBA8Unorm,
			wgpu.TextureUsageRenderAttachment)
		src := newStencilSource(t, device, make([]byte, depthReadbackSize*depthReadbackSize))
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{stencilCopyRegion(tex)})
		// The locked encoder turns invalid, so End already reports the error.
		endErr := pass.End()
		_, finishErr := enc.Finish()
		for _, err := range []error{endErr, finishErr} {
			if err == nil || !strings.Contains(err.Error(), "CommandEncoder.CopyBufferToTexture: encoder in Locked state") {
				t.Fatalf("End: %v, Finish: %v; want both to report the copy on the locked encoder", endErr, finishErr)
			}
		}
	})
}

// forEachStencilFormat runs fn for every backend and combined depth/stencil
// format, with forEachStencilUpload's skips; fn picks its upload paths.
func forEachStencilFormat(t *testing.T, fn func(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat)) {
	t.Helper()
	d32s8 := gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	forEachOverrideBackendFeatures(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, d32s8, func(t *testing.T, device *wgpu.Device) {
		for _, format := range combinedStencilFormats {
			t.Run(format.String(), func(t *testing.T) {
				if format == gputypes.TextureFormatDepth32FloatStencil8 &&
					!device.Features().Contains(gputypes.FeatureDepth32FloatStencil8) {
					t.Skip("the adapter has no depth32float-stencil8")
				}
				fn(t, device, format)
			})
		}
	})
}

// newStencilSource returns a copy source buffer holding tight (rows of
// depthReadbackSize bytes) at depthReadbackBytesPerRow rows.
func newStencilSource(t *testing.T, device *wgpu.Device, tight []byte) *wgpu.Buffer {
	t.Helper()
	padded := make([]byte, depthReadbackBytesPerRow*depthReadbackSize)
	for y := 0; y < depthReadbackSize; y++ {
		copy(padded[y*depthReadbackBytesPerRow:], tight[y*depthReadbackSize:(y+1)*depthReadbackSize])
	}
	src := newPublicBuffer(t, device, uint64(len(padded)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(src, 0, padded); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return src
}

// stencilCopyRegion is the whole stencil aspect of mip level 0 of a
// depthReadbackSize square texture, from a newStencilSource buffer.
func stencilCopyRegion(tex *wgpu.Texture) wgpu.BufferTextureCopy {
	return wgpu.BufferTextureCopy{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: depthReadbackSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectStencilOnly},
		Size:         depthReadbackExtent,
	}
}

// readStencilLevel reads the stencil aspect of mip level mip and array layer
// layer of tex, a size-square level, and returns its texels tightly packed.
func readStencilLevel(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, mip, layer, size uint32) []byte {
	t.Helper()
	bufSize := uint64(depthReadbackBytesPerRow) * uint64(size)
	staging := newPublicBuffer(t, device, bufSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: size},
			TextureBase: wgpu.ImageCopyTexture{
				Texture: tex, MipLevel: mip, Origin: wgpu.Origin3D{Z: layer}, Aspect: gputypes.TextureAspectStencilOnly,
			},
			Size: wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		}})
	})
	data := mapStaging(t, staging, bufSize)
	tight := make([]byte, 0, size*size)
	for y := uint32(0); y < size; y++ {
		tight = append(tight, data[y*depthReadbackBytesPerRow:][:size]...)
	}
	return tight
}
