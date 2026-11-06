// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// errNoCompressedTextures is returned when a compressed texture is created on
// a context without immutable storage and compressed uploads. The adapter
// offers no compression feature there (hasCompressedTexturePath), so core
// refuses such a texture first.
var errNoCompressedTextures = errors.New("gles: compressed textures need glTexStorage2D/3D and glCompressedTexSubImage2D/3D, which this context lacks")

// allocateCompressedStorage allocates immutable storage for every level of a
// compressed texture bound to target, as Rust wgpu-hal gles/device.rs
// create_texture does with tex_storage_2d / tex_storage_3d. glTexImage takes
// no compressed format, so a compressed texture has no mutable-storage path;
// uncompressed textures keep glTexImage (see CreateTexture).
func allocateCompressedStorage(ctx *gl.Context, target, internalFormat uint32, size hal.Extent3D, levels uint32) error {
	if !ctx.HasCompressedTextures() {
		return errNoCompressedTextures
	}
	levelCount := int32(max(levels, 1))
	w, h := int32(size.Width), int32(size.Height)
	switch target {
	case gl.TEXTURE_2D, gl.TEXTURE_CUBE_MAP:
		ctx.TexStorage2D(target, levelCount, internalFormat, w, h)
	case gl.TEXTURE_3D, gl.TEXTURE_2D_ARRAY:
		ctx.TexStorage3D(target, levelCount, internalFormat, w, h, int32(size.DepthOrArrayLayers))
	default:
		return fmt.Errorf("gles: a compressed texture cannot have target %#x", target)
	}
	return nil
}

// compressedCopy is one upload of whole blocks into a compressed texture, in
// WebGPU's buffer layout: block rows bytesPerRow apart, images rowsPerImage
// block rows apart.
type compressedCopy struct {
	tex          *Texture
	mipLevel     uint32
	origin       [3]uint32 // x, y in texels; z is the depth slice or first layer
	size         [3]uint32 // width, height in texels; depth or layer count
	bytesPerRow  uint32    // 0: tightly packed block rows
	rowsPerImage uint32    // in block rows; 0: the copy's block rows
}

// compressedLayout is the block geometry of a compressedCopy.
type compressedLayout struct {
	blockHeight uint32
	blockRows   uint32 // block rows of one image of the copy
	rowBytes    uint64 // bytes of one block row of the copy
	stride      uint64 // bytes between block rows in the source
	imageBytes  uint64 // bytes between images in the source
}

func (c *compressedCopy) layout() compressedLayout {
	format := c.tex.format
	bw, bh := format.BlockDimensions()
	blocksWide := (c.size[0] + bw - 1) / bw
	blockRows := (c.size[1] + bh - 1) / bh
	rowBytes := uint64(blocksWide) * uint64(format.BlockCopySize())
	stride := max(uint64(c.bytesPerRow), rowBytes)
	return compressedLayout{
		blockHeight: bh,
		blockRows:   blockRows,
		rowBytes:    rowBytes,
		stride:      stride,
		imageBytes:  stride * uint64(max(c.rowsPerImage, blockRows)),
	}
}

// required returns the bytes the copy reads past its first byte.
func (l compressedLayout) required(layers uint32) uint64 {
	return uint64(layers-1)*l.imageBytes + uint64(l.blockRows-1)*l.stride + l.rowBytes
}

// upload writes the copy with glCompressedTexSubImage2D/3D, source(offset)
// giving the data pointer of the byte at offset: client memory, or a byte
// offset into the bound PIXEL_UNPACK_BUFFER. The texture must be bound to its
// target.
//
// Rust wgpu-hal (gles/queue.rs CopyBufferToTexture) uploads every image in one
// call of bytes_per_row * rows bytes and relies on UNPACK_ROW_LENGTH and
// UNPACK_IMAGE_HEIGHT. GL ignores the unpack state for compressed data unless
// UNPACK_COMPRESSED_BLOCK_* are set (desktop GL only, ES has none), and
// imageSize must equal the size of the written blocks, so a padded row pitch
// (256-byte aligned buffer copies) is an INVALID_VALUE there. We upload a
// tightly packed image in one call, as Rust does, and an image with padded
// rows one block row per call, which needs no unpack state on GL or ES.
func (c *compressedCopy) upload(ctx *gl.Context, internalFormat uint32, source func(offset uint64) unsafe.Pointer) {
	l := c.layout()
	if c.size[0] == 0 || c.size[1] == 0 || l.blockRows == 0 {
		return
	}
	rowsPerCall, callsPerImage := l.blockRows, uint32(1)
	if l.stride != l.rowBytes {
		rowsPerCall, callsPerImage = 1, l.blockRows
	}
	for z := uint32(0); z < max(c.size[2], 1); z++ {
		for call := uint32(0); call < callsPerImage; call++ {
			y := call * rowsPerCall * l.blockHeight
			height := min(rowsPerCall*l.blockHeight, c.size[1]-y)
			offset := uint64(z)*l.imageBytes + uint64(call)*l.stride
			c.writeBlocks(ctx, internalFormat, z, y, height,
				int32(uint64(rowsPerCall)*l.rowBytes), source(offset))
		}
	}
}

// writeBlocks writes one rectangle of blocks of layer z of the copy, starting
// y texels below the copy's origin.
func (c *compressedCopy) writeBlocks(ctx *gl.Context, internalFormat, z, y, height uint32, imageSize int32, data unsafe.Pointer) {
	target := c.tex.target
	if isLayeredTarget(target) {
		ctx.CompressedTexSubImage3D(target, int32(c.mipLevel),
			int32(c.origin[0]), int32(c.origin[1]+y), int32(c.origin[2]+z),
			int32(c.size[0]), int32(height), 1, internalFormat, imageSize, data)
		return
	}
	ctx.CompressedTexSubImage2D(get2DTarget(target, c.origin[2]+z), int32(c.mipLevel),
		int32(c.origin[0]), int32(c.origin[1]+y), int32(c.size[0]), int32(height),
		internalFormat, imageSize, data)
}

// writeCompressedTexture uploads data to a compressed texture bound to its
// target (Queue.WriteTexture).
func writeCompressedTexture(ctx *gl.Context, tex *Texture, internalFormat uint32, dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	c := compressedCopy{
		tex:          tex,
		mipLevel:     dst.MipLevel,
		origin:       [3]uint32{dst.Origin.X, dst.Origin.Y, dst.Origin.Z},
		size:         [3]uint32{size.Width, size.Height, size.DepthOrArrayLayers},
		bytesPerRow:  layout.BytesPerRow,
		rowsPerImage: layout.RowsPerImage,
	}
	if size.Width == 0 || size.Height == 0 {
		return nil
	}
	if need := layout.Offset + c.layout().required(max(size.DepthOrArrayLayers, 1)); need > uint64(len(data)) {
		return fmt.Errorf("gles: WriteTexture: layout needs %d bytes, data has %d", need, len(data))
	}
	c.upload(ctx, internalFormat, func(offset uint64) unsafe.Pointer {
		return unsafe.Pointer(&data[layout.Offset+offset])
	})
	return nil
}
