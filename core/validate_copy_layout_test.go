//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// bytesInCopy returns the number of buffer bytes the layout covers for a copy,
// read from the end-overrun error of a buffer that ends at the layout's
// offset, or 0 when the copy needs no bytes. It fails the test for any other
// error.
func bytesInCopy(t *testing.T, layout TexelCopyBufferLayout, format gputypes.TextureFormat, size gputypes.Extent3D) uint64 {
	t.Helper()
	err := ValidateLinearTextureData(layout, format, gputypes.TextureAspectAll, layout.Offset, CopySideDestination, size)
	if err == nil {
		return 0
	}
	var te *TransferError
	if !errors.As(err, &te) || te.Kind != TransferErrorBufferEndOffsetOverrun {
		t.Fatalf("unexpected error: %v", err)
	}
	return te.Size
}

// TestValidateLinearTextureDataBytes pins the byte size of a copy with the
// cases of the wgpu-types tests of get_buffer_texture_copy_info
// (wgpu-types/src/transfers.rs, commit 14328f4): 1D, 2D/3D and compressed
// copies, with and without padding between rows and images.
func TestValidateLinearTextureDataBytes(t *testing.T) {
	t.Parallel()
	rgba, bc1 := gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatBC1RGBAUnorm
	layout := func(offset uint64, bytesPerRow, rowsPerImage uint32) TexelCopyBufferLayout {
		return TexelCopyBufferLayout{Offset: offset, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
	}
	tests := []struct {
		name   string
		layout TexelCopyBufferLayout
		format gputypes.TextureFormat
		size   gputypes.Extent3D
		want   uint64
	}{
		{"1d copy", layout(0, 0, 0), rgba, ext(4, 1, 1), 16},
		{"1d copy, padded row", layout(0, 32, 0), rgba, ext(4, 1, 1), 16},
		{"1d copy, padded image", layout(0, 32, 4), rgba, ext(4, 1, 1), 16},
		{"1d copy, offset", layout(4, 32, 4), rgba, ext(4, 1, 1), 16},
		{"2d copy", layout(0, 28, 12), rgba, ext(7, 12, 1), 4 * 7 * 12},
		{"2d copy, padded rows", layout(0, 48, 0), rgba, ext(7, 12, 1), 48*11 + 4*7},
		{"3d copy, 4 layers", layout(0, 48, 12), rgba, ext(7, 12, 4), 48*12*3 + 48*11 + 4*7},
		{"3d copy, padded images", layout(0, 48, 20), rgba, ext(7, 12, 4), 48*20*3 + 48*11 + 4*7},
		{"bc1 copy", layout(0, 16, 4), bc1, ext(7, 13, 1), 8 * 2 * 4},
		{"bc1 copy, padded rows", layout(0, 48, 0), bc1, ext(7, 13, 1), 48*3 + 8*2},
		{"bc1 copy, padded images", layout(0, 48, 8), bc1, ext(7, 13, 1), 48*3 + 8*2},
		{"bc1 copy, 4 layers", layout(0, 48, 8), bc1, ext(7, 13, 4), 48*8*3 + 48*3 + 8*2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := bytesInCopy(t, tt.layout, tt.format, tt.size); got != tt.want {
				t.Errorf("bytes in copy = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestValidateLinearTextureData pins the checks of Rust wgpu-core
// validate_linear_texture_data: complete rows and images, specified strides
// where they apply, overflow of the copy size and the buffer bounds. The
// overflow cases are those of the wgpu-types tests.
func TestValidateLinearTextureData(t *testing.T) {
	t.Parallel()
	rgba := gputypes.TextureFormatRGBA8Unorm
	layout := func(offset uint64, bytesPerRow, rowsPerImage uint32) TexelCopyBufferLayout {
		return TexelCopyBufferLayout{Offset: offset, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
	}
	const big = uint64(1) << 62
	tests := []struct {
		name       string
		layout     TexelCopyBufferLayout
		format     gputypes.TextureFormat
		aspect     gputypes.TextureAspect
		bufferSize uint64
		size       gputypes.Extent3D
		want       TransferErrorKind
		wantErr    bool
	}{
		{name: "fits exactly", layout: layout(0, 48, 12), format: rgba, bufferSize: 48*11 + 28, size: ext(7, 12, 1)},
		{name: "fits exactly with offset", layout: layout(100, 48, 12), format: rgba, bufferSize: 100 + 48*11 + 28, size: ext(7, 12, 1)},
		{name: "empty copy needs nothing", layout: layout(0, 0, 0), format: rgba, bufferSize: 0, size: ext(0, 0, 0)},
		{name: "zero layers, several rows", layout: layout(0, 0, 0), format: rgba, bufferSize: 0, size: ext(7, 12, 0),
			want: TransferErrorUnspecifiedBytesPerRow, wantErr: true},
		{name: "bytesPerRow too small", layout: layout(0, 20, 0), format: rgba, bufferSize: big, size: ext(7, 12, 1),
			want: TransferErrorInvalidBytesPerRow, wantErr: true},
		{name: "rowsPerImage too small", layout: layout(0, 48, 8), format: rgba, bufferSize: big, size: ext(7, 12, 1),
			want: TransferErrorInvalidRowsPerImage, wantErr: true},
		{name: "bytesPerRow unspecified, several rows", layout: layout(0, 0, 12), format: rgba, bufferSize: big, size: ext(7, 12, 1),
			want: TransferErrorUnspecifiedBytesPerRow, wantErr: true},
		{name: "bytesPerRow unspecified, several layers", layout: layout(0, 0, 1), format: rgba, bufferSize: big, size: ext(7, 1, 2),
			want: TransferErrorUnspecifiedBytesPerRow, wantErr: true},
		{name: "rowsPerImage unspecified, several layers", layout: layout(0, 28, 0), format: rgba, bufferSize: big, size: ext(7, 1, 2),
			want: TransferErrorUnspecifiedRowsPerImage, wantErr: true},
		{name: "rowsPerImage unspecified, one layer", layout: layout(0, 28, 0), format: rgba, bufferSize: big, size: ext(7, 12, 1)},
		{name: "overflow, width and height at the maximum", layout: layout(0, 0, 0), format: rgba, bufferSize: big,
			size: ext(^uint32(0), ^uint32(0), 1), want: TransferErrorSizeOverflow, wantErr: true},
		{name: "overflow, last row addition", layout: layout(0, 0, 0), format: rgba, bufferSize: big,
			size: ext(0x8000_0000, 0x8000_0000, 1), want: TransferErrorSizeOverflow, wantErr: true},
		{name: "overflow, image stride", layout: layout(0, 0, 0x8000_0000), format: rgba, bufferSize: big,
			size: ext(0x8000_0000, 12, 1), want: TransferErrorSizeOverflow, wantErr: true},
		{name: "overflow, layers times image stride", layout: layout(0, 0, 0), format: rgba, bufferSize: big,
			size: ext(0x1_0000, 0x1_0000, 0x8000_0000), want: TransferErrorSizeOverflow, wantErr: true},
		{name: "overflow, total size", layout: layout(0, 0, 0), format: rgba, bufferSize: big,
			size: ext(0x1_0001, 0x1_0001, 0x3fff_8001), want: TransferErrorSizeOverflow, wantErr: true},
		{name: "offset past the buffer", layout: layout(65, 48, 12), format: rgba, bufferSize: 64, size: ext(1, 1, 1),
			want: TransferErrorBufferStartOffsetOverrun, wantErr: true},
		{name: "offset at the end of the buffer, empty copy", layout: layout(64, 0, 0), format: rgba, bufferSize: 64, size: ext(0, 0, 0)},
		{name: "range one byte too long", layout: layout(0, 48, 12), format: rgba, bufferSize: 48*11 + 27, size: ext(7, 12, 1),
			want: TransferErrorBufferEndOffsetOverrun, wantErr: true},
		// Several images: every image but the last spans rowsPerImage rows,
		// and the last ends at bytesInLastRow (offset + bytesPerRow *
		// rowsPerImage * (layers-1) + bytesPerRow * (rows-1) + bytesInLastRow).
		{name: "several layers fit exactly", layout: layout(8, 48, 14), format: rgba, bufferSize: 8 + 2*48*14 + 48*11 + 28, size: ext(7, 12, 3)},
		{name: "several layers, one byte too short", layout: layout(8, 48, 14), format: rgba, bufferSize: 8 + 2*48*14 + 48*11 + 27, size: ext(7, 12, 3),
			want: TransferErrorBufferEndOffsetOverrun, wantErr: true},
		{name: "offset plus size does not wrap", layout: layout(^uint64(0)-3, 0, 0), format: rgba, bufferSize: ^uint64(0), size: ext(2, 1, 1),
			want: TransferErrorBufferEndOffsetOverrun, wantErr: true},
		// The aspect picks the block size of a combined depth/stencil format.
		{name: "stencil of depth24plus-stencil8", layout: layout(0, 8, 0), format: gputypes.TextureFormatDepth24PlusStencil8,
			aspect: gputypes.TextureAspectStencilOnly, bufferSize: 8, size: ext(8, 1, 1)},
		{name: "depth of depth32float-stencil8", layout: layout(0, 8, 0), format: gputypes.TextureFormatDepth32FloatStencil8,
			aspect: gputypes.TextureAspectDepthOnly, bufferSize: 8, size: ext(2, 1, 1)},
		{name: "depth of depth32float-stencil8, too small", layout: layout(0, 4, 0), format: gputypes.TextureFormatDepth32FloatStencil8,
			aspect: gputypes.TextureAspectDepthOnly, bufferSize: big, size: ext(2, 1, 1),
			want: TransferErrorInvalidBytesPerRow, wantErr: true},
		{name: "depth of depth24plus-stencil8 is not copyable", layout: layout(0, 8, 0), format: gputypes.TextureFormatDepth24PlusStencil8,
			aspect: gputypes.TextureAspectDepthOnly, bufferSize: big, size: ext(2, 1, 1),
			want: TransferErrorCopyAspectNotOne, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			aspect := tt.aspect
			if aspect == gputypes.TextureAspectUndefined {
				aspect = gputypes.TextureAspectAll
			}
			err := ValidateLinearTextureData(tt.layout, tt.format, aspect, tt.bufferSize, CopySideDestination, tt.size)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var te *TransferError
			if !errors.As(err, &te) || te.Kind != tt.want {
				t.Fatalf("error %v, want kind %d", err, tt.want)
			}
			if te.Error() == "" || te.Error() == "copy: unknown error" {
				t.Errorf("kind %d has no message", tt.want)
			}
		})
	}
}
