// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"math"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestMetalTextureDataCopyLayoutValidatesCompleteSourceRange(t *testing.T) {
	const (
		width         = uint32(4)
		height        = uint32(2)
		depth         = uint32(3)
		bytesPerRow   = uint32(32)
		rowsPerImage  = uint32(4)
		requiredBytes = uint64(321)
	)
	layout := hal.ImageDataLayout{Offset: 17, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
	size := hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: depth}

	got, err := validateMetalTextureDataCopyLayout(requiredBytes, gputypes.TextureFormatRGBA8Unorm, layout, size)
	if err != nil {
		t.Fatalf("exact source range rejected: %v", err)
	}
	if got.bytesPerRow != bytesPerRow || got.bytesPerImage != uint64(bytesPerRow)*uint64(rowsPerImage) {
		t.Fatalf("normalized layout = %+v; want bytesPerRow %d, bytesPerImage %d", got, bytesPerRow, bytesPerRow*rowsPerImage)
	}

	tests := []struct {
		name       string
		dataLength uint64
		layout     hal.ImageDataLayout
		size       hal.Extent3D
	}{
		{
			name:       "truncated later layer",
			dataLength: requiredBytes - 1,
			layout:     layout,
			size:       size,
		},
		{
			name:       "offset plus final row overflows",
			dataLength: math.MaxUint64,
			layout:     hal.ImageDataLayout{Offset: math.MaxUint64 - 3, BytesPerRow: 8, RowsPerImage: 1},
			size:       hal.Extent3D{Width: 2, Height: 1, DepthOrArrayLayers: 1},
		},
		{
			name:       "rows per image times later layers overflows",
			dataLength: math.MaxUint64,
			layout:     hal.ImageDataLayout{BytesPerRow: math.MaxUint32, RowsPerImage: math.MaxUint32},
			size:       hal.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: math.MaxUint32},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateMetalTextureDataCopyLayout(test.dataLength, gputypes.TextureFormatRGBA8Unorm, test.layout, test.size); err == nil {
				t.Fatal("invalid source range accepted")
			}
		})
	}
}

func TestMetalTextureFormatBlockInfoCoversCompressedFormats(t *testing.T) {
	for format := gputypes.TextureFormatBC1RGBAUnorm; format <= gputypes.TextureFormatASTC12x12UnormSrgb; format++ {
		width, height, size, ok := metalTextureFormatBlockInfo(format)
		if !ok || width == 0 || height == 0 || size == 0 {
			t.Fatalf("compressed format %s (%d) has incomplete block info: %dx%d, %d bytes, ok=%t", format, format, width, height, size, ok)
		}
	}

	tests := []struct {
		format        gputypes.TextureFormat
		width, height uint32
	}{
		{gputypes.TextureFormatBC7RGBAUnorm, 4, 4},
		{gputypes.TextureFormatETC2RGBA8Unorm, 4, 4},
		{gputypes.TextureFormatASTC4x4Unorm, 4, 4},
		{gputypes.TextureFormatASTC5x4Unorm, 5, 4},
		{gputypes.TextureFormatASTC5x5Unorm, 5, 5},
		{gputypes.TextureFormatASTC6x5Unorm, 6, 5},
		{gputypes.TextureFormatASTC6x6Unorm, 6, 6},
		{gputypes.TextureFormatASTC8x5Unorm, 8, 5},
		{gputypes.TextureFormatASTC8x6Unorm, 8, 6},
		{gputypes.TextureFormatASTC8x8Unorm, 8, 8},
		{gputypes.TextureFormatASTC10x5Unorm, 10, 5},
		{gputypes.TextureFormatASTC10x6Unorm, 10, 6},
		{gputypes.TextureFormatASTC10x8Unorm, 10, 8},
		{gputypes.TextureFormatASTC10x10Unorm, 10, 10},
		{gputypes.TextureFormatASTC12x10Unorm, 12, 10},
		{gputypes.TextureFormatASTC12x12Unorm, 12, 12},
	}
	for _, test := range tests {
		width, height, _, ok := metalTextureFormatBlockInfo(test.format)
		if !ok || width != test.width || height != test.height {
			t.Errorf("%s block dimensions = %dx%d, ok=%t; want %dx%d", test.format, width, height, ok, test.width, test.height)
		}
	}
}

