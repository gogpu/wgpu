// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

const (
	depthReadbackSize        = 4
	depthReadbackBytesPerRow = 256
	depthReadbackDepth16     = 0.25 // 16383.75 in 16-bit unorm
	depthReadbackDepth32     = 0.75
	depthReadbackStencil     = 0xA5
)

// TestDepthReadback reads the depth and the stencil aspect of every format a
// buffer copy may read (WebGPU "depth-stencil formats" copy rules:
// depth16unorm and depth32float as depth, every stencil aspect as stencil8;
// depth24plus is not copyable) back after a render pass cleared them.
// GLES read every depth and stencil texture through COLOR_ATTACHMENT0, so the
// framebuffer was incomplete and the copy wrote nothing.
func TestDepthReadback(t *testing.T) {
	cases := []struct {
		format gputypes.TextureFormat
		aspect gputypes.TextureAspect
	}{
		{gputypes.TextureFormatDepth16Unorm, gputypes.TextureAspectDepthOnly},
		{gputypes.TextureFormatDepth32Float, gputypes.TextureAspectDepthOnly},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectDepthOnly},
		{gputypes.TextureFormatStencil8, gputypes.TextureAspectStencilOnly},
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectStencilOnly},
	}
	d32s8 := gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	forEachOverrideBackendFeatures(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, d32s8, func(t *testing.T, device *wgpu.Device) {
		software := strings.HasSuffix(t.Name(), "/software")
		for _, c := range cases {
			name := c.format.String() + "/depth"
			if c.aspect == gputypes.TextureAspectStencilOnly {
				name = c.format.String() + "/stencil"
			}
			t.Run(name, func(t *testing.T) {
				if c.format == gputypes.TextureFormatDepth32FloatStencil8 &&
					!device.Features().Contains(gputypes.FeatureDepth32FloatStencil8) {
					t.Skip("the adapter has no depth32float-stencil8")
				}
				if software && c.aspect == gputypes.TextureAspectStencilOnly &&
					c.format != gputypes.TextureFormatDepth32FloatStencil8 {
					t.Skip("software: stencil aspect is not stored, P1-SW-021")
				}
				data := clearAndReadBack(t, device, c.format, c.aspect)
				checkDepthReadback(t, c.format, c.aspect, data)
			})
		}
	})
}

// TestDepthStencilUploadRoundTrip uploads a texel pattern into every aspect a
// buffer copy may write (depth16unorm and every stencil aspect), through
// WriteTexture with tightly packed rows (the backends' staging repack) and
// through CopyBufferToTexture at 256-byte rows, and reads it back. DX12 put
// the resource format (D24_UNORM_S8_UINT, D32_FLOAT_S8X24_UINT) in the placed
// footprint of a stencil plane, so the copy failed or moved nothing. GLES
// reports StencilCopyNeedsBlit (GL uploads depth and stencil only together),
// and the frontend writes the stencil aspect of its combined formats with a
// draw instead; before that the copy was refused.
func TestDepthStencilUploadRoundTrip(t *testing.T) {
	cases := []struct {
		format gputypes.TextureFormat
		aspect gputypes.TextureAspect
	}{
		{gputypes.TextureFormatDepth16Unorm, gputypes.TextureAspectAll},
		{gputypes.TextureFormatStencil8, gputypes.TextureAspectAll},
		{gputypes.TextureFormatStencil8, gputypes.TextureAspectStencilOnly},
		{gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly},
		{gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectStencilOnly},
	}
	d32s8 := gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	forEachOverrideBackendFeatures(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, d32s8, func(t *testing.T, device *wgpu.Device) {
		software := strings.HasSuffix(t.Name(), "/software")
		for _, c := range cases {
			for _, path := range []string{"write-texture", "copy-buffer"} {
				t.Run(c.format.String()+"/"+aspectName(c.aspect)+"/"+path, func(t *testing.T) {
					if c.format == gputypes.TextureFormatDepth32FloatStencil8 &&
						!device.Features().Contains(gputypes.FeatureDepth32FloatStencil8) {
						t.Skip("the adapter has no depth32float-stencil8")
					}
					if software && c.format != gputypes.TextureFormatDepth16Unorm &&
						c.format != gputypes.TextureFormatDepth32FloatStencil8 {
						t.Skip("software: stencil aspect is not stored, P1-SW-021")
					}
					uploadAndCheck(t, device, c.format, c.aspect, path == "write-texture")
				})
			}
		}
	})
}

