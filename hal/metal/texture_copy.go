// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

type metalTextureShape struct {
	depth       uint32
	arrayLength uint32
}

// metalTextureDescriptorShape separates WebGPU's combined depth-or-layers
// value into Metal's physical depth and array length properties.
func metalTextureDescriptorShape(dimension gputypes.TextureDimension, depthOrArrayLayers uint32) metalTextureShape {
	if depthOrArrayLayers == 0 {
		depthOrArrayLayers = 1
	}
	if dimension == gputypes.TextureDimension3D {
		return metalTextureShape{depth: depthOrArrayLayers, arrayLength: 1}
	}
	return metalTextureShape{depth: 1, arrayLength: depthOrArrayLayers}
}

type metalTextureCopyRegion struct {
	slice  NSUInteger
	origin MTLOrigin
	size   MTLSize
}

type metalCopyPlan struct {
	operationCount uint32
	splitDepth     bool
}

func planMetalBufferTextureCopy(dimension gputypes.TextureDimension, depthOrArrayLayers uint32) metalCopyPlan {
	return newMetalCopyPlan(dimension != gputypes.TextureDimension3D, depthOrArrayLayers)
}

func planMetalTextureCopy(sourceDimension, destinationDimension gputypes.TextureDimension, depthOrArrayLayers uint32) metalCopyPlan {
	return newMetalCopyPlan(
		sourceDimension != gputypes.TextureDimension3D || destinationDimension != gputypes.TextureDimension3D,
		depthOrArrayLayers,
	)
}

func newMetalCopyPlan(splitDepth bool, depthOrArrayLayers uint32) metalCopyPlan {
	if depthOrArrayLayers == 0 {
		return metalCopyPlan{splitDepth: splitDepth}
	}
	operationCount := uint32(1)
	if splitDepth {
		operationCount = depthOrArrayLayers
	}
	return metalCopyPlan{operationCount: operationCount, splitDepth: splitDepth}
}

func (p metalCopyPlan) textureRegion(dimension gputypes.TextureDimension, origin hal.Origin3D, size hal.Extent3D, operation uint32) (metalTextureCopyRegion, bool) {
	if operation >= p.operationCount {
		return metalTextureCopyRegion{}, false
	}
	region := metalTextureCopyRegion{
		origin: MTLOrigin{X: NSUInteger(origin.X), Y: NSUInteger(origin.Y), Z: NSUInteger(origin.Z)},
		size: MTLSize{
			Width:  NSUInteger(size.Width),
			Height: NSUInteger(size.Height),
			Depth:  NSUInteger(size.DepthOrArrayLayers),
		},
	}
	if dimension != gputypes.TextureDimension3D {
		layer, ok := checkedMetalTextureDataAdd32(origin.Z, operation)
		if !ok {
			return metalTextureCopyRegion{}, false
		}
		region.slice = NSUInteger(layer)
		region.origin.Z = 0
		region.size.Depth = 1
	} else if p.splitDepth {
		depth, ok := checkedMetalTextureDataAdd32(origin.Z, operation)
		if !ok {
			return metalTextureCopyRegion{}, false
		}
		region.origin.Z = NSUInteger(depth)
		region.size.Depth = 1
	}
	return region, true
}

func (p metalCopyPlan) bufferOffset(base, bytesPerImage uint64, operation uint32) (uint64, bool) {
	if operation >= p.operationCount {
		return 0, false
	}
	if !p.splitDepth {
		return base, true
	}
	imageOffset, ok := checkedMetalTextureDataMul(uint64(operation), bytesPerImage)
	if !ok {
		return 0, false
	}
	return checkedMetalTextureDataAdd(base, imageOffset)
}

func metalCopyBytesPerImage(format gputypes.TextureFormat, bytesPerRow, rowsPerImage, imageHeight uint32) (uint64, bool) {
	if rowsPerImage == 0 {
		_, blockHeight, _, ok := metalTextureFormatBlockInfo(format)
		if !ok {
			return 0, false
		}
		rowsPerImage = uint32((uint64(imageHeight) + uint64(blockHeight) - 1) / uint64(blockHeight))
	}
	return checkedMetalTextureDataMul(uint64(rowsPerImage), uint64(bytesPerRow))
}

type metalTextureDataCopyLayout struct {
	bytesPerRow   uint32
	bytesPerImage uint64
}

type metalCopyStrides struct {
	bytesPerRow   uint64
	bytesPerImage uint64
}

// metalReplaceRegionStrides applies MTLTexture.replaceRegion's dimension rules.
// Metal infers the sole row of a 1D texture and only accepts an image stride for 3D.
func metalReplaceRegionStrides(dimension gputypes.TextureDimension, bytesPerRow, bytesPerImage uint64) metalCopyStrides {
	switch dimension {
	case gputypes.TextureDimension1D:
		return metalCopyStrides{}
	case gputypes.TextureDimension3D:
		return metalCopyStrides{bytesPerRow: bytesPerRow, bytesPerImage: bytesPerImage}
	default:
		return metalCopyStrides{bytesPerRow: bytesPerRow}
	}
}