func TestMetalTextureDataCopyLayoutUsesCompressedBlockRows(t *testing.T) {
	layout, err := validateMetalTextureDataCopyLayout(
		64,
		gputypes.TextureFormatASTC5x4Unorm,
		hal.ImageDataLayout{},
		hal.Extent3D{Width: 6, Height: 5, DepthOrArrayLayers: 1},
	)
	if err != nil {
		t.Fatalf("compressed layout rejected: %v", err)
	}
	if layout.bytesPerRow != 32 || layout.bytesPerImage != 64 {
		t.Fatalf("compressed layout = %+v; want 32 bytes per row and 64 bytes per image", layout)
	}
}

func TestMetalCopyBytesPerImageUsesCompressedBlockRows(t *testing.T) {
	got, ok := metalCopyBytesPerImage(gputypes.TextureFormatASTC5x4Unorm, 32, 0, 5)
	if !ok || got != 64 {
		t.Fatalf("compressed bytes per image = %d, ok=%t; want 64, true", got, ok)
	}

	got, ok = metalCopyBytesPerImage(gputypes.TextureFormatRGBA8Unorm, 32, 0, 5)
	if !ok || got != 160 {
		t.Fatalf("plain bytes per image = %d, ok=%t; want 160, true", got, ok)
	}
}

func TestMetalReplaceRegionStridesMatchTextureDimension(t *testing.T) {
	tests := []struct {
		name                          string
		dimension                     gputypes.TextureDimension
		wantBytesPerRow, wantPerImage uint64
	}{
		{name: "1D array", dimension: gputypes.TextureDimension1D, wantBytesPerRow: 0, wantPerImage: 0},
		{name: "2D array", dimension: gputypes.TextureDimension2D, wantBytesPerRow: 256, wantPerImage: 0},
		{name: "3D", dimension: gputypes.TextureDimension3D, wantBytesPerRow: 256, wantPerImage: 1024},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := metalReplaceRegionStrides(test.dimension, 256, 1024)
			if got.bytesPerRow != test.wantBytesPerRow || got.bytesPerImage != test.wantPerImage {
				t.Fatalf("strides = %+v; want row %d image %d", got, test.wantBytesPerRow, test.wantPerImage)
			}
		})
	}
}

func TestMetalBlitStridesMatchTextureDimension(t *testing.T) {
	for _, dimension := range []gputypes.TextureDimension{gputypes.TextureDimension1D, gputypes.TextureDimension2D} {
		got := metalBlitStrides(dimension, 256, 1024)
		if got.bytesPerRow != 256 || got.bytesPerImage != 0 {
			t.Errorf("%s blit strides = %+v; want row 256 image 0", dimension, got)
		}
	}
	got := metalBlitStrides(gputypes.TextureDimension3D, 256, 1024)
	if got.bytesPerRow != 256 || got.bytesPerImage != 1024 {
		t.Errorf("3D blit strides = %+v; want row 256 image 1024", got)
	}
}

func TestMetalTextureDescriptorShape2DArray(t *testing.T) {
	shape := metalTextureDescriptorShape(gputypes.TextureDimension2D, 4)

	if shape.depth != 1 || shape.arrayLength != 4 {
		t.Fatalf("2D array shape = depth %d, array length %d; want 1, 4", shape.depth, shape.arrayLength)
	}
}