// TestCopyOnlyTextureView creates a view of textures whose only usages are
// copies. Vulkan created their images with transfer usages alone and made a
// VkImageView anyway, which fails VUID-VkImageViewCreateInfo-image-04441
// under the validation layer.
func TestCopyOnlyTextureView(t *testing.T) {
	formats := []gputypes.TextureFormat{
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatR32Float,
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatStencil8,
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, format := range formats {
			t.Run(format.String(), func(t *testing.T) {
				newReadbackTexture(t, device, "copy-only", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
			})
		}
	})
}

func aspectName(aspect gputypes.TextureAspect) string {
	switch aspect {
	case gputypes.TextureAspectDepthOnly:
		return "depth"
	case gputypes.TextureAspectStencilOnly:
		return "stencil"
	}
	return "all"
}

// uploadAndCheck writes a pattern into aspect of a new texture of format and
// checks that CopyTextureToBuffer returns it.
func uploadAndCheck(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, aspect gputypes.TextureAspect, viaWriteTexture bool) {
	t.Helper()
	bpp := 1
	if format == gputypes.TextureFormatDepth16Unorm {
		bpp = 2
	}
	tight := make([]byte, depthReadbackSize*depthReadbackSize*bpp)
	for i := 0; i < depthReadbackSize*depthReadbackSize; i++ {
		if bpp == 2 {
			binary.LittleEndian.PutUint16(tight[i*2:], uint16(1000*(i+1)))
		} else {
			tight[i] = byte(0x10 + i)
		}
	}
	tex, _ := newReadbackTexture(t, device, "upload-round-trip", format, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
	dst := wgpu.ImageCopyTexture{Texture: tex, Aspect: aspect}
	rowBytes := depthReadbackSize * bpp
	if viaWriteTexture {
		layout := wgpu.ImageDataLayout{BytesPerRow: uint32(rowBytes), RowsPerImage: depthReadbackSize}
		if err := device.Queue().WriteTexture(&dst, tight, &layout, &depthReadbackExtent); err != nil {
			t.Fatalf("WriteTexture: %v", err)
		}
	} else {
		padded := make([]byte, depthReadbackBytesPerRow*depthReadbackSize)
		for y := 0; y < depthReadbackSize; y++ {
			copy(padded[y*depthReadbackBytesPerRow:], tight[y*rowBytes:(y+1)*rowBytes])
		}
		src := newPublicBuffer(t, device, uint64(len(padded)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		if err := device.Queue().WriteBuffer(src, 0, padded); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
			enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
				BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: depthReadbackSize},
				TextureBase:  dst,
				Size:         depthReadbackExtent,
			}})
		})
	}
	got := readAspect(t, device, tex, aspect)
	for y := 0; y < depthReadbackSize; y++ {
		row, want := got[y*depthReadbackBytesPerRow:][:rowBytes], tight[y*rowBytes:(y+1)*rowBytes]
		if !bytes.Equal(row, want) {
			t.Fatalf("row %d = %x, want %x", y, row, want)
		}
	}
}

