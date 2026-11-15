//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// TextureCopyView describes one texture endpoint of a copy command recorded on
// a command encoder: the texture and the part of its image the copy touches.
type TextureCopyView struct {
	// Format, Usage, SampleCount, MipLevelCount, Dimension and Size are the
	// texture's. Size is the size of mip level 0.
	Format        gputypes.TextureFormat
	Usage         gputypes.TextureUsage
	SampleCount   uint32
	MipLevelCount uint32
	Dimension     gputypes.TextureDimension
	Size          gputypes.Extent3D
	// MipLevel, Origin and Aspect come from the copy's texel copy texture
	// info. TextureAspectUndefined means TextureAspectAll, as in WebGPU.
	MipLevel uint32
	Origin   gputypes.Origin3D
	Aspect   gputypes.TextureAspect
}

// formatAspects is a set of texture aspects (Rust hal::FormatAspects).
type formatAspects uint8

const (
	aspectColor formatAspects = 1 << iota
	aspectDepth
	aspectStencil
)

// isOne reports whether exactly one aspect is set (Rust FormatAspects::is_one).
func (a formatAspects) isOne() bool { return a != 0 && a&(a-1) == 0 }

// allAspects returns the aspects a format has.
func allAspects(format gputypes.TextureFormat) formatAspects {
	var aspects formatAspects
	if format.HasDepth() {
		aspects |= aspectDepth
	}
	if format.HasStencil() {
		aspects |= aspectStencil
	}
	if aspects == 0 {
		return aspectColor
	}
	return aspects
}

// copyAspects returns the aspects of format the copy aspect selects; empty
// when the format has no such aspect (Rust FormatAspects::new).
func copyAspects(format gputypes.TextureFormat, aspect gputypes.TextureAspect) formatAspects {
	all := allAspects(format)
	switch aspect {
	case gputypes.TextureAspectUndefined, gputypes.TextureAspectAll:
		return all
	case gputypes.TextureAspectDepthOnly:
		return all & aspectDepth
	case gputypes.TextureAspectStencilOnly:
		return all & aspectStencil
	default:
		return 0
	}
}

// validateTextureCopyView checks that the copy's aspect exists in the texture
// format (Rust extract_texture_selector). The mip level is checked by
// ValidateTextureCopyRange, which every texture copy runs first, as Rust does.
func validateTextureCopyView(v TextureCopyView) error {
	if copyAspects(v.Format, v.Aspect) == 0 {
		return &TransferError{Kind: TransferErrorInvalidTextureAspect, Format: v.Format, Aspect: v.Aspect}
	}
	return nil
}

// validateTextureCopyUsage checks that the texture was created with the usage
// the copy needs (Rust check_usage(COPY_SRC / COPY_DST)).
func validateTextureCopyUsage(side CopySide, v TextureCopyView) error {
	required := gputypes.TextureUsageCopySrc
	if side == CopySideDestination {
		required = gputypes.TextureUsageCopyDst
	}
	if !v.Usage.Contains(required) {
		return &TransferError{Kind: TransferErrorMissingTextureUsage, Side: side, TextureUsage: required}
	}
	return nil
}