func TestMetalBufferTextureCopyPlan2DArray(t *testing.T) {
	plan := planMetalBufferTextureCopy(gputypes.TextureDimension2D, 4)
	if plan.operationCount != 4 {
		t.Fatalf("operation count = %d; want 4", plan.operationCount)
	}

	region, ok := plan.textureRegion(
		gputypes.TextureDimension2D,
		hal.Origin3D{X: 3, Y: 4, Z: 5},
		hal.Extent3D{Width: 16, Height: 8, DepthOrArrayLayers: 4},
		2,
	)
	if !ok {
		t.Fatal("valid texture region overflowed")
	}
	if region.slice != 7 {
		t.Errorf("slice = %d; want 7", region.slice)
	}
	if region.origin != (MTLOrigin{X: 3, Y: 4, Z: 0}) {
		t.Errorf("origin = %+v; want {3 4 0}", region.origin)
	}
	if region.size != (MTLSize{Width: 16, Height: 8, Depth: 1}) {
		t.Errorf("size = %+v; want {16 8 1}", region.size)
	}
	if got, ok := plan.bufferOffset(64, 1024, 2); !ok || got != 2112 {
		t.Errorf("buffer offset = %d; want 2112", got)
	}
}

func TestMetalCopyPlanRejectsWrappedOriginAndBufferOffset(t *testing.T) {
	plan := planMetalBufferTextureCopy(gputypes.TextureDimension2D, 2)
	if _, ok := plan.textureRegion(
		gputypes.TextureDimension2D,
		hal.Origin3D{Z: math.MaxUint32},
		hal.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 2},
		1,
	); ok {
		t.Fatal("wrapped array-layer origin accepted")
	}
	if _, ok := plan.bufferOffset(math.MaxUint64-7, 8, 1); ok {
		t.Fatal("wrapped buffer offset accepted")
	}
	if _, ok := plan.bufferOffset(0, math.MaxUint64, 2); ok {
		t.Fatal("wrapped image-stride multiplication accepted")
	}
}

func TestCommandEncoderRejectsWrappedCopyBeforeMetalCall(t *testing.T) {
	// Sentinel native handles are deliberately invalid. These calls only remain
	// safe if arithmetic preflight returns before asking Objective-C for a blit
	// encoder or issuing a copy selector.
	encoder := &CommandEncoder{cmdBuffer: 1}
	buffer := &Buffer{raw: 1}
	array := &Texture{raw: 1, dimension: gputypes.TextureDimension2D, format: gputypes.TextureFormatRGBA8Unorm}
	volume := &Texture{raw: 1, dimension: gputypes.TextureDimension3D, format: gputypes.TextureFormatRGBA8Unorm}
	size := hal.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 2}

	encoder.CopyBufferToTexture(buffer, array, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{Offset: math.MaxUint64 - 1, BytesPerRow: 4, RowsPerImage: 1},
		TextureBase:  hal.ImageCopyTexture{Texture: array},
		Size:         size,
	}})
	encoder.CopyTextureToBuffer(array, buffer, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: 4, RowsPerImage: 1},
		TextureBase:  hal.ImageCopyTexture{Texture: array, Origin: hal.Origin3D{Z: math.MaxUint32}},
		Size:         size,
	}})
	encoder.CopyTextureToTexture(volume, array, []hal.TextureCopy{{
		SrcBase: hal.ImageCopyTexture{Texture: volume, Origin: hal.Origin3D{Z: math.MaxUint32}},
		DstBase: hal.ImageCopyTexture{Texture: array},
		Size:    size,
	}})
}

func TestMetalBufferTextureCopyPlan3D(t *testing.T) {
	plan := planMetalBufferTextureCopy(gputypes.TextureDimension3D, 4)
	if plan.operationCount != 1 {
		t.Fatalf("operation count = %d; want 1", plan.operationCount)
	}

	region, ok := plan.textureRegion(
		gputypes.TextureDimension3D,
		hal.Origin3D{X: 3, Y: 4, Z: 5},
		hal.Extent3D{Width: 16, Height: 8, DepthOrArrayLayers: 4},
		0,
	)
	if !ok {
		t.Fatal("valid texture region overflowed")
	}
	if region.slice != 0 {
		t.Errorf("slice = %d; want 0", region.slice)
	}
	if region.origin != (MTLOrigin{X: 3, Y: 4, Z: 5}) {
		t.Errorf("origin = %+v; want {3 4 5}", region.origin)
	}
	if region.size != (MTLSize{Width: 16, Height: 8, Depth: 4}) {
		t.Errorf("size = %+v; want {16 8 4}", region.size)
	}
	if got, ok := plan.bufferOffset(64, 1024, 0); !ok || got != 64 {
		t.Errorf("buffer offset = %d; want 64", got)
	}
}

