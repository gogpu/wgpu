//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// mipLevelSize returns the logical size of a mip level of a texture
// (https://gpuweb.github.io/gpuweb/#logical-miplevel-specific-texture-extent;
// Rust wgpu-types Extent3d::mip_level_size). It does not round up to the
// block size of the format. A shift of 32 or more yields 0, which max(1, _)
// turns into 1, as Rust's unbounded_shr does.
func mipLevelSize(size gputypes.Extent3D, level uint32, dim gputypes.TextureDimension) gputypes.Extent3D {
	out := gputypes.Extent3D{
		Width:              max(1, size.Width>>level),
		Height:             1,
		DepthOrArrayLayers: 1,
	}
	if dim != gputypes.TextureDimension1D {
		out.Height = max(1, size.Height>>level)
	}
	switch dim {
	case gputypes.TextureDimension2D:
		out.DepthOrArrayLayers = size.DepthOrArrayLayers
	case gputypes.TextureDimension3D:
		out.DepthOrArrayLayers = max(1, size.DepthOrArrayLayers>>level)
	}
	return out
}

// physicalSize rounds the width and height of a mip level up to whole texel
// blocks (https://gpuweb.github.io/gpuweb/#physical-miplevel-specific-texture-extent;
// Rust wgpu-types Extent3d::physical_size). Only block-compressed formats
// change.
func physicalSize(size gputypes.Extent3D, format gputypes.TextureFormat) gputypes.Extent3D {
	bw, bh := format.BlockDimensions()
	size.Width = (size.Width + bw - 1) / bw * bw
	size.Height = (size.Height + bh - 1) / bh * bh
	return size
}

// ValidateTextureCopyRange checks the extent and alignment of a texture copy
// region, as Rust wgpu-core command/transfer.rs validate_texture_copy_range
// (W3C "validating GPUTexelCopyTextureInfo" and "validating texture copy
// range"): the mip level exists, the region lies inside the physical size of
// the mip level, depth/stencil and multisampled textures are copied whole in
// x and y (partial copies of layers stay valid), and origin and size are
// multiples of the format's block size. copySize is the region size. The
// overrun checks compare against the remaining texels, so a huge origin
// cannot wrap around.
func ValidateTextureCopyRange(side CopySide, v TextureCopyView, copySize gputypes.Extent3D) error {
	if v.MipLevel >= v.MipLevelCount {
		return &TransferError{Kind: TransferErrorInvalidTextureMipLevel, MipLevel: v.MipLevel, MipLevelCount: v.MipLevelCount}
	}
	extent := physicalSize(mipLevelSize(v.Size, v.MipLevel, v.Dimension), v.Format)
	// Multisampled and depth/stencil formats do not support partial copies in
	// x and y, but do support copying a subset of layers.
	exact := v.Format.IsDepthStencil() || v.SampleCount > 1

	dims := [...]struct {
		dim                      TextureErrorDimension
		start, size, textureSize uint32
		exact                    bool
	}{
		{TextureErrorDimensionX, v.Origin.X, copySize.Width, extent.Width, exact},
		{TextureErrorDimensionY, v.Origin.Y, copySize.Height, extent.Height, exact},
		{TextureErrorDimensionZ, v.Origin.Z, copySize.DepthOrArrayLayers, extent.DepthOrArrayLayers, false},
	}
	for _, d := range dims {
		if err := checkCopyDimension(side, v, d.dim, d.start, d.size, d.textureSize, d.exact); err != nil {
			return err
		}
	}
	return validateCopyBlockAlignment(v, copySize)
}

// checkCopyDimension checks that a run of size texels starting at start fits
// a dimension of textureSize texels (Rust check_dimension).
func checkCopyDimension(side CopySide, v TextureCopyView, dim TextureErrorDimension, start, size, textureSize uint32, exact bool) error {
	end := start + size // wraps like Rust's wrapping_add; only reported
	if exact && (start != 0 || size != textureSize) {
		return &TransferError{
			Kind: TransferErrorUnsupportedPartialTransfer, Side: side, Format: v.Format, SampleCount: v.SampleCount,
			Dimension: dim, StartOffset: start, EndOffset: end, TextureSize: textureSize,
		}
	}
	if start > textureSize || textureSize-start < size {
		return &TransferError{
			Kind: TransferErrorTextureOverrun, Side: side,
			Dimension: dim, StartOffset: start, EndOffset: end, TextureSize: textureSize,
		}
	}
	return nil
}

// validateCopyBlockAlignment requires origin and size of a copy to be
// multiples of the texel block of the format.
func validateCopyBlockAlignment(v TextureCopyView, copySize gputypes.Extent3D) error {
	bw, bh := v.Format.BlockDimensions()
	switch {
	case v.Origin.X%bw != 0:
		return &TransferError{Kind: TransferErrorUnalignedCopyOriginX}
	case v.Origin.Y%bh != 0:
		return &TransferError{Kind: TransferErrorUnalignedCopyOriginY}
	case copySize.Width%bw != 0:
		return &TransferError{Kind: TransferErrorUnalignedCopyWidth}
	case copySize.Height%bh != 0:
		return &TransferError{Kind: TransferErrorUnalignedCopyHeight}
	}
	return nil
}