// ValidateBufferTextureCopyTexture checks the usages and the texture side of a
// buffer-texture copy recorded on a command encoder, in the order of Rust
// wgpu-core command/transfer.rs copy_texture_to_buffer and
// copy_buffer_to_texture and the W3C validating texture buffer copy
// algorithm: the copy range is valid (ValidateTextureCopyRange), the aspect
// exists, the buffer has COPY_DST (side Source: texture to buffer) or
// COPY_SRC (side Destination: buffer to texture), the texture has COPY_SRC
// (side Source) or COPY_DST (side Destination), the format is copyable on
// that side, and the copy names a single aspect. side is the texture's side;
// bufferUsage is the buffer's usage; copySize is the copy's size. The sample
// count, offset and bytesPerRow rules are in ValidateTextureBufferCopy, the
// buffer layout in ValidateLinearTextureData.
func ValidateBufferTextureCopyTexture(side CopySide, v TextureCopyView, bufferUsage gputypes.BufferUsage, copySize gputypes.Extent3D) error {
	if err := ValidateTextureCopyRange(side, v, copySize); err != nil {
		return err
	}
	if err := validateTextureCopyView(v); err != nil {
		return err
	}
	bufferSide, bufferRequired := CopySideDestination, gputypes.BufferUsageCopyDst
	if side == CopySideDestination {
		bufferSide, bufferRequired = CopySideSource, gputypes.BufferUsageCopySrc
	}
	if !bufferUsage.Contains(bufferRequired) {
		return &TransferError{Kind: TransferErrorMissingBufferUsage, Side: bufferSide, Usage: bufferRequired}
	}
	if err := validateTextureCopyUsage(side, v); err != nil {
		return err
	}
	if side == CopySideSource {
		if err := validateTextureCopySrcFormat(v.Format, v.Aspect); err != nil {
			return err
		}
	} else if err := validateTextureCopyDstFormat(v.Format, v.Aspect); err != nil {
		return err
	}
	if !copyAspects(v.Format, v.Aspect).isOne() {
		return &TransferError{Kind: TransferErrorCopyAspectNotOne}
	}
	return nil
}

// validateTextureCopySrcFormat rejects formats and aspects that are not a
// valid texel copy source (Rust validate_texture_copy_src_format).
func validateTextureCopySrcFormat(format gputypes.TextureFormat, aspect gputypes.TextureAspect) error {
	switch {
	case format == gputypes.TextureFormatDepth24Plus:
		return &TransferError{Kind: TransferErrorCopyFromForbiddenTextureFormat, Format: format}
	case format == gputypes.TextureFormatDepth24PlusStencil8 && aspect == gputypes.TextureAspectDepthOnly:
		return &TransferError{Kind: TransferErrorCopyFromForbiddenTextureFormatAspect, Format: format, Aspect: aspect}
	default:
		return nil
	}
}

// validateTextureCopyDstFormat rejects formats and aspects that are not a
// valid texel copy destination (Rust validate_texture_copy_dst_format).
func validateTextureCopyDstFormat(format gputypes.TextureFormat, aspect gputypes.TextureAspect) error {
	switch {
	case format == gputypes.TextureFormatDepth24Plus || format == gputypes.TextureFormatDepth32Float:
		return &TransferError{Kind: TransferErrorCopyToForbiddenTextureFormat, Format: format}
	case (format == gputypes.TextureFormatDepth24PlusStencil8 || format == gputypes.TextureFormatDepth32FloatStencil8) &&
		aspect == gputypes.TextureAspectDepthOnly:
		return &TransferError{Kind: TransferErrorCopyToForbiddenTextureFormatAspect, Format: format, Aspect: aspect}
	default:
		return nil
	}
}

// ValidateTextureToTextureCopy checks a copyTextureToTexture command recorded
// on a command encoder, in the order of Rust wgpu-core command/transfer.rs
// copy_texture_to_texture: the formats are copy-compatible (equal except for
// the sRGB suffix; https://gpuweb.github.io/gpuweb/#copy-compatible), both
// copy ranges are valid (ValidateTextureCopyRange) and both aspects exist,
// copies inside one texture do not overlap, both sides name every aspect of
// their format, the sample counts match, the source has COPY_SRC and the
// destination COPY_DST. copySize is the size of the copy region;
// sameTexture reports that src and dst are one texture.
func ValidateTextureToTextureCopy(sameTexture bool, src, dst TextureCopyView, copySize gputypes.Extent3D) error {
	if src.Format.RemoveSrgbSuffix() != dst.Format.RemoveSrgbSuffix() {
		return &TransferError{Kind: TransferErrorTextureFormatsNotCopyCompatible, Format: src.Format, DstFormat: dst.Format}
	}
	if err := ValidateTextureCopyRange(CopySideSource, src, copySize); err != nil {
		return err
	}
	if err := ValidateTextureCopyRange(CopySideDestination, dst, copySize); err != nil {
		return err
	}
	if err := validateTextureCopyView(src); err != nil {
		return err
	}
	if err := validateTextureCopyView(dst); err != nil {
		return err
	}
	if sameTexture {
		if err := validateCopyWithinSameTexture(src, dst, copySize.DepthOrArrayLayers); err != nil {
			return err
		}
	}
	if copyAspects(src.Format, src.Aspect) != allAspects(src.Format) {
		return &TransferError{Kind: TransferErrorCopySrcMissingAspects}
	}
	if copyAspects(dst.Format, dst.Aspect) != allAspects(dst.Format) {
		return &TransferError{Kind: TransferErrorCopyDstMissingAspects}
	}
	if src.SampleCount != dst.SampleCount {
		return &TransferError{Kind: TransferErrorSampleCountNotEqual, SampleCount: src.SampleCount, DstSampleCount: dst.SampleCount}
	}
	if err := validateTextureCopyUsage(CopySideSource, src); err != nil {
		return err
	}
	return validateTextureCopyUsage(CopySideDestination, dst)
}

