//go:build !rust && !(js && wasm)

package queue

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// newTestUploadTexture builds the upload target of one table row: an RGBA8Unorm
// texture of 4x4 with two array layers (one for a multisampled texture), the
// state the core upload validation reads. The fixtures build the core texture
// directly: the destination cannot call the public CreateTexture.
func newTestUploadTexture(t *testing.T, device *core.Device, host *testHost, usage gputypes.TextureUsage, mips, samples uint32) *resource.Texture {
	t.Helper()
	layers := uint32(2)
	if samples > 1 {
		layers = 1 // a multisampled texture has one layer
	}
	coreTex := core.NewTexture(
		testHALTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		usage,
		gputypes.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: layers},
		mips, samples, "upload",
	)
	t.Cleanup(coreTex.Destroy)
	tex := resource.NewTexture(testHALTexture{}, host, gputypes.TextureFormatRGBA8Unorm, nil, coreTex)
	// The root package's version created its textures through the public
	// device and reported a creation failure; the destination builds the core
	// texture directly, so the same precondition is checked here.
	if tex.CoreTexture() == nil || !tex.CoreTexture().TrackingData().Index().IsValid() {
		t.Fatalf("upload texture has no tracked core texture")
	}
	return tex
}

// TestQueueWriteTextureValidation: WriteTexture validates the upload as Rust
// wgpu-core write_texture does (usage, mip level and range, aspect, format,
// sample count, data layout against the data length) and reports a
// *core.TransferError before the backend sees the data; a valid upload of size
// zero is dropped after validation.
//
// It moved here with the queue (ADR-070 S9) and keeps its own fixtures: the
// root package's copy-zero-size fixture set stays there, because the encoder's
// copy paths still test against it.
func TestQueueWriteTextureValidation(t *testing.T) {
	host, device := newTestHost(t, false)
	halQueue := &testHALQueue{}
	q := New(halQueue, nil)
	q.SetDevice(host)

	newTexture := func(usage gputypes.TextureUsage, mips, samples uint32) *resource.Texture {
		return newTestUploadTexture(t, device, host, usage, mips, samples)
	}
	color := newTexture(gputypes.TextureUsageCopyDst, 2, 1)
	noCopyDst := newTexture(gputypes.TextureUsageCopySrc, 1, 1)
	multisampled := newTexture(gputypes.TextureUsageCopyDst|gputypes.TextureUsageRenderAttachment, 1, 4)

	extent := func(w, h, d uint32) *Extent3D { return &Extent3D{Width: w, Height: h, DepthOrArrayLayers: d} }
	rows := func(bytesPerRow, rowsPerImage uint32) *ImageDataLayout {
		return &ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
	}

	tests := []struct {
		name       string
		dst        *ImageCopyTexture
		dataLen    int
		layout     *ImageDataLayout
		size       *Extent3D
		wantKind   core.TransferErrorKind
		wantErr    bool
		wantWrites int
	}{
		{name: "valid full mip", dst: &ImageCopyTexture{Texture: color}, dataLen: 64,
			layout: rows(16, 4), size: extent(4, 4, 1), wantWrites: 1},
		{name: "valid with unaligned bytesPerRow", dst: &ImageCopyTexture{Texture: color}, dataLen: 20*3 + 16,
			layout: rows(20, 4), size: extent(4, 4, 1), wantWrites: 1},
		{name: "valid second mip", dst: &ImageCopyTexture{Texture: color, MipLevel: 1}, dataLen: 16,
			layout: rows(8, 2), size: extent(2, 2, 1), wantWrites: 1},
		{name: "texture without COPY_DST", dst: &ImageCopyTexture{Texture: noCopyDst}, dataLen: 64,
			layout: rows(16, 4), size: extent(4, 4, 1), wantKind: core.TransferErrorMissingTextureUsage, wantErr: true},
		{name: "mip level out of range", dst: &ImageCopyTexture{Texture: color, MipLevel: 2}, dataLen: 64,
			layout: rows(16, 4), size: extent(1, 1, 1), wantKind: core.TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "region past the texture", dst: &ImageCopyTexture{Texture: color}, dataLen: 1024,
			layout: rows(32, 4), size: extent(8, 4, 1), wantKind: core.TransferErrorTextureOverrun, wantErr: true},
		{name: "region past the layers", dst: &ImageCopyTexture{Texture: color, Origin: Origin3D{Z: 1}}, dataLen: 1024,
			layout: rows(16, 4), size: extent(4, 4, 2), wantKind: core.TransferErrorTextureOverrun, wantErr: true},
		{name: "aspect missing in the format", dst: &ImageCopyTexture{Texture: color, Aspect: gputypes.TextureAspectStencilOnly}, dataLen: 64,
			layout: rows(16, 4), size: extent(4, 4, 1), wantKind: core.TransferErrorInvalidTextureAspect, wantErr: true},
		{name: "multisampled texture", dst: &ImageCopyTexture{Texture: multisampled}, dataLen: 64,
			layout: rows(16, 4), size: extent(4, 4, 1), wantKind: core.TransferErrorInvalidSampleCount, wantErr: true},
		{name: "data shorter than the layout", dst: &ImageCopyTexture{Texture: color}, dataLen: 63,
			layout: rows(16, 4), size: extent(4, 4, 1), wantKind: core.TransferErrorBufferEndOffsetOverrun, wantErr: true},
		// The WebGPU minimum: offset + bytesPerRow*rowsPerImage*(layers-1) +
		// bytesPerRow*(rows-1) + bytesInLastRow, with no padding after the
		// last row and no padding rows after the last image.
		{name: "valid two layers, data ends at the last row", dst: &ImageCopyTexture{Texture: color}, dataLen: 4 + 20*5 + 20*3 + 12,
			layout: &ImageDataLayout{Offset: 4, BytesPerRow: 20, RowsPerImage: 5}, size: extent(3, 4, 2), wantWrites: 1},
		{name: "two layers, data one byte short of the last row", dst: &ImageCopyTexture{Texture: color}, dataLen: 4 + 20*5 + 20*3 + 11,
			layout: &ImageDataLayout{Offset: 4, BytesPerRow: 20, RowsPerImage: 5}, size: extent(3, 4, 2),
			wantKind: core.TransferErrorBufferEndOffsetOverrun, wantErr: true},
		{name: "offset past the data", dst: &ImageCopyTexture{Texture: color}, dataLen: 8,
			layout: &ImageDataLayout{Offset: 16, BytesPerRow: 16, RowsPerImage: 4}, size: extent(4, 1, 1),
			wantKind: core.TransferErrorBufferStartOffsetOverrun, wantErr: true},
		{name: "bytesPerRow missing for several rows", dst: &ImageCopyTexture{Texture: color}, dataLen: 64,
			layout: rows(0, 4), size: extent(4, 4, 1), wantKind: core.TransferErrorUnspecifiedBytesPerRow, wantErr: true},
		{name: "bytesPerRow shorter than a row", dst: &ImageCopyTexture{Texture: color}, dataLen: 64,
			layout: rows(8, 4), size: extent(4, 4, 1), wantKind: core.TransferErrorInvalidBytesPerRow, wantErr: true},
		{name: "zero width is dropped after validation", dst: &ImageCopyTexture{Texture: color}, dataLen: 64,
			layout: rows(16, 4), size: extent(0, 4, 1)},
		{name: "zero depth is dropped after validation", dst: &ImageCopyTexture{Texture: color}, dataLen: 64,
			layout: rows(16, 4), size: extent(4, 4, 0)},
		{name: "zero-size upload to a texture without COPY_DST is still an error", dst: &ImageCopyTexture{Texture: noCopyDst},
			layout: rows(0, 0), size: extent(0, 0, 0), wantKind: core.TransferErrorMissingTextureUsage, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := halQueue.writeTextures
			err := q.WriteTexture(tt.dst, make([]byte, tt.dataLen), tt.layout, tt.size)
			if tt.wantErr {
				var te *core.TransferError
				if !errors.As(err, &te) || te.Kind != tt.wantKind {
					t.Fatalf("WriteTexture = %v (%T), want *core.TransferError kind %d", err, err, tt.wantKind)
				}
			} else if err != nil {
				t.Fatalf("WriteTexture: %v", err)
			}
			if got := halQueue.writeTextures - before; got != tt.wantWrites {
				t.Errorf("backend writes = %d, want %d", got, tt.wantWrites)
			}
		})
	}
}
