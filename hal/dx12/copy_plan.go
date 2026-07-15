// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

const d3d12TexturePlacementAlignment = 512

// bufferTextureCopyPlan describes one native D3D12 copy. WebGPU treats a
// 2D-array layer as a separate subresource, while D3D12 treats a 3D slice as
// a depth-one copy into one subresource. The buffer origin fields select the
// logical byte range inside an aligned, possibly enlarged footprint.
type bufferTextureCopyPlan struct {
	subresource     uint32
	bufferOffset    uint64
	bufferOriginX   uint32
	bufferOriginY   uint32
	footprintWidth  uint32
	footprintHeight uint32
	footprintDepth  uint32
	textureOriginX  uint32
	textureOriginY  uint32
	textureOriginZ  uint32
	rowPitch        uint32
	copyWidth       uint32
	copyHeight      uint32
}

type textureTextureCopyPlan struct {
	srcSubresource uint32
	dstSubresource uint32
	srcFront       uint32
	srcBack        uint32
	dstZ           uint32
}

type textureBlockInfo struct {
	width  uint32
	height uint32
	bytes  uint32
}

func textureBlockInfoForFormat(format gputypes.TextureFormat) (textureBlockInfo, bool) {
	bytes := format.BlockCopySize()
	if bytes == 0 {
		return textureBlockInfo{}, false
	}
	if textureFormatIsBC(format) {
		return textureBlockInfo{width: 4, height: 4, bytes: bytes}, true
	}
	return textureBlockInfo{width: 1, height: 1, bytes: bytes}, true
}

func textureFormatIsBC(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatBC1RGBAUnorm,
		gputypes.TextureFormatBC1RGBAUnormSrgb,
		gputypes.TextureFormatBC2RGBAUnorm,
		gputypes.TextureFormatBC2RGBAUnormSrgb,
		gputypes.TextureFormatBC3RGBAUnorm,
		gputypes.TextureFormatBC3RGBAUnormSrgb,
		gputypes.TextureFormatBC4RUnorm,
		gputypes.TextureFormatBC4RSnorm,
		gputypes.TextureFormatBC5RGUnorm,
		gputypes.TextureFormatBC5RGSnorm,
		gputypes.TextureFormatBC6HRGBUfloat,
		gputypes.TextureFormatBC6HRGBFloat,
		gputypes.TextureFormatBC7RGBAUnorm,
		gputypes.TextureFormatBC7RGBAUnormSrgb:
		return true
	default:
		return false
	}
}

func textureFormatBlockHeight(format gputypes.TextureFormat) uint32 {
	if info, ok := textureBlockInfoForFormat(format); ok {
		return info.height
	}
	return 1
}

func alignUint32(value, alignment uint32) (uint32, bool) {
	if alignment == 0 {
		return value, true
	}
	if value > math.MaxUint32-(alignment-1) {
		return 0, false
	}
	return (value + alignment - 1) / alignment * alignment, true
}

func copyExtentBlocks(info textureBlockInfo, origin hal.Origin3D, extent hal.Extent3D) (width, height uint32, ok bool) {
	if extent.Width == 0 || extent.Height == 0 {
		return 0, 0, false
	}
	if origin.X > math.MaxUint32-extent.Width || origin.Y > math.MaxUint32-extent.Height {
		return 0, 0, false
	}
	endX := origin.X + extent.Width
	endY := origin.Y + extent.Height
	if endX > math.MaxUint32-(info.width-1) || endY > math.MaxUint32-(info.height-1) {
		return 0, 0, false
	}
	startBlockX := origin.X / info.width
	startBlockY := origin.Y / info.height
	endBlockX := (endX + info.width - 1) / info.width
	endBlockY := (endY + info.height - 1) / info.height
	if endBlockX < startBlockX || endBlockY < startBlockY {
		return 0, 0, false
	}
	return endBlockX - startBlockX, endBlockY - startBlockY, true
}

func textureAspectPlanes(texture *Texture, aspect gputypes.TextureAspect) []uint32 {
	if texture == nil || texture.planeCount() < 2 {
		return []uint32{0}
	}
	if texture.format == gputypes.TextureFormatStencil8 {
		if aspect == gputypes.TextureAspectDepthOnly {
			return nil
		}
		return []uint32{1}
	}
	if texture.format == gputypes.TextureFormatDepth24Plus {
		if aspect == gputypes.TextureAspectStencilOnly {
			return nil
		}
		return []uint32{0}
	}
	switch aspect {
	case gputypes.TextureAspectDepthOnly:
		return []uint32{0}
	case gputypes.TextureAspectStencilOnly:
		return []uint32{1}
	default:
		return []uint32{0, 1}
	}
}

