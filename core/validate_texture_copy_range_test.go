//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

func ext(w, h, d uint32) gputypes.Extent3D {
	return gputypes.Extent3D{Width: w, Height: h, DepthOrArrayLayers: d}
}

// rangeView is a texture of the given format and size with 4 mip levels.
func rangeView(format gputypes.TextureFormat, size gputypes.Extent3D) TextureCopyView {
	return TextureCopyView{
		Format:        format,
		Usage:         gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst,
		SampleCount:   1,
		MipLevelCount: 4,
		Dimension:     gputypes.TextureDimension2D,
		Size:          size,
	}
}

// TestMipLevelSizeAndPhysicalSize pins the mip chain and block rounding with
// the examples of the wgpu-types TextureDescriptor::mip_level_size doc comment
// (a 100x60x1 texture: 50x30, 25x15, 12x7, 6x3, 3x1, 1x1) and the W3C physical
// miplevel-specific texture extent.
func TestMipLevelSizeAndPhysicalSize(t *testing.T) {
	t.Parallel()
	base := ext(100, 60, 1)
	for level, want := range []gputypes.Extent3D{
		ext(100, 60, 1), ext(50, 30, 1), ext(25, 15, 1), ext(12, 7, 1), ext(6, 3, 1), ext(3, 1, 1), ext(1, 1, 1),
	} {
		if got := mipLevelSize(base, uint32(level), gputypes.TextureDimension2D); got != want {
			t.Errorf("2D level %d: %v, want %v", level, got, want)
		}
	}
	// A shift of 32 or more does not wrap around to the full size.
	if got := mipLevelSize(base, 40, gputypes.TextureDimension2D); got != ext(1, 1, 1) {
		t.Errorf("2D level 40: %v, want 1x1x1", got)
	}
	// 2D array textures keep their layers, 3D textures halve their depth, 1D has no height.
	if got := mipLevelSize(ext(8, 8, 6), 2, gputypes.TextureDimension2D); got != ext(2, 2, 6) {
		t.Errorf("2D array: %v", got)
	}
	if got := mipLevelSize(ext(8, 8, 8), 2, gputypes.TextureDimension3D); got != ext(2, 2, 2) {
		t.Errorf("3D: %v", got)
	}
	if got := mipLevelSize(ext(8, 8, 8), 1, gputypes.TextureDimension1D); got != ext(4, 1, 1) {
		t.Errorf("1D: %v", got)
	}
	// Physical size: widths and heights round up to whole blocks.
	if got := physicalSize(ext(6, 3, 1), gputypes.TextureFormatBC1RGBAUnorm); got != ext(8, 4, 1) {
		t.Errorf("BC1 6x3: %v, want 8x4x1", got)
	}
	if got := physicalSize(ext(1, 1, 1), gputypes.TextureFormatASTC12x10Unorm); got != ext(12, 10, 1) {
		t.Errorf("ASTC12x10 1x1: %v, want 12x10x1", got)
	}
	if got := physicalSize(ext(6, 3, 1), gputypes.TextureFormatRGBA8Unorm); got != ext(6, 3, 1) {
		t.Errorf("RGBA8 6x3: %v, want unchanged", got)
	}
}

