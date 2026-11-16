//go:build !(js && wasm)

package track

// TextureSelector selects the subresources of one texture a usage covers:
// MipLevelCount mip levels from BaseMipLevel and ArrayLayerCount array layers
// from BaseArrayLayer, of a texture of TextureMipLevelCount mip levels and
// TextureArrayLayerCount array layers. A count of 0 selects every level or
// layer from the base on, as in hal.TextureRange.
//
// A selector whose texture counts are not known (0, as in the zero value)
// selects every subresource of the texture.
//
// Reference: wgpu-core track/texture.rs TextureSelector
type TextureSelector struct {
	TextureMipLevelCount   uint32
	TextureArrayLayerCount uint32
	BaseMipLevel           uint32
	MipLevelCount          uint32
	BaseArrayLayer         uint32
	ArrayLayerCount        uint32
}

// SelectsAll returns true if the selector covers every subresource of its
// texture.
func (s TextureSelector) SelectsAll() bool {
	mipStart, mipEnd, layerStart, layerEnd := s.bounds(s.TextureMipLevelCount, s.TextureArrayLayerCount)
	return mipStart == 0 && mipEnd == s.TextureMipLevelCount &&
		layerStart == 0 && layerEnd == s.TextureArrayLayerCount
}

// bounds returns the selected mip levels [mipStart, mipEnd) and array layers
// [layerStart, layerEnd) of a texture of mipLevelCount levels and
// arrayLayerCount layers. A selector whose texture counts are not known
// covers them all.
func (s TextureSelector) bounds(mipLevelCount, arrayLayerCount uint32) (mipStart, mipEnd, layerStart, layerEnd uint32) {
	if s.TextureMipLevelCount == 0 || s.TextureArrayLayerCount == 0 {
		return 0, mipLevelCount, 0, arrayLayerCount
	}
	return min(s.BaseMipLevel, mipLevelCount), selectedEnd(s.BaseMipLevel, s.MipLevelCount, mipLevelCount),
		min(s.BaseArrayLayer, arrayLayerCount), selectedEnd(s.BaseArrayLayer, s.ArrayLayerCount, arrayLayerCount)
}

// selectedEnd returns the end of count items from base among total, every
// item from base on when count is 0, clamped to total.
func selectedEnd(base, count, total uint32) uint32 {
	if count == 0 || base+count > total {
		return total
	}
	return base + count
}
