// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
)

// copyRoundTripFormats are the uncompressed color formats of the WebGPU
// spec, all of them copyable both ways. The 16-bit normalized formats are a
// native extension and are left out.
var copyRoundTripFormats = []gputypes.TextureFormat{
	gputypes.TextureFormatR8Unorm, gputypes.TextureFormatR8Snorm,
	gputypes.TextureFormatR8Uint, gputypes.TextureFormatR8Sint,
	gputypes.TextureFormatR16Uint, gputypes.TextureFormatR16Sint, gputypes.TextureFormatR16Float,
	gputypes.TextureFormatRG8Unorm, gputypes.TextureFormatRG8Snorm,
	gputypes.TextureFormatRG8Uint, gputypes.TextureFormatRG8Sint,
	gputypes.TextureFormatR32Uint, gputypes.TextureFormatR32Sint, gputypes.TextureFormatR32Float,
	gputypes.TextureFormatRG16Uint, gputypes.TextureFormatRG16Sint, gputypes.TextureFormatRG16Float,
	gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb, gputypes.TextureFormatRGBA8Snorm,
	gputypes.TextureFormatRGBA8Uint, gputypes.TextureFormatRGBA8Sint,
	gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatBGRA8UnormSrgb,
	gputypes.TextureFormatRGB10A2Uint, gputypes.TextureFormatRGB10A2Unorm,
	gputypes.TextureFormatRG11B10Ufloat, gputypes.TextureFormatRGB9E5Ufloat,
	gputypes.TextureFormatRG32Uint, gputypes.TextureFormatRG32Sint, gputypes.TextureFormatRG32Float,
	gputypes.TextureFormatRGBA16Uint, gputypes.TextureFormatRGBA16Sint, gputypes.TextureFormatRGBA16Float,
	gputypes.TextureFormatRGBA32Uint, gputypes.TextureFormatRGBA32Sint, gputypes.TextureFormatRGBA32Float,
}

// TestTextureFormatCopyRoundTrip: the texels of every color format come back
// from CopyBufferToTexture and CopyTextureToBuffer byte for byte. A backend
// that stores a format as another one (GLES created every format it had no
// row for as RGBA8, so R8Uint read back four bytes a texel and the integer
// targets read zeros) returns other bytes. The bytes are 0x30..0x3f: normal
// float16, float32 and packed-float values and no snorm -128, so no value
// has two encodings a backend may canonicalize between.
func TestTextureFormatCopyRoundTrip(t *testing.T) {
	const width, height, rowStride = 4, 2, 256
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, format := range copyRoundTripFormats {
			t.Run(format.String(), func(t *testing.T) {
				rowBytes := width * int(format.BlockCopySize())
				upload := make([]byte, rowStride*height)
				for y := range height {
					for x := range rowBytes {
						upload[y*rowStride+x] = byte(0x30 + (y*rowBytes+x)%16)
					}
				}
				drops := captureDroppedCopies(t)
				out := formatCopyRoundTrip(t, device, format, upload, width, height, rowStride)
				if dropped, status := drops.seen(); dropped {
					t.Logf("framebuffer status %s", status)
					if format == gputypes.TextureFormatRGB9E5Ufloat {
						t.Skip("gles: RGB9E5 is not color-renderable on this context, readback needs a compute blit, P2-GLES-023")
					}
					t.Fatalf("the HAL dropped the copy of %v: its source framebuffer is incomplete", format)
				}
				for y := range height {
					want := upload[y*rowStride : y*rowStride+rowBytes]
					if got := out[y*rowStride : y*rowStride+rowBytes]; !bytes.Equal(got, want) {
						t.Errorf("row %d = % x, want % x", y, got, want)
					}
				}
			})
		}
	})
}

// glesCopyDroppedMsg is the error hal/gles logs when glReadPixels has no
// complete framebuffer for a copy's source and the copy is dropped.
const glesCopyDroppedMsg = "gles: copy source framebuffer is incomplete, the copy is dropped"

// dropCapture notes whether the GLES HAL dropped a texture-to-buffer copy,
// and forwards every record to the handler it replaced (the Vulkan validation
// capture among them).
type dropCapture struct {
	next    slog.Handler
	mu      sync.Mutex
	dropped bool
	status  string // the GL framebuffer status the HAL logged
}

func (c *dropCapture) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelError || c.next.Enabled(ctx, level)
}

func (c *dropCapture) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler takes the Record by value
	if r.Message == glesCopyDroppedMsg {
		status := ""
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "status" {
				status = a.Value.String()
			}
			return true
		})
		c.mu.Lock()
		c.dropped = true
		c.status = status
		c.mu.Unlock()
	}
	if c.next.Enabled(ctx, r.Level) {
		return c.next.Handle(ctx, r)
	}
	return nil
}

func (c *dropCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *dropCapture) WithGroup(string) slog.Handler      { return c }

// seen reports whether a copy was dropped, and the framebuffer status.
func (c *dropCapture) seen() (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped, c.status
}

// captureDroppedCopies installs a dropCapture until the test ends.
func captureDroppedCopies(t *testing.T) *dropCapture {
	t.Helper()
	prev := hal.Logger()
	capture := &dropCapture{next: prev.Handler()}
	hal.SetLogger(slog.New(capture))
	t.Cleanup(func() { hal.SetLogger(prev) })
	return capture
}

func formatCopyRoundTrip(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, upload []byte, width, height, rowStride uint32) []byte {
	t.Helper()
	size := wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1}
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "format-copy",
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	layout := wgpu.ImageDataLayout{BytesPerRow: rowStride, RowsPerImage: height}
	src := newPublicBuffer(t, device, uint64(len(upload)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(src, 0, upload); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	staging := newPublicBuffer(t, device, uint64(len(upload)), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
			BufferLayout: layout, TextureBase: wgpu.ImageCopyTexture{Texture: tex}, Size: size,
		}})
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: layout, TextureBase: wgpu.ImageCopyTexture{Texture: tex}, Size: size,
		}})
	})
	out := make([]byte, 0, len(upload))
	for _, w := range readPublicBuffer(t, staging, len(upload)/4) {
		out = binary.LittleEndian.AppendUint32(out, w)
	}
	return out
}