func (t *Texture) planeCount() uint32 {
	switch t.format {
	case gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8:
		return 2
	default:
		return 1
	}
}

func (t *Texture) subresourceCount() uint32 {
	mips := t.mipLevels
	if mips == 0 {
		mips = 1
	}
	layers := uint32(1)
	if t.dimension != gputypes.TextureDimension3D {
		layers = t.size.DepthOrArrayLayers
		if layers == 0 {
			layers = 1
		}
	}
	return mips * layers * t.planeCount()
}

func (t *Texture) subresourceIndexForPlane(mipLevel, arrayLayer, plane uint32) uint32 {
	mips := t.mipLevels
	if mips == 0 {
		mips = 1
	}
	layers := uint32(1)
	if t.dimension != gputypes.TextureDimension3D {
		layers = t.size.DepthOrArrayLayers
		if layers == 0 {
			layers = 1
		}
	}
	return mipLevel + arrayLayer*mips + plane*mips*layers
}

func (t *Texture) subresourceIndex(mipLevel, arrayLayer uint32) uint32 {
	return t.subresourceIndexForPlane(mipLevel, arrayLayer, 0)
}

// planPlacedBufferSlice aligns a logical image slice down to D3D12's 512-byte
// placement boundary. The byte delta is represented by block-aware source
// origins, so the selected source box still starts at the original address.
func planPlacedBufferSlice(logicalOffset uint64, rowPitch, blockWidth, blockHeight, blockBytes, copyWidth, copyHeight uint32) (bufferOffset uint64, bufferOriginX, bufferOriginY, footprintWidth, footprintHeight uint32, ok bool) {
	if rowPitch == 0 || blockWidth == 0 || blockHeight == 0 || blockBytes == 0 {
		return 0, 0, 0, 0, 0, false
	}
	bufferOffset = logicalOffset &^ (d3d12TexturePlacementAlignment - 1)
	delta := logicalOffset - bufferOffset
	rowBytes := uint64(rowPitch)
	yRows := delta / rowBytes
	xBytes := delta % rowBytes
	if xBytes%uint64(blockBytes) != 0 {
		return 0, 0, 0, 0, 0, false
	}
	xBlocks := xBytes / uint64(blockBytes)
	if xBlocks > uint64(math.MaxUint32/blockWidth) || yRows > uint64(math.MaxUint32/blockHeight) {
		return 0, 0, 0, 0, 0, false
	}
	bufferOriginX = uint32(xBlocks) * blockWidth
	bufferOriginY = uint32(yRows) * blockHeight
	if bufferOriginX > math.MaxUint32-copyWidth || bufferOriginY > math.MaxUint32-copyHeight {
		return 0, 0, 0, 0, 0, false
	}
	footprintWidth, ok = alignUint32(bufferOriginX+copyWidth, blockWidth)
	if !ok {
		return 0, 0, 0, 0, 0, false
	}
	footprintHeight, ok = alignUint32(bufferOriginY+copyHeight, blockHeight)
	if !ok {
		return 0, 0, 0, 0, 0, false
	}
	if uint64(footprintWidth/blockWidth)*uint64(blockBytes) > rowBytes {
		return 0, 0, 0, 0, 0, false
	}
	return bufferOffset, bufferOriginX, bufferOriginY, footprintWidth, footprintHeight, true
}

func normalizedCopyLayout(texture *Texture, layout hal.ImageDataLayout, size hal.Extent3D) (textureBlockInfo, uint32, uint32, bool) {
	if texture == nil {
		return textureBlockInfo{}, 0, 0, false
	}
	info, ok := textureBlockInfoForFormat(texture.format)
	if !ok {
		return textureBlockInfo{}, 0, 0, false
	}
	minWidth := (uint64(size.Width) + uint64(info.width) - 1) / uint64(info.width)
	minRowBytes64 := minWidth * uint64(info.bytes)
	if minRowBytes64 == 0 || minRowBytes64 > math.MaxUint32 {
		return textureBlockInfo{}, 0, 0, false
	}
	rowPitch := layout.BytesPerRow
	if rowPitch == 0 {
		if size.Height > info.height {
			return textureBlockInfo{}, 0, 0, false
		}
		aligned, alignedOK := alignUint32(uint32(minRowBytes64), d3d12TexturePitchAlignment)
		if !alignedOK {
			return textureBlockInfo{}, 0, 0, false
		}
		rowPitch = aligned
	} else if uint64(rowPitch) < minRowBytes64 || rowPitch%d3d12TexturePitchAlignment != 0 {
		return textureBlockInfo{}, 0, 0, false
	}
	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = size.Height
	}
	if rowsPerImage < size.Height {
		return textureBlockInfo{}, 0, 0, false
	}
	blockRows := (rowsPerImage + info.height - 1) / info.height
	return info, rowPitch, blockRows, true
}