// validateCopyWithinSameTexture requires the subresources a copy reads and
// writes inside one texture to be disjoint (Rust
// validate_copy_within_same_texture; W3C "set of subresources for texture
// copy"). The origin z of a 3D texture is a depth, not a layer, so it spans
// one layer.
func validateCopyWithinSameTexture(src, dst TextureCopyView, depthOrArrayLayers uint32) error {
	if copyAspects(src.Format, src.Aspect)&copyAspects(dst.Format, dst.Aspect) == 0 {
		return nil
	}
	layers := depthOrArrayLayers
	if src.Dimension != gputypes.TextureDimension2D {
		layers = 1
	}
	// Written as differences, not sums, so a huge origin cannot wrap around.
	if src.Origin.Z >= dst.Origin.Z && src.Origin.Z-dst.Origin.Z >= layers {
		return nil
	}
	if dst.Origin.Z >= src.Origin.Z && dst.Origin.Z-src.Origin.Z >= layers {
		return nil
	}
	if src.MipLevel != dst.MipLevel {
		return nil
	}
	return &TransferError{
		Kind:    TransferErrorInvalidCopyWithinSameTexture,
		Aspect:  src.Aspect,
		OriginZ: src.Origin.Z, DstOriginZ: dst.Origin.Z, Layers: layers,
	}
}

// RequireDepthTextureAndBufferCopies checks that the device can copy between
// buffers and depth/stencil textures: such a copy needs the
// DEPTH_TEXTURE_AND_BUFFER_COPIES downlevel flag (Rust wgpu-core
// copy_buffer_to_texture and copy_texture_to_buffer). Other formats need
// nothing. The check is one device query, so a nil device accepts the copy.
func (d *Device) RequireDepthTextureAndBufferCopies(format gputypes.TextureFormat) error {
	if d == nil || !format.IsDepthStencil() {
		return nil
	}
	if err := d.RequireDownlevelFlags(gputypes.DownlevelFlagsDepthTextureAndBufferCopies); err != nil {
		return &TransferError{Kind: TransferErrorMissingDownlevelFlags, Format: format, Cause: err}
	}
	return nil
}

// StencilUploadNeedsBlit reports whether a write of the stencil aspect of
// format from a buffer or from queue data (the destination side of
// CopyBufferToTexture, and WriteTexture) must be lowered by the frontend to
// the stencil blit of internal/stencilblit instead of being passed to the
// backend. It holds on a backend that sets hal.Capabilities.StencilCopyNeedsBlit
// (GLES) for the combined depth-stencil formats: GL uploads their depth and
// stencil only together. Stencil8, depth aspects and other backends copy
// directly; a nil device copies directly.
//
// Dawn lowers the same copies (UseBlitForBufferToStencilTextureCopy,
// src/dawn/native/BlitBufferToDepthStencil.cpp). Rust wgpu-core refuses every
// depth/stencil buffer copy on GL instead: its GL backend never reports
// DEPTH_TEXTURE_AND_BUFFER_COPIES. A frontend that cannot set the blit up
// returns TransferErrorStencilCopyNeedsBlit.
func (d *Device) StencilUploadNeedsBlit(format gputypes.TextureFormat, aspect gputypes.TextureAspect) bool {
	if d == nil || d.adapter == nil || !d.adapter.StencilCopyNeedsBlit {
		return false
	}
	return aspect == gputypes.TextureAspectStencilOnly && format.HasDepth() && format.HasStencil()
}
