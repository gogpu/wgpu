//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestValidateTextureWrite pins Rust wgpu-core device/queue.rs
// write_texture_inner: usage, copy range, aspect, destination format, sample
// count, single aspect and the data layout, in that order, with no alignment
// requirement for the layout.
func TestValidateTextureWrite(t *testing.T) {
	t.Parallel()
	dst := CopySideDestination
	view := func(mut func(*TextureCopyView)) TextureCopyView {
		v := colorView(gputypes.TextureUsageCopyDst)
		v.Size = gputypes.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 8}
		if mut != nil {
			mut(&v)
		}
		return v
	}
	size := func(w, h, d uint32) gputypes.Extent3D {
		return gputypes.Extent3D{Width: w, Height: h, DepthOrArrayLayers: d}
	}
	tests := []struct {
		name     string
		view     TextureCopyView
		layout   TexelCopyBufferLayout
		dataSize uint64
		copy     gputypes.Extent3D
		want     TransferErrorKind
		side     CopySide
		wantErr  bool
	}{
		{name: "one row", view: view(nil), dataSize: 16, copy: size(4, 1, 1)},
		{name: "unaligned bytesPerRow and offset are accepted", view: view(nil),
			layout: TexelCopyBufferLayout{Offset: 3, BytesPerRow: 20, RowsPerImage: 4}, dataSize: 3 + 20*3 + 16, copy: size(4, 4, 1)},
		{name: "missing COPY_DST", view: view(func(v *TextureCopyView) { v.Usage = gputypes.TextureUsageCopySrc }),
			dataSize: 16, copy: size(4, 1, 1), want: TransferErrorMissingTextureUsage, side: dst, wantErr: true},
		{name: "mip level out of range", view: view(func(v *TextureCopyView) { v.MipLevel = 3 }),
			dataSize: 16, copy: size(1, 1, 1), want: TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "region past the texture", view: view(nil),
			dataSize: 1 << 20, copy: size(5, 1, 1), want: TransferErrorTextureOverrun, side: dst, wantErr: true},
		{name: "aspect missing in format", view: view(func(v *TextureCopyView) { v.Aspect = gputypes.TextureAspectDepthOnly }),
			dataSize: 16, copy: size(4, 1, 1), want: TransferErrorInvalidTextureAspect, wantErr: true},
		{name: "depth-only aspect of a combined format", view: view(func(v *TextureCopyView) {
			v.Format, v.Aspect = gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectDepthOnly
			v.Size = gputypes.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 8}
		}), dataSize: 1 << 10, copy: size(4, 4, 1), want: TransferErrorCopyToForbiddenTextureFormatAspect, wantErr: true},
		{name: "depth32 float destination", view: view(func(v *TextureCopyView) { v.Format = gputypes.TextureFormatDepth32Float }),
			dataSize: 1 << 10, copy: size(4, 4, 1), want: TransferErrorCopyToForbiddenTextureFormat, wantErr: true},
		{name: "stencil aspect of a combined format", view: view(func(v *TextureCopyView) {
			v.Format, v.Aspect = gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly
		}), layout: TexelCopyBufferLayout{BytesPerRow: 4, RowsPerImage: 4}, dataSize: 16, copy: size(4, 4, 1)},
		{name: "multisampled texture", view: view(func(v *TextureCopyView) { v.SampleCount = 4 }),
			dataSize: 16, copy: size(4, 4, 1), layout: TexelCopyBufferLayout{BytesPerRow: 16, RowsPerImage: 4},
			want: TransferErrorInvalidSampleCount, wantErr: true},
		{name: "combined format needs one aspect", view: view(func(v *TextureCopyView) { v.Format = gputypes.TextureFormatDepth24PlusStencil8 }),
			dataSize: 1 << 10, copy: size(4, 4, 1), want: TransferErrorCopyAspectNotOne, wantErr: true},
		{name: "data too short", view: view(nil),
			layout: TexelCopyBufferLayout{BytesPerRow: 16, RowsPerImage: 4}, dataSize: 15, copy: size(4, 1, 1),
			want: TransferErrorBufferEndOffsetOverrun, side: CopySideSource, wantErr: true},
		{name: "offset past the data", view: view(nil),
			layout: TexelCopyBufferLayout{Offset: 32}, dataSize: 16, copy: size(4, 1, 1),
			want: TransferErrorBufferStartOffsetOverrun, side: CopySideSource, wantErr: true},
		{name: "bytesPerRow missing for two rows", view: view(nil),
			dataSize: 64, copy: size(4, 2, 1), want: TransferErrorUnspecifiedBytesPerRow, wantErr: true},
		{name: "bytesPerRow shorter than a row", view: view(nil),
			layout: TexelCopyBufferLayout{BytesPerRow: 8, RowsPerImage: 4}, dataSize: 64, copy: size(4, 2, 1),
			want: TransferErrorInvalidBytesPerRow, wantErr: true},
		{name: "usage is checked before the range", view: view(func(v *TextureCopyView) { v.Usage = 0; v.MipLevel = 9 }),
			dataSize: 16, copy: size(4, 1, 1), want: TransferErrorMissingTextureUsage, side: dst, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTextureWrite(tt.view, tt.layout, tt.dataSize, tt.copy)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("ValidateTextureWrite = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTextureWrite = nil, want kind %d", tt.want)
			}
			checkTransferKind(t, err, tt.want, tt.side)
		})
	}
}