// metalBlitStrides applies MTLBlitCommandEncoder's buffer/texture copy rules.
// Its row stride remains explicit; its image stride is only valid for true 3D.
func metalBlitStrides(dimension gputypes.TextureDimension, bytesPerRow, bytesPerImage uint64) metalCopyStrides {
	strides := metalCopyStrides{bytesPerRow: bytesPerRow}
	if dimension == gputypes.TextureDimension3D {
		strides.bytesPerImage = bytesPerImage
	}
	return strides
}

func validateMetalBufferTextureCopyPlan(
	format gputypes.TextureFormat,
	dimension gputypes.TextureDimension,
	layout hal.ImageDataLayout,
	origin hal.Origin3D,
	size hal.Extent3D,
) (metalCopyPlan, uint64, bool) {
	bytesPerImage, ok := metalCopyBytesPerImage(format, layout.BytesPerRow, layout.RowsPerImage, size.Height)
	if !ok {
		return metalCopyPlan{}, 0, false
	}
	plan := planMetalBufferTextureCopy(dimension, size.DepthOrArrayLayers)
	if plan.operationCount == 0 {
		return metalCopyPlan{}, 0, false
	}
	lastOperation := plan.operationCount - 1
	if _, ok := plan.textureRegion(dimension, origin, size, lastOperation); !ok {
		return metalCopyPlan{}, 0, false
	}
	if _, ok := plan.bufferOffset(layout.Offset, bytesPerImage, lastOperation); !ok {
		return metalCopyPlan{}, 0, false
	}
	return plan, bytesPerImage, true
}

func validateMetalTextureCopyPlan(
	sourceDimension, destinationDimension gputypes.TextureDimension,
	sourceOrigin, destinationOrigin hal.Origin3D,
	size hal.Extent3D,
) (metalCopyPlan, bool) {
	plan := planMetalTextureCopy(sourceDimension, destinationDimension, size.DepthOrArrayLayers)
	if plan.operationCount == 0 {
		return metalCopyPlan{}, false
	}
	lastOperation := plan.operationCount - 1
	if _, ok := plan.textureRegion(sourceDimension, sourceOrigin, size, lastOperation); !ok {
		return metalCopyPlan{}, false
	}
	if _, ok := plan.textureRegion(destinationDimension, destinationOrigin, size, lastOperation); !ok {
		return metalCopyPlan{}, false
	}
	return plan, true
}

// validateMetalTextureDataCopyLayout proves that every byte Metal may read is
// present before either the Shared direct-write or Private staging path starts.
func validateMetalTextureDataCopyLayout(dataLength uint64, format gputypes.TextureFormat, layout hal.ImageDataLayout, size hal.Extent3D) (metalTextureDataCopyLayout, error) {
	if size.Width == 0 || size.Height == 0 || size.DepthOrArrayLayers == 0 {
		return metalTextureDataCopyLayout{}, fmt.Errorf("copy extent must be non-zero")
	}

	rowBytes, blockRows, ok := metalTextureCopyGeometry(format, size.Width, size.Height)
	if !ok {
		return metalTextureDataCopyLayout{}, fmt.Errorf("texture format %s has no defined copy block size", format)
	}
	if rowBytes > math.MaxUint32 {
		return metalTextureDataCopyLayout{}, fmt.Errorf("texture row size overflows BytesPerRow")
	}

	bytesPerRow := layout.BytesPerRow
	if bytesPerRow == 0 {
		bytesPerRow = uint32(rowBytes)
	}
	if uint64(bytesPerRow) < rowBytes {
		return metalTextureDataCopyLayout{}, fmt.Errorf("BytesPerRow %d is smaller than the copied row size %d", bytesPerRow, rowBytes)
	}

	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = uint32(blockRows)
	}
	if uint64(rowsPerImage) < blockRows {
		return metalTextureDataCopyLayout{}, fmt.Errorf("RowsPerImage %d is smaller than the copied block-row count %d", rowsPerImage, blockRows)
	}

	bytesPerImage, ok := checkedMetalTextureDataMul(uint64(rowsPerImage), uint64(bytesPerRow))
	if !ok {
		return metalTextureDataCopyLayout{}, fmt.Errorf("BytesPerImage overflows uint64")
	}
	lastImageOffset, ok := checkedMetalTextureDataMul(uint64(size.DepthOrArrayLayers-1), bytesPerImage)
	if !ok {
		return metalTextureDataCopyLayout{}, fmt.Errorf("last image offset overflows uint64")
	}
	lastRowOffset, ok := checkedMetalTextureDataMul(blockRows-1, uint64(bytesPerRow))
	if !ok {
		return metalTextureDataCopyLayout{}, fmt.Errorf("last row offset overflows uint64")
	}
	requiredBytes, ok := checkedMetalTextureDataAdd(layout.Offset, lastImageOffset)
	if ok {
		requiredBytes, ok = checkedMetalTextureDataAdd(requiredBytes, lastRowOffset)
	}
	if ok {
		requiredBytes, ok = checkedMetalTextureDataAdd(requiredBytes, rowBytes)
	}
	if !ok {
		return metalTextureDataCopyLayout{}, fmt.Errorf("source data range overflows uint64")
	}
	if requiredBytes > dataLength {
		return metalTextureDataCopyLayout{}, fmt.Errorf("source data is too short: need %d bytes, have %d", requiredBytes, dataLength)
	}

	return metalTextureDataCopyLayout{bytesPerRow: bytesPerRow, bytesPerImage: bytesPerImage}, nil
}

