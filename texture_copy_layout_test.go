//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// TestBufferTextureCopyLayoutValidation: the buffer layout of a buffer-texture
// copy must describe complete rows and images, be specified where the copy
// spans several rows or images, and fit the buffer (Rust wgpu-core
// validate_linear_texture_data, W3C "validating linear texture data").
func TestBufferTextureCopyLayoutValidation(t *testing.T) {
	rig := newCopyRig(t) // buffer of 16384 bytes; an 8x8 RGBA8 copy needs 7*256+32 = 1824
	layout := func(offset uint64, bytesPerRow, rowsPerImage uint32) wgpu.ImageDataLayout {
		return wgpu.ImageDataLayout{Offset: offset, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
	}

	tests := []struct {
		name   string
		layout wgpu.ImageDataLayout
		size   wgpu.Extent3D
		wide   bool // copy the 128x2 texture instead of the 8x8x4 one
		want   core.TransferErrorKind
	}{
		{name: "dense layout", layout: layout(0, 256, 8), size: extent(8, 8, 1), want: -1},
		{name: "whole texture, 4 layers", layout: layout(0, 256, 8), size: extent(8, 8, 4), want: -1},
		{name: "layout fits the buffer exactly", layout: layout(16384-1824, 256, 8), size: extent(8, 8, 1), want: -1},
		{name: "one row needs no bytesPerRow", layout: layout(0, 0, 0), size: extent(8, 1, 1), want: -1},
		{name: "one image needs no rowsPerImage", layout: layout(0, 256, 0), size: extent(8, 8, 1), want: -1},
		{name: "empty copy", layout: layout(0, 0, 0), size: extent(0, 0, 0), want: -1},
		{name: "bytesPerRow missing for several rows", layout: layout(0, 0, 8), size: extent(8, 2, 1),
			want: core.TransferErrorUnspecifiedBytesPerRow},
		{name: "bytesPerRow missing for several layers", layout: layout(0, 0, 8), size: extent(8, 1, 2),
			want: core.TransferErrorUnspecifiedBytesPerRow},
		{name: "rowsPerImage missing for several layers", layout: layout(0, 256, 0), size: extent(8, 8, 2),
			want: core.TransferErrorUnspecifiedRowsPerImage},
		{name: "bytesPerRow smaller than a row", layout: layout(0, 256, 2), size: extent(128, 2, 1), wide: true,
			want: core.TransferErrorInvalidBytesPerRow},
		{name: "rowsPerImage smaller than the image", layout: layout(0, 256, 4), size: extent(8, 8, 1),
			want: core.TransferErrorInvalidRowsPerImage},
		{name: "copy overruns the buffer", layout: layout(16384-1820, 256, 8), size: extent(8, 8, 1),
			want: core.TransferErrorBufferEndOffsetOverrun},
		{name: "offset past the buffer", layout: layout(20000, 256, 8), size: extent(8, 8, 1),
			want: core.TransferErrorBufferStartOffsetOverrun},
	}
	for _, tc := range tests {
		tex := rig.color
		if tc.wide {
			tex = rig.wide
		}
		t.Run("texture to buffer, "+tc.name, func(t *testing.T) {
			got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
				enc.CopyTextureToBuffer(tex, rig.buf, btCopy(0, wgpu.Origin3D{}, tc.layout, tc.size))
			})
			if got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
		t.Run("buffer to texture, "+tc.name, func(t *testing.T) {
			got := rig.finishKind(t, func(enc *wgpu.CommandEncoder) {
				enc.CopyBufferToTexture(rig.buf, tex, btCopy(0, wgpu.Origin3D{}, tc.layout, tc.size))
			})
			if got != tc.want {
				t.Fatalf("error kind = %d, want %d", got, tc.want)
			}
		})
	}
}
