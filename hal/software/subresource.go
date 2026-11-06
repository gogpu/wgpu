//go:build !(js && wasm)

package software

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Texture storage layout (P1-SW-026).
//
// A texture keeps every subresource in its one data slice, mip-major: all of
// mip level 0, then all of level 1, and so on. A level holds its 2D images
// back to back: the array layers of a 1D or 2D texture, or the depth slices
// of a 3D texture, whose depth halves per level like its width and height.
// An image is its rows of texel blocks, rowPitch bytes each, with no padding.
//
// Mip-major keeps level 0 exactly where the backend stored it before mip
// chains were stored (width x height x layers images from offset 0), so the
// level-0 readers, the surface texture and data written at offset 0 are
// unchanged. It also makes a level of a 2D array and of a 3D texture the same
// shape, one contiguous run of images. D3D12 numbers subresources layer-major
// (mip + layer * mipLevels); that is an index scheme, not a memory layout, and
// nothing here depends on it. Everything addresses storage through
// subresourceOffset and image.
//
// Block-compressed formats are laid out per texel block: a row is
// ceil(width / blockWidth) blocks of BlockCopySize bytes, an image
// ceil(height / blockHeight) rows. That is the layout of the WebGPU copy
// operations (bytesPerRow counts block rows), so uploads and readbacks round
// trip byte for byte. The adapter exposes no texture-compression feature, so
// core rejects such formats; the interpreter does not decode them.

// mipLevels returns the number of stored mip levels, at least 1.
func (t *Texture) mipLevels() uint32 { return max(t.mipLevelCount, 1) }

// is3D reports whether the texture is three-dimensional: its depth is depth
// slices, which shrink per mip level, rather than array layers.
func (t *Texture) is3D() bool { return t.dimension == gputypes.TextureDimension3D }

// arrayLayers returns the array layer count: 1 for a 3D texture.
func (t *Texture) arrayLayers() uint32 {
	if t.is3D() {
		return 1
	}
	return max(t.depth, 1)
}

// levelExtent returns the size of mip level level in texels: width and
// height halved per level, at least 1, and the number of images in the
// level: the depth slices of a 3D texture (halved too), otherwise the array
// layers.
func (t *Texture) levelExtent(level uint32) (w, h, images uint32) {
	w, h, images = mipExtent(t.width, level), mipExtent(t.height, level), max(t.depth, 1)
	if t.is3D() {
		images = mipExtent(t.depth, level)
	}
	return w, h, images
}

// mipExtent is the extent of mip level level of a dimension whose level-0
// extent is base: base >> level, at least 1.
func mipExtent(base, level uint32) uint32 {
	if level >= 32 {
		return 1
	}
	return max(base>>level, 1)
}

// blockSize returns the texel block of the texture's format: its width and
// height in texels (1 x 1 for uncompressed formats) and its size in bytes.
func (t *Texture) blockSize() (bw, bh uint32, bytes uint64) {
	bw, bh = t.format.BlockDimensions()
	return max(bw, 1), max(bh, 1), formatBytesPerPixel(t.format)
}

// levelPitch returns the bytes of one row of texel blocks of mip level
// level, and the bytes of one of its images.
func (t *Texture) levelPitch(level uint32) (rowPitch, imageSize uint64) {
	w, h, _ := t.levelExtent(level)
	bw, bh, bytes := t.blockSize()
	rowPitch = uint64((w+bw-1)/bw) * bytes
	return rowPitch, rowPitch * uint64((h+bh-1)/bh)
}

// levelOffset returns the byte offset of mip level level in the texture's
// storage; levelOffset(mipLevels()) is the storage size.
func (t *Texture) levelOffset(level uint32) uint64 {
	var off uint64
	for l := range min(level, t.mipLevels()) {
		_, imageSize := t.levelPitch(l)
		_, _, images := t.levelExtent(l)
		off += imageSize * uint64(images)
	}
	return off
}

// storageSize returns the bytes the texture's storage needs.
func (t *Texture) storageSize() uint64 { return t.levelOffset(t.mipLevels()) }

// subresourceOffset returns the byte offset of image z of mip level level:
// array layer z of a 1D or 2D texture, depth slice z of a 3D texture.
func (t *Texture) subresourceOffset(level, z uint32) uint64 {
	_, imageSize := t.levelPitch(level)
	return t.levelOffset(level) + uint64(z)*imageSize
}

// textureImage is one 2D image of a texture's storage: an array layer or
// depth slice of one mip level.
type textureImage struct {
	// data is exactly the image's bytes and aliases the texture's storage.
	data          []byte
	width, height uint32
	rowPitch      uint64
}

// image returns image z of mip level level, or false when the texture has
// no such image or its storage is too short (a texture built without
// CreateTexture). The caller holds the texture lock while it uses data.
func (t *Texture) image(level, z uint32) (textureImage, bool) {
	w, h, images := t.levelExtent(level)
	if level >= t.mipLevels() || z >= images {
		return textureImage{}, false
	}
	rowPitch, imageSize := t.levelPitch(level)
	off := t.subresourceOffset(level, z)
	if off+imageSize > uint64(len(t.data)) {
		return textureImage{}, false
	}
	return textureImage{data: t.data[off : off+imageSize : off+imageSize], width: w, height: h, rowPitch: rowPitch}, true
}