func metalTextureCopyGeometry(format gputypes.TextureFormat, width, height uint32) (rowBytes, blockRows uint64, ok bool) {
	blockWidth, blockHeight, blockSize, ok := metalTextureFormatBlockInfo(format)
	if !ok {
		return 0, 0, false
	}
	blockColumns := (uint64(width) + uint64(blockWidth) - 1) / uint64(blockWidth)
	blockRows = (uint64(height) + uint64(blockHeight) - 1) / uint64(blockHeight)
	rowBytes, ok = checkedMetalTextureDataMul(blockColumns, uint64(blockSize))
	return rowBytes, blockRows, ok
}

func metalTextureFormatBlockInfo(format gputypes.TextureFormat) (width, height, size uint32, ok bool) {
	blockSize := format.BlockCopySize()
	if blockSize == 0 {
		return 0, 0, 0, false
	}

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
		gputypes.TextureFormatBC7RGBAUnormSrgb,
		gputypes.TextureFormatETC2RGB8Unorm,
		gputypes.TextureFormatETC2RGB8UnormSrgb,
		gputypes.TextureFormatETC2RGB8A1Unorm,
		gputypes.TextureFormatETC2RGB8A1UnormSrgb,
		gputypes.TextureFormatETC2RGBA8Unorm,
		gputypes.TextureFormatETC2RGBA8UnormSrgb,
		gputypes.TextureFormatEACR11Unorm,
		gputypes.TextureFormatEACR11Snorm,
		gputypes.TextureFormatEACRG11Unorm,
		gputypes.TextureFormatEACRG11Snorm:
		return 4, 4, blockSize, true
	}

	switch format {
	case gputypes.TextureFormatASTC4x4Unorm, gputypes.TextureFormatASTC4x4UnormSrgb:
		return 4, 4, blockSize, true
	case gputypes.TextureFormatASTC5x4Unorm, gputypes.TextureFormatASTC5x4UnormSrgb:
		return 5, 4, blockSize, true
	case gputypes.TextureFormatASTC5x5Unorm, gputypes.TextureFormatASTC5x5UnormSrgb:
		return 5, 5, blockSize, true
	case gputypes.TextureFormatASTC6x5Unorm, gputypes.TextureFormatASTC6x5UnormSrgb:
		return 6, 5, blockSize, true
	case gputypes.TextureFormatASTC6x6Unorm, gputypes.TextureFormatASTC6x6UnormSrgb:
		return 6, 6, blockSize, true
	case gputypes.TextureFormatASTC8x5Unorm, gputypes.TextureFormatASTC8x5UnormSrgb:
		return 8, 5, blockSize, true
	case gputypes.TextureFormatASTC8x6Unorm, gputypes.TextureFormatASTC8x6UnormSrgb:
		return 8, 6, blockSize, true
	case gputypes.TextureFormatASTC8x8Unorm, gputypes.TextureFormatASTC8x8UnormSrgb:
		return 8, 8, blockSize, true
	case gputypes.TextureFormatASTC10x5Unorm, gputypes.TextureFormatASTC10x5UnormSrgb:
		return 10, 5, blockSize, true
	case gputypes.TextureFormatASTC10x6Unorm, gputypes.TextureFormatASTC10x6UnormSrgb:
		return 10, 6, blockSize, true
	case gputypes.TextureFormatASTC10x8Unorm, gputypes.TextureFormatASTC10x8UnormSrgb:
		return 10, 8, blockSize, true
	case gputypes.TextureFormatASTC10x10Unorm, gputypes.TextureFormatASTC10x10UnormSrgb:
		return 10, 10, blockSize, true
	case gputypes.TextureFormatASTC12x10Unorm, gputypes.TextureFormatASTC12x10UnormSrgb:
		return 12, 10, blockSize, true
	case gputypes.TextureFormatASTC12x12Unorm, gputypes.TextureFormatASTC12x12UnormSrgb:
		return 12, 12, blockSize, true
	default:
		return 1, 1, blockSize, true
	}
}

func checkedMetalTextureDataMul(a, b uint64) (uint64, bool) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, false
	}
	return a * b, true
}

func checkedMetalTextureDataAdd(a, b uint64) (uint64, bool) {
	if b > math.MaxUint64-a {
		return 0, false
	}
	return a + b, true
}

func checkedMetalTextureDataAdd32(a, b uint32) (uint32, bool) {
	if b > math.MaxUint32-a {
		return 0, false
	}
	return a + b, true
}