func TestMetalTextureCopyPlanSplits3DTo2DArray(t *testing.T) {
	plan := planMetalTextureCopy(gputypes.TextureDimension3D, gputypes.TextureDimension2D, 4)
	if plan.operationCount != 4 {
		t.Fatalf("operation count = %d; want 4", plan.operationCount)
	}

	source, ok := plan.textureRegion(
		gputypes.TextureDimension3D,
		hal.Origin3D{X: 1, Y: 2, Z: 3},
		hal.Extent3D{Width: 16, Height: 8, DepthOrArrayLayers: 4},
		2,
	)
	if !ok {
		t.Fatal("valid source region overflowed")
	}
	destination, ok := plan.textureRegion(
		gputypes.TextureDimension2D,
		hal.Origin3D{X: 4, Y: 5, Z: 6},
		hal.Extent3D{Width: 16, Height: 8, DepthOrArrayLayers: 4},
		2,
	)
	if !ok {
		t.Fatal("valid destination region overflowed")
	}
	if source.slice != 0 || source.origin.Z != 5 || source.size.Depth != 1 {
		t.Errorf("3D source = slice %d, origin %+v, size %+v; want slice 0, z 5, depth 1", source.slice, source.origin, source.size)
	}
	if destination.slice != 8 || destination.origin.Z != 0 || destination.size.Depth != 1 {
		t.Errorf("2D-array destination = slice %d, origin %+v, size %+v; want slice 8, z 0, depth 1", destination.slice, destination.origin, destination.size)
	}
}

func TestMetalTextureDescriptorShape3D(t *testing.T) {
	shape := metalTextureDescriptorShape(gputypes.TextureDimension3D, 4)

	if shape.depth != 4 || shape.arrayLength != 1 {
		t.Fatalf("3D shape = depth %d, array length %d; want 4, 1", shape.depth, shape.arrayLength)
	}
}

func TestDeviceCreateTexture2DArrayUsesMetalArrayShape(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	rawDevice := CreateSystemDefaultDevice()
	if rawDevice == 0 {
		t.Skip("no Metal device available")
	}
	defer Release(rawDevice)

	device, err := newDevice(&Adapter{raw: rawDevice})
	if err != nil {
		t.Fatalf("newDevice failed: %v", err)
	}
	defer device.Destroy()

	rawTexture, err := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 4},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture failed: %v", err)
	}
	texture := rawTexture.(*Texture)
	defer texture.Destroy()

	if texture.depth != 4 {
		t.Errorf("retained logical depth = %d; want 4 array layers", texture.depth)
	}
	if got := MsgSendUint(texture.raw, Sel("depth")); got != 1 {
		t.Errorf("Metal physical depth = %d; want 1", got)
	}
	if got := MsgSendUint(texture.raw, Sel("arrayLength")); got != 4 {
		t.Errorf("Metal array length = %d; want 4", got)
	}
	if got := MTLTextureType(MsgSendUint(texture.raw, Sel("textureType"))); got != MTLTextureType2DArray {
		t.Errorf("Metal texture type = %d; want %d", got, MTLTextureType2DArray)
	}
}