// baseImage returns the view's first image: its base mip level and base
// array layer, or depth slice 0 of a 3D view. The caller holds the texture
// lock while it uses data.
func (v *TextureView) baseImage() (textureImage, bool) {
	if v.texture == nil {
		return textureImage{}, false
	}
	z := v.baseArrayLayer
	if v.texture.is3D() {
		z = 0
	}
	return v.texture.image(v.baseMipLevel, z)
}

// levelData returns the storage of images [z, z+n) of mip level level, or
// nil when they are out of range. The caller holds the texture lock while it
// uses the slice.
func (t *Texture) levelData(level, z, n uint32) []byte {
	_, _, images := t.levelExtent(level)
	if level >= t.mipLevels() || z+n > images || n == 0 {
		return nil
	}
	_, imageSize := t.levelPitch(level)
	off := t.subresourceOffset(level, z)
	end := off + uint64(n)*imageSize
	if end > uint64(len(t.data)) {
		return nil
	}
	return t.data[off:end:end]
}

// isSingleSubresource reports whether the texture's storage is one 2D image:
// one mip level and one layer or slice. A render pass then writes the
// texture directly; otherwise it writes a pass-local copy of the view's
// image (subresourceTarget).
func (t *Texture) isSingleSubresource() bool {
	return t.mipLevels() == 1 && max(t.depth, 1) == 1
}

// copyRegion is a texture copy region in blocks of one mip level.
type copyRegion struct {
	level    uint32
	origin   hal.Origin3D
	rowBytes uint64 // bytes of one row of blocks of the region
	rows     uint32 // rows of blocks per image
	images   uint32 // layers or depth slices
}

// regionOf returns the region of a copy of size at origin in mip level
// level, or false when it does not lie inside the level. Origins and sizes
// of block-compressed formats are multiples of the block (core validates
// that); the level's extent is rounded up to whole blocks, its physical
// size in WebGPU terms.
func (t *Texture) regionOf(level uint32, origin hal.Origin3D, size hal.Extent3D) (copyRegion, bool) {
	if level >= t.mipLevels() {
		return copyRegion{}, false
	}
	w, h, images := t.levelExtent(level)
	bw, bh, bytes := t.blockSize()
	physW, physH := (w+bw-1)/bw*bw, (h+bh-1)/bh*bh
	n := max(size.DepthOrArrayLayers, 1)
	if uint64(origin.X)+uint64(size.Width) > uint64(physW) || uint64(origin.Y)+uint64(size.Height) > uint64(physH) ||
		uint64(origin.Z)+uint64(n) > uint64(images) {
		return copyRegion{}, false
	}
	return copyRegion{
		level:    level,
		origin:   origin,
		rowBytes: uint64((size.Width+bw-1)/bw) * bytes,
		rows:     (size.Height + bh - 1) / bh,
		images:   n,
	}, true
}

// rowOffset returns the byte offset in the texture's storage of block row
// row of image z (relative to the region's origin) of region r.
func (t *Texture) rowOffset(r copyRegion, z, row uint32) uint64 {
	rowPitch, _ := t.levelPitch(r.level)
	bw, bh, bytes := t.blockSize()
	return t.subresourceOffset(r.level, r.origin.Z+z) + uint64(r.origin.Y/bh+row)*rowPitch + uint64(r.origin.X/bw)*bytes
}

// bufferRowOffset returns the byte offset of block row row of image z of
// region r in linear memory laid out by layout: rows BytesPerRow apart
// (tight when 0) and images RowsPerImage texel rows apart (the region's
// height when 0).
func (t *Texture) bufferRowOffset(r copyRegion, layout hal.ImageDataLayout, z, row uint32) uint64 {
	stride := uint64(layout.BytesPerRow)
	if stride == 0 {
		stride = r.rowBytes
	}
	rowsPerImage := uint64(r.rows)
	if layout.RowsPerImage != 0 {
		_, bh, _ := t.blockSize()
		rowsPerImage = uint64((layout.RowsPerImage + bh - 1) / bh)
	}
	return layout.Offset + (uint64(z)*rowsPerImage+uint64(row))*stride
}

// copyLinear copies region r between the texture's storage and linear
// memory mem laid out by layout: into the texture when toTexture is set,
// out of it otherwise. A row that lies past the end of mem is skipped. The
// caller holds the texture lock (a write lock when toTexture).
func (t *Texture) copyLinear(r copyRegion, mem []byte, layout hal.ImageDataLayout, toTexture bool) {
	for z := range r.images {
		for row := range r.rows {
			memStart := t.bufferRowOffset(r, layout, z, row)
			texStart := t.rowOffset(r, z, row)
			if memStart+r.rowBytes > uint64(len(mem)) || texStart+r.rowBytes > uint64(len(t.data)) {
				continue
			}
			if toTexture {
				copy(t.data[texStart:texStart+r.rowBytes], mem[memStart:memStart+r.rowBytes])
			} else {
				copy(mem[memStart:memStart+r.rowBytes], t.data[texStart:texStart+r.rowBytes])
			}
		}
	}
}

// logRegionOutside logs a transfer whose region lies outside the texture's
// mip level; core validation rejects those, so it is a validation gap.
func logRegionOutside(op string, t *Texture, level uint32, origin hal.Origin3D, size hal.Extent3D) {
	w, h, images := t.levelExtent(level)
	hal.Logger().Error("software: texture transfer outside the mip level skipped",
		"op", op, "mipLevel", level, "mipLevels", t.mipLevels(),
		"origin", origin, "size", size, "levelWidth", w, "levelHeight", h, "levelImages", images)
}