func textureWriteSourceOffset(base uint64, sourceBytesPerRow, blockRows, slice, row uint32) uint64 {
	return base + uint64(slice)*uint64(sourceBytesPerRow)*uint64(blockRows) + uint64(row)*uint64(sourceBytesPerRow)
}

func writeTextureNativeLayout(texture *Texture, layout hal.ImageDataLayout, size hal.Extent3D) (textureBlockInfo, hal.ImageDataLayout, uint32, uint32, bool) {
	if texture == nil || size.Width == 0 {
		return textureBlockInfo{}, hal.ImageDataLayout{}, 0, 0, false
	}
	info, ok := textureBlockInfoForFormat(texture.format)
	if !ok {
		return textureBlockInfo{}, hal.ImageDataLayout{}, 0, 0, false
	}
	logicalRowBytes64 := ((uint64(size.Width) + uint64(info.width) - 1) / uint64(info.width)) * uint64(info.bytes)
	if logicalRowBytes64 == 0 || logicalRowBytes64 > math.MaxUint32 {
		return textureBlockInfo{}, hal.ImageDataLayout{}, 0, 0, false
	}
	sourceBytesPerRow := layout.BytesPerRow
	if sourceBytesPerRow == 0 {
		sourceBytesPerRow = uint32(logicalRowBytes64)
	}
	if uint64(sourceBytesPerRow) < logicalRowBytes64 {
		return textureBlockInfo{}, hal.ImageDataLayout{}, 0, 0, false
	}
	nativeRowPitch, ok := alignUint32(uint32(logicalRowBytes64), d3d12TexturePitchAlignment)
	if !ok {
		return textureBlockInfo{}, hal.ImageDataLayout{}, 0, 0, false
	}
	nativeLayout := layout
	nativeLayout.BytesPerRow = nativeRowPitch
	_, _, blockRows, ok := normalizedCopyLayout(texture, nativeLayout, size)
	if !ok {
		return textureBlockInfo{}, hal.ImageDataLayout{}, 0, 0, false
	}
	return info, nativeLayout, sourceBytesPerRow, blockRows, true
}

// planBufferTextureCopies converts a WebGPU buffer/texture copy into one plan
// per D3D12 subresource or 3D depth slice. It returns nil when the layout
// cannot be represented without changing logical byte addresses.
func planBufferTextureCopies(texture *Texture, copy hal.ImageCopyTexture, layout hal.ImageDataLayout, size hal.Extent3D) []bufferTextureCopyPlan {
	info, rowPitch, blockRows, ok := normalizedCopyLayout(texture, layout, size)
	if !ok || texture == nil {
		return nil
	}
	widthBlocks, heightBlocks, ok := copyExtentBlocks(info, copy.Origin, size)
	if !ok || widthBlocks == 0 || heightBlocks == 0 {
		return nil
	}
	if copy.MipLevel >= maxTextureMips(texture) {
		return nil
	}
	depth := size.DepthOrArrayLayers
	if depth == 0 {
		depth = 1
	}
	if !validTextureCopyDepth(texture, copy.Origin.Z, depth) {
		return nil
	}
	planes := textureAspectPlanes(texture, copy.Aspect)
	if len(planes) == 0 {
		return nil
	}
	stride := uint64(rowPitch) * uint64(blockRows)
	if stride > math.MaxUint64/uint64(depth) {
		return nil
	}
	plans := make([]bufferTextureCopyPlan, 0, len(planes)*int(depth))
	for _, plane := range planes {
		for slice := uint32(0); slice < depth; slice++ {
			logicalOffset := layout.Offset + uint64(slice)*stride
			if logicalOffset < layout.Offset {
				return nil
			}
			bufferOffset, originX, originY, footprintWidth, footprintHeight, representable := planPlacedBufferSlice(
				logicalOffset, rowPitch, info.width, info.height, info.bytes,
				widthBlocks*info.width, heightBlocks*info.height)
			if !representable {
				return nil
			}
			subresource := texture.subresourceIndexForPlane(copy.MipLevel, 0, plane)
			textureOriginZ := copy.Origin.Z + slice
			if texture.dimension != gputypes.TextureDimension3D {
				subresource = texture.subresourceIndexForPlane(copy.MipLevel, copy.Origin.Z+slice, plane)
				textureOriginZ = 0
			}
			plans = append(plans, bufferTextureCopyPlan{
				subresource:     subresource,
				bufferOffset:    bufferOffset,
				bufferOriginX:   originX,
				bufferOriginY:   originY,
				footprintWidth:  footprintWidth,
				footprintHeight: footprintHeight,
				footprintDepth:  1,
				textureOriginX:  copy.Origin.X,
				textureOriginY:  copy.Origin.Y,
				textureOriginZ:  textureOriginZ,
				rowPitch:        rowPitch,
				copyWidth:       widthBlocks * info.width,
				copyHeight:      heightBlocks * info.height,
			})
		}
	}
	return plans
}