func TestCommandEncoderCopiesLayeredShapes(t *testing.T) {
	device, queue := newMetalTextureCopyTestDevice(t)
	for _, shape := range []struct {
		name      string
		dimension gputypes.TextureDimension
		height    uint32
		yOrigin   uint32
	}{
		{name: "1D array", dimension: gputypes.TextureDimension1D, height: 1},
		{name: "2D array", dimension: gputypes.TextureDimension2D, height: 2, yOrigin: 1},
		{name: "true 3D", dimension: gputypes.TextureDimension3D, height: 2, yOrigin: 1},
	} {
		t.Run(shape.name, func(t *testing.T) {
			const width, depth, bytesPerRow, offset = uint32(4), uint32(3), uint32(256), uint64(16)
			rowsPerImage := shape.height + 1
			bytesPerImage := bytesPerRow * rowsPerImage
			data := metalTextureArrayTestData(width, shape.height, depth, bytesPerRow, bytesPerImage)
			upload := createMetalTextureCopyTestUpload(t, device, offset, data)
			defer upload.Destroy()
			textureHeight := uint32(1)
			if shape.dimension != gputypes.TextureDimension1D {
				textureHeight = shape.height + 2
			}
			texture := createMetalTextureCopyTestTexture(t, device, shape.dimension, width+2, textureHeight, depth+2)
			defer texture.Destroy()
			readback := createMetalTextureCopyTestReadback(t, device, offset+uint64(len(data)))
			defer readback.Destroy()

			layout := hal.ImageDataLayout{Offset: offset, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
			origin := hal.Origin3D{X: 1, Y: shape.yOrigin, Z: 1}
			size := hal.Extent3D{Width: width, Height: shape.height, DepthOrArrayLayers: depth}
			encoder := &CommandEncoder{device: device}
			if err := encoder.BeginEncoding(shape.name + " buffer round trip"); err != nil {
				t.Fatalf("BeginEncoding failed: %v", err)
			}
			encoder.CopyBufferToTexture(upload, texture, []hal.BufferTextureCopy{{BufferLayout: layout, TextureBase: hal.ImageCopyTexture{Texture: texture, Origin: origin}, Size: size}})
			encoder.CopyTextureToBuffer(texture, readback, []hal.BufferTextureCopy{{BufferLayout: layout, TextureBase: hal.ImageCopyTexture{Texture: texture, Origin: origin}, Size: size}})
			submitMetalTextureCopyTestEncoder(t, device, queue, encoder)

			got := unsafe.Slice((*byte)(readback.Contents()), int(offset)+len(data))[offset:]
			assertMetalTextureArrayTestData(t, got, data, width, shape.height, depth, bytesPerRow, bytesPerImage)
		})
	}
}

func TestCommandEncoderCopiesBetweenLayeredTextures(t *testing.T) {
	device, queue := newMetalTextureCopyTestDevice(t)
	for _, test := range []struct {
		name                string
		source, destination gputypes.TextureDimension
	}{
		{name: "3D to 3D", source: gputypes.TextureDimension3D, destination: gputypes.TextureDimension3D},
		{name: "3D to 2D array", source: gputypes.TextureDimension3D, destination: gputypes.TextureDimension2D},
		{name: "2D array to 3D", source: gputypes.TextureDimension2D, destination: gputypes.TextureDimension3D},
	} {
		t.Run(test.name, func(t *testing.T) {
			const width, height, depth, bytesPerRow, offset = uint32(4), uint32(2), uint32(3), uint32(256), uint64(16)
			const rowsPerImage = uint32(3)
			const bytesPerImage = bytesPerRow * rowsPerImage
			data := metalTextureArrayTestData(width, height, depth, bytesPerRow, bytesPerImage)
			upload := createMetalTextureCopyTestUpload(t, device, offset, data)
			defer upload.Destroy()
			source := createMetalTextureCopyTestTexture(t, device, test.source, width+3, height+2, depth+2)
			defer source.Destroy()
			destination := createMetalTextureCopyTestTexture(t, device, test.destination, width+3, height+2, depth+2)
			defer destination.Destroy()
			readback := createMetalTextureCopyTestReadback(t, device, offset+uint64(len(data)))
			defer readback.Destroy()

			layout := hal.ImageDataLayout{Offset: offset, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
			size := hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: depth}
			sourceOrigin := hal.Origin3D{X: 1, Y: 1, Z: 1}
			destinationOrigin := hal.Origin3D{X: 2, Y: 1, Z: 1}
			encoder := &CommandEncoder{device: device}
			if err := encoder.BeginEncoding(test.name); err != nil {
				t.Fatalf("BeginEncoding failed: %v", err)
			}
			encoder.CopyBufferToTexture(upload, source, []hal.BufferTextureCopy{{BufferLayout: layout, TextureBase: hal.ImageCopyTexture{Texture: source, Origin: sourceOrigin}, Size: size}})
			encoder.CopyTextureToTexture(source, destination, []hal.TextureCopy{{SrcBase: hal.ImageCopyTexture{Texture: source, Origin: sourceOrigin}, DstBase: hal.ImageCopyTexture{Texture: destination, Origin: destinationOrigin}, Size: size}})
			encoder.CopyTextureToBuffer(destination, readback, []hal.BufferTextureCopy{{BufferLayout: layout, TextureBase: hal.ImageCopyTexture{Texture: destination, Origin: destinationOrigin}, Size: size}})
			submitMetalTextureCopyTestEncoder(t, device, queue, encoder)

			got := unsafe.Slice((*byte)(readback.Contents()), int(offset)+len(data))[offset:]
			assertMetalTextureArrayTestData(t, got, data, width, height, depth, bytesPerRow, bytesPerImage)
		})
	}
}

func TestQueueWriteTextureLayeredShapes(t *testing.T) {
	device, queue := newMetalTextureCopyTestDevice(t)
	hasUnifiedMemory := device.hasUnifiedMemory

	shapes := []struct {
		name      string
		dimension gputypes.TextureDimension
		height    uint32
	}{
		{name: "1D array", dimension: gputypes.TextureDimension1D, height: 1},
		{name: "2D array", dimension: gputypes.TextureDimension2D, height: 2},
		{name: "true 3D", dimension: gputypes.TextureDimension3D, height: 2},
	}
	storageModes := []struct {
		name   string
		shared bool
	}{
		{name: "private staging", shared: false},
		{name: "shared direct", shared: true},
	}
	for _, shape := range shapes {
		for _, storage := range storageModes {
			t.Run(shape.name+"/"+storage.name, func(t *testing.T) {
				if storage.shared && !hasUnifiedMemory {
					t.Skip("direct Shared-texture writes require unified memory")
				}
				device.hasUnifiedMemory = storage.shared

				const (
					width       = uint32(4)
					layers      = uint32(3)
					bytesPerRow = uint32(256)
					offset      = uint64(16)
				)
				rowsPerImage := shape.height + 1
				bytesPerImage := bytesPerRow * rowsPerImage
				expected := metalTextureArrayTestData(width, shape.height, layers, bytesPerRow, bytesPerImage)
				data := make([]byte, offset+uint64(len(expected)))
				copy(data[offset:], expected)

				texture := createMetalTextureCopyTestTexture(t, device, shape.dimension, width, shape.height, layers)
				defer texture.Destroy()
				if texture.isShared != storage.shared {
					t.Fatalf("texture shared mode = %t; want %t", texture.isShared, storage.shared)
				}
				readback := createMetalTextureCopyTestReadback(t, device, uint64(len(expected)))
				defer readback.Destroy()

				layout := &hal.ImageDataLayout{Offset: offset, BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
				size := &hal.Extent3D{Width: width, Height: shape.height, DepthOrArrayLayers: layers}
				requiredBytes := offset + uint64(layers-1)*uint64(bytesPerImage) + uint64(shape.height-1)*uint64(bytesPerRow) + uint64(width*4)
				if err := queue.WriteTexture(&hal.ImageCopyTexture{Texture: texture}, data[:requiredBytes-1], layout, size); err == nil {
					t.Fatal("WriteTexture accepted a source truncated in the final image")
				}
				if err := queue.WriteTexture(&hal.ImageCopyTexture{Texture: texture}, data, layout, size); err != nil {
					t.Fatalf("WriteTexture failed: %v", err)
				}

				encoder := &CommandEncoder{device: device}
				if err := encoder.BeginEncoding(shape.name + " WriteTexture readback"); err != nil {
					t.Fatalf("BeginEncoding failed: %v", err)
				}
				encoder.CopyTextureToBuffer(texture, readback, []hal.BufferTextureCopy{{
					BufferLayout: hal.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage},
					TextureBase:  hal.ImageCopyTexture{Texture: texture},
					Size:         *size,
				}})
				commandBuffer, err := encoder.EndEncoding()
				if err != nil {
					t.Fatalf("EndEncoding failed: %v", err)
				}
				defer commandBuffer.Destroy()
				if _, err := queue.Submit([]hal.CommandBuffer{commandBuffer}); err != nil {
					t.Fatalf("Submit failed: %v", err)
				}
				if err := device.WaitIdle(); err != nil {
					t.Fatalf("WaitIdle failed: %v", err)
				}

				got := unsafe.Slice((*byte)(readback.Contents()), len(expected))
				assertMetalTextureArrayTestData(t, got, expected, width, shape.height, layers, bytesPerRow, bytesPerImage)
			})
		}
	}
}

func newMetalTextureCopyTestDevice(t *testing.T) (*Device, *Queue) {
	t.Helper()
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	rawDevice := CreateSystemDefaultDevice()
	if rawDevice == 0 {
		t.Skip("no Metal device available")
	}
	t.Cleanup(func() { Release(rawDevice) })

	device, err := newDevice(&Adapter{raw: rawDevice})
	if err != nil {
		t.Fatalf("newDevice failed: %v", err)
	}
	t.Cleanup(device.Destroy)
	queue := &Queue{device: device, commandQueue: device.commandQueue}
	device.queue = queue
	return device, queue
}

func createMetalTextureCopyTestArray(t *testing.T, device *Device, width, height, layers uint32) *Texture {
	return createMetalTextureCopyTestTexture(t, device, gputypes.TextureDimension2D, width, height, layers)
}

func createMetalTextureCopyTestTexture(t *testing.T, device *Device, dimension gputypes.TextureDimension, width, height, depthOrLayers uint32) *Texture {
	t.Helper()
	raw, err := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: depthOrLayers},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     dimension,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture failed: %v", err)
	}
	return raw.(*Texture)
}