// clearAndReadBack clears a depth/stencil texture of format in a render pass
// and returns the aspect's rows as CopyTextureToBuffer wrote them.
func clearAndReadBack(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, aspect gputypes.TextureAspect) []byte {
	t.Helper()
	_, colorView := newReadbackTexture(t, device, "depth-readback-color", gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageRenderAttachment)
	depth, depthView := newReadbackTexture(t, device, "depth-readback-depth", format, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)

	ds := &wgpu.RenderPassDepthStencilAttachment{View: depthView}
	if format != gputypes.TextureFormatStencil8 {
		ds.DepthLoadOp, ds.DepthStoreOp, ds.DepthClearValue = gputypes.LoadOpClear, gputypes.StoreOpStore, depthPassValue(format)
	}
	if format != gputypes.TextureFormatDepth16Unorm && format != gputypes.TextureFormatDepth32Float {
		ds.StencilLoadOp, ds.StencilStoreOp, ds.StencilClearValue = gputypes.LoadOpClear, gputypes.StoreOpStore, depthReadbackStencil
	}

	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			}},
			DepthStencilAttachment: ds,
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	})
	return readAspect(t, device, depth, aspect)
}

var depthReadbackExtent = wgpu.Extent3D{Width: depthReadbackSize, Height: depthReadbackSize, DepthOrArrayLayers: 1}

func newReadbackTexture(t *testing.T, device *wgpu.Device, label string, format gputypes.TextureFormat,
	usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: label, Size: depthReadbackExtent, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: format, Usage: usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture %s: %v", label, err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView %s: %v", label, err)
	}
	t.Cleanup(view.Release)
	return tex, view
}

// readAspect copies aspect of tex into a buffer at depthReadbackBytesPerRow
// and returns the buffer's bytes.
func readAspect(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, aspect gputypes.TextureAspect) []byte {
	t.Helper()
	bufSize := uint64(depthReadbackBytesPerRow * depthReadbackSize)
	staging := newPublicBuffer(t, device, bufSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthReadbackBytesPerRow, RowsPerImage: depthReadbackSize},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex, Aspect: aspect},
			Size:         depthReadbackExtent,
		}})
	})
	return mapStaging(t, staging, bufSize)
}

// mapStaging maps the first bufSize bytes of staging for reading and returns
// a copy of them.
func mapStaging(t *testing.T, staging *wgpu.Buffer, bufSize uint64) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := staging.Map(ctx, wgpu.MapModeRead, 0, bufSize); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := staging.MappedRange(0, bufSize)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	data := append([]byte(nil), rng.Bytes()...)
	if err := staging.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	return data
}

// checkDepthReadback checks every texel of the rows in the aspect's buffer
// layout: depth16unorm as 16-bit unorm, depth32float as float32, stencil as
// one byte.
func checkDepthReadback(t *testing.T, format gputypes.TextureFormat, aspect gputypes.TextureAspect, data []byte) {
	t.Helper()
	for y := 0; y < depthReadbackSize; y++ {
		row := data[y*depthReadbackBytesPerRow:]
		for x := 0; x < depthReadbackSize; x++ {
			switch {
			case aspect == gputypes.TextureAspectStencilOnly:
				if got := row[x]; got != depthReadbackStencil {
					t.Fatalf("texel (%d,%d): stencil = %#x, want %#x", x, y, got, depthReadbackStencil)
				}
			case format == gputypes.TextureFormatDepth16Unorm:
				// 0.25 * 65535 = 16383.75; a backend may round either way.
				if got := binary.LittleEndian.Uint16(row[x*2:]); got != 16383 && got != 16384 {
					t.Fatalf("texel (%d,%d): depth = %d, want 16384 (%v)", x, y, got, depthReadbackDepth16)
				}
			default:
				// GLES reads 0.75000006 back from its depth store.
				got := math.Float32frombits(binary.LittleEndian.Uint32(row[x*4:]))
				if math.Abs(float64(got)-depthReadbackDepth32) > 1e-6 {
					t.Fatalf("texel (%d,%d): depth = %v, want %v", x, y, got, depthReadbackDepth32)
				}
			}
		}
	}
}