func maxTextureMips(texture *Texture) uint32 {
	if texture.mipLevels == 0 {
		return 1
	}
	return texture.mipLevels
}

func validTextureCopyDepth(texture *Texture, origin, depth uint32) bool {
	if origin > math.MaxUint32-depth {
		return false
	}
	if texture.dimension == gputypes.TextureDimension3D {
		layers := texture.size.DepthOrArrayLayers
		return layers == 0 || origin+depth <= layers
	}
	layers := texture.size.DepthOrArrayLayers
	if layers == 0 {
		layers = 1
	}
	return origin+depth <= layers
}

// planTextureTextureCopies applies D3D12's distinct 3D-volume and 2D-array
// copy models while pairing selected source and destination planes.
func planTextureTextureCopies(src, dst *Texture, copy hal.TextureCopy) []textureTextureCopyPlan {
	if src == nil || dst == nil || src.dimension != dst.dimension {
		return nil
	}
	depth := copy.Size.DepthOrArrayLayers
	if depth == 0 {
		depth = 1
	}
	if copy.SrcBase.MipLevel >= maxTextureMips(src) || copy.DstBase.MipLevel >= maxTextureMips(dst) {
		return nil
	}
	if !validTextureCopyDepth(src, copy.SrcBase.Origin.Z, depth) || !validTextureCopyDepth(dst, copy.DstBase.Origin.Z, depth) {
		return nil
	}
	srcPlanes := textureAspectPlanes(src, copy.SrcBase.Aspect)
	dstPlanes := textureAspectPlanes(dst, copy.DstBase.Aspect)
	if len(srcPlanes) == 0 || len(srcPlanes) != len(dstPlanes) {
		return nil
	}
	if copy.Size.Width == 0 || copy.Size.Height == 0 {
		return nil
	}
	if src.dimension == gputypes.TextureDimension3D {
		plans := make([]textureTextureCopyPlan, 0, len(srcPlanes))
		for plane := range srcPlanes {
			plans = append(plans, textureTextureCopyPlan{
				srcSubresource: src.subresourceIndexForPlane(copy.SrcBase.MipLevel, 0, srcPlanes[plane]),
				dstSubresource: dst.subresourceIndexForPlane(copy.DstBase.MipLevel, 0, dstPlanes[plane]),
				srcFront:       copy.SrcBase.Origin.Z,
				srcBack:        copy.SrcBase.Origin.Z + depth,
				dstZ:           copy.DstBase.Origin.Z,
			})
		}
		return plans
	}
	plans := make([]textureTextureCopyPlan, 0, len(srcPlanes)*int(depth))
	for plane := range srcPlanes {
		for layer := uint32(0); layer < depth; layer++ {
			plans = append(plans, textureTextureCopyPlan{
				srcSubresource: src.subresourceIndexForPlane(copy.SrcBase.MipLevel, copy.SrcBase.Origin.Z+layer, srcPlanes[plane]),
				dstSubresource: dst.subresourceIndexForPlane(copy.DstBase.MipLevel, copy.DstBase.Origin.Z+layer, dstPlanes[plane]),
				srcFront:       0,
				srcBack:        1,
				dstZ:           0,
			})
		}
	}
	return plans
}