func createMetalTextureCopyTestReadback(t *testing.T, device *Device, size uint64) *Buffer {
	t.Helper()
	raw, err := device.CreateBuffer(&hal.BufferDescriptor{
		Size:  size,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(readback) failed: %v", err)
	}
	return raw.(*Buffer)
}

func createMetalTextureCopyTestUpload(t *testing.T, device *Device, offset uint64, data []byte) *Buffer {
	t.Helper()
	raw, err := device.CreateBuffer(&hal.BufferDescriptor{
		Size:  offset + uint64(len(data)),
		Usage: gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(upload) failed: %v", err)
	}
	upload := raw.(*Buffer)
	copy(unsafe.Slice((*byte)(upload.Contents()), int(offset)+len(data))[offset:], data)
	return upload
}

func submitMetalTextureCopyTestEncoder(t *testing.T, device *Device, queue *Queue, encoder *CommandEncoder) {
	t.Helper()
	commandBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}
	defer commandBuffer.Destroy()
	if _, err := queue.Submit([]hal.CommandBuffer{commandBuffer}); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}
}

func metalTextureArrayTestData(width, height, layers, bytesPerRow, bytesPerImage uint32) []byte {
	data := make([]byte, bytesPerImage*layers)
	for layer := uint32(0); layer < layers; layer++ {
		for row := uint32(0); row < height; row++ {
			for column := uint32(0); column < width; column++ {
				offset := layer*bytesPerImage + row*bytesPerRow + column*4
				data[offset+0] = byte(10 + layer)
				data[offset+1] = byte(20 + row)
				data[offset+2] = byte(30 + column)
				data[offset+3] = 255
			}
		}
	}
	return data
}

func assertMetalTextureArrayTestData(t *testing.T, got, want []byte, width, height, layers, bytesPerRow, bytesPerImage uint32) {
	t.Helper()
	for layer := uint32(0); layer < layers; layer++ {
		for row := uint32(0); row < height; row++ {
			rowStart := layer*bytesPerImage + row*bytesPerRow
			rowEnd := rowStart + width*4
			for index := rowStart; index < rowEnd; index++ {
				if got[index] != want[index] {
					t.Fatalf("layer %d row %d byte %d = %d; want %d", layer, row, index-rowStart, got[index], want[index])
				}
			}
		}
	}
}
