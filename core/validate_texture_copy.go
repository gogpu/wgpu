//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// TextureCopyView describes one texture endpoint of a copy command recorded on
// a command encoder: the texture and the part of its image the copy touches.
type TextureCopyView struct {
	// Format, Usage, SampleCount, MipLevelCount and Dimension are the texture's.
	Format        gputypes.TextureFormat
	Usage         gputypes.TextureUsage
	SampleCount   uint32
	MipLevelCount uint32
	Dimension     gputypes.TextureDimension
	// MipLevel, OriginZ and Aspect come from the copy's texel copy texture
	// info. TextureAspectUndefined means TextureAspectAll, as in WebGPU.
	MipLevel uint32
	OriginZ  uint32
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

// validateTextureCopyView checks the endpoint-local rules shared by every
// texture copy: the mip level exists and the aspect exists in the format
// (Rust validate_texture_copy_range / extract_texture_selector).
func validateTextureCopyView(v TextureCopyView) error {
	if v.MipLevel >= v.MipLevelCount {
		return &TransferError{Kind: TransferErrorInvalidTextureMipLevel, MipLevel: v.MipLevel, MipLevelCount: v.MipLevelCount}
	}
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
// algorithm: mip level and aspect exist, the buffer has COPY_DST (side
// Source: texture to buffer) or COPY_SRC (side Destination: buffer to
// texture), the texture has COPY_SRC (side Source) or COPY_DST (side
// Destination), the format is copyable on that side, and the copy names a
// single aspect. side is the texture's side; bufferUsage is the buffer's
// usage. The sample count, offset and bytesPerRow rules are in
// ValidateTextureBufferCopy.
func ValidateBufferTextureCopyTexture(side CopySide, v TextureCopyView, bufferUsage gputypes.BufferUsage) error {
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
// copy_texture_to_texture: both mip levels exist and both aspects exist,
// copies inside one texture do not overlap, both sides name every aspect of
// their format, the sample counts match, the source has COPY_SRC and the
// destination COPY_DST. depthOrArrayLayers is the copy size's third
// dimension; sameTexture reports that src and dst are one texture. Format
// copy compatibility and the copy range are not checked here.
func ValidateTextureToTextureCopy(sameTexture bool, src, dst TextureCopyView, depthOrArrayLayers uint32) error {
	if err := validateTextureCopyView(src); err != nil {
		return err
	}
	if err := validateTextureCopyView(dst); err != nil {
		return err
	}
	if sameTexture {
		if err := validateCopyWithinSameTexture(src, dst, depthOrArrayLayers); err != nil {
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
	if src.OriginZ >= dst.OriginZ && src.OriginZ-dst.OriginZ >= layers {
		return nil
	}
	if dst.OriginZ >= src.OriginZ && dst.OriginZ-src.OriginZ >= layers {
		return nil
	}
	if src.MipLevel != dst.MipLevel {
		return nil
	}
	return &TransferError{
		Kind:    TransferErrorInvalidCopyWithinSameTexture,
		Aspect:  src.Aspect,
		OriginZ: src.OriginZ, DstOriginZ: dst.OriginZ, Layers: layers,
	}
}