// TestValidateTextureCopyRange pins Rust wgpu-core validate_texture_copy_range
// (wgpu-core/src/command/transfer.rs): the mip level exists, the region fits
// the physical mip extent, depth/stencil and multisampled textures are copied
// whole in x and y, and origin and size follow the block grid.
func TestValidateTextureCopyRange(t *testing.T) {
	t.Parallel()
	color := rangeView(gputypes.TextureFormatRGBA8Unorm, ext(16, 8, 4))
	withOrigin := func(v TextureCopyView, x, y, z uint32) TextureCopyView {
		v.Origin = gputypes.Origin3D{X: x, Y: y, Z: z}
		return v
	}
	withMip := func(v TextureCopyView, mip uint32) TextureCopyView {
		v.MipLevel = mip
		return v
	}
	multisampled := color
	multisampled.SampleCount = 4
	depth := rangeView(gputypes.TextureFormatDepth32Float, ext(16, 8, 4))
	bc := rangeView(gputypes.TextureFormatBC1RGBAUnorm, ext(16, 8, 1))
	astc := rangeView(gputypes.TextureFormatASTC10x5Unorm, ext(30, 15, 1))

	tests := []struct {
		name    string
		view    TextureCopyView
		size    gputypes.Extent3D
		want    TransferErrorKind
		wantErr bool
		dim     TextureErrorDimension
	}{
		{name: "whole mip 0", view: color, size: ext(16, 8, 4)},
		{name: "inner region", view: withOrigin(color, 3, 2, 1), size: ext(10, 5, 2)},
		{name: "empty region", view: color, size: ext(0, 0, 0)},
		{name: "region at the far corner", view: withOrigin(color, 8, 4, 3), size: ext(8, 4, 1)},
		{name: "mip level out of range", view: withMip(color, 4), size: ext(1, 1, 1),
			want: TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "width overruns", view: withOrigin(color, 9, 0, 0), size: ext(8, 1, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
		{name: "height overruns", view: withOrigin(color, 0, 5, 0), size: ext(1, 4, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionY},
		{name: "layers overrun", view: withOrigin(color, 0, 0, 3), size: ext(1, 1, 2),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionZ},
		{name: "mip 1 is half the size", view: withMip(color, 1), size: ext(9, 1, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
		{name: "whole mip 1", view: withMip(color, 1), size: ext(8, 4, 4)},
		{name: "mip 3 of 16x8 is 2x1", view: withMip(color, 3), size: ext(2, 1, 1)},
		{name: "origin past the end", view: withOrigin(color, 17, 0, 0), size: ext(0, 1, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
		{name: "huge origin does not wrap", view: withOrigin(color, ^uint32(0), 0, 0), size: ext(2, 1, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
		{name: "huge size does not wrap", view: withOrigin(color, 2, 0, 0), size: ext(^uint32(0)-1, 1, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
		{name: "depth whole", view: depth, size: ext(16, 8, 1)},
		{name: "depth a subset of layers", view: withOrigin(depth, 0, 0, 2), size: ext(16, 8, 2)},
		{name: "depth partial width", view: depth, size: ext(8, 8, 1),
			want: TransferErrorUnsupportedPartialTransfer, wantErr: true, dim: TextureErrorDimensionX},
		{name: "depth partial height", view: depth, size: ext(16, 4, 1),
			want: TransferErrorUnsupportedPartialTransfer, wantErr: true, dim: TextureErrorDimensionY},
		{name: "depth offset origin", view: withOrigin(depth, 0, 2, 0), size: ext(16, 6, 1),
			want: TransferErrorUnsupportedPartialTransfer, wantErr: true, dim: TextureErrorDimensionY},
		{name: "multisampled whole", view: multisampled, size: ext(16, 8, 4)},
		{name: "multisampled partial", view: multisampled, size: ext(4, 8, 4),
			want: TransferErrorUnsupportedPartialTransfer, wantErr: true, dim: TextureErrorDimensionX},
		{name: "bc whole", view: bc, size: ext(16, 8, 1)},
		{name: "bc block aligned region", view: withOrigin(bc, 4, 4, 0), size: ext(8, 4, 1)},
		{name: "bc origin x unaligned", view: withOrigin(bc, 2, 0, 0), size: ext(4, 4, 1),
			want: TransferErrorUnalignedCopyOriginX, wantErr: true},
		{name: "bc origin y unaligned", view: withOrigin(bc, 0, 1, 0), size: ext(4, 4, 1),
			want: TransferErrorUnalignedCopyOriginY, wantErr: true},
		{name: "bc width unaligned", view: bc, size: ext(6, 4, 1),
			want: TransferErrorUnalignedCopyWidth, wantErr: true},
		{name: "bc height unaligned", view: bc, size: ext(8, 2, 1),
			want: TransferErrorUnalignedCopyHeight, wantErr: true},
		// Mip 2 of a 16x8 BC texture is 4x2 texels: the physical size is 4x4, so
		// the whole block is copyable although the logical mip is smaller.
		{name: "bc physical size of a small mip", view: withMip(bc, 2), size: ext(4, 4, 1)},
		{name: "bc past the physical size", view: withMip(bc, 2), size: ext(8, 4, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
		{name: "bc smallest mip is one block", view: withMip(bc, 3), size: ext(4, 4, 1)},
		{name: "astc whole", view: astc, size: ext(30, 15, 1)},
		{name: "astc block aligned", view: withOrigin(astc, 10, 5, 0), size: ext(20, 10, 1)},
		{name: "astc width unaligned", view: astc, size: ext(25, 5, 1),
			want: TransferErrorUnalignedCopyWidth, wantErr: true},
		{name: "astc height unaligned", view: astc, size: ext(10, 6, 1),
			want: TransferErrorUnalignedCopyHeight, wantErr: true},
		// Rust order: the overrun (and partial copy) checks run before the block alignment.
		{name: "overrun before alignment", view: withOrigin(bc, 2, 0, 0), size: ext(16, 4, 1),
			want: TransferErrorTextureOverrun, wantErr: true, dim: TextureErrorDimensionX},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTextureCopyRange(CopySideDestination, tt.view, tt.size)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			// Only the overrun and partial-copy kinds name the side.
			wantSide := CopySideSource
			sided := tt.want == TransferErrorTextureOverrun || tt.want == TransferErrorUnsupportedPartialTransfer
			if sided {
				wantSide = CopySideDestination
			}
			checkTransferKind(t, err, tt.want, wantSide)
			var te *TransferError
			if errors.As(err, &te) && sided && te.Dimension != tt.dim {
				t.Errorf("dimension = %v, want %v", te.Dimension, tt.dim)
			}
		})
	}
}

// TestTransferErrorRangeText: every copy range, layout and compatibility kind
// has its own message.
func TestTransferErrorRangeText(t *testing.T) {
	t.Parallel()
	seen := map[string]TransferErrorKind{}
	for _, kind := range []TransferErrorKind{
		TransferErrorTextureOverrun,
		TransferErrorUnsupportedPartialTransfer,
		TransferErrorUnalignedCopyOriginX,
		TransferErrorUnalignedCopyOriginY,
		TransferErrorUnalignedCopyWidth,
		TransferErrorUnalignedCopyHeight,
		TransferErrorUnspecifiedBytesPerRow,
		TransferErrorUnspecifiedRowsPerImage,
		TransferErrorInvalidBytesPerRow,
		TransferErrorInvalidRowsPerImage,
		TransferErrorSizeOverflow,
		TransferErrorTextureFormatsNotCopyCompatible,
		TransferErrorMissingDownlevelFlags,
		TransferErrorStencilCopyNeedsBlit,
	} {
		text := (&TransferError{Kind: kind}).Error()
		if text == "copy: unknown error" {
			t.Errorf("kind %d has no message", kind)
		}
		if other, dup := seen[text]; dup {
			t.Errorf("kinds %d and %d share the message %q", other, kind, text)
		}
		seen[text] = kind
	}
}

// TestValidateCopyWithinSameTextureHugeOrigin: the overlap test is written as
// differences, so an origin near the top of uint32 cannot wrap into a false
// overlap.
func TestValidateCopyWithinSameTextureHugeOrigin(t *testing.T) {
	t.Parallel()
	src := rangeView(gputypes.TextureFormatRGBA8Unorm, ext(1, 1, 8))
	dst := src
	src.Origin.Z = ^uint32(0)
	if err := validateCopyWithinSameTexture(src, dst, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
