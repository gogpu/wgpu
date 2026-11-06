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

// errNoLayeredTextures is returned when a 3D or 2D array texture is created
// on a context without the GL 3.0 / ES 3.0 functions such textures need.
var errNoLayeredTextures = errors.New("gles: 3D and 2D array textures need glTexImage3D, glTexSubImage3D and glFramebufferTextureLayer, which this context lacks")

// isLayeredTarget reports whether a texture target holds layers that are
// attached, written and read one at a time (Rust wgpu-hal
// gles/conv.rs is_layered_target, without the cube map array).
func isLayeredTarget(target uint32) bool {
	return target == gl.TEXTURE_3D || target == gl.TEXTURE_2D_ARRAY
}

// layeredLevelSize returns the width, height and depth (3D) or layer count
// (2D array) of one mip level. A 3D level halves its depth with its width and
// height; an array keeps its layer count at every level.
func layeredLevelSize(target uint32, size hal.Extent3D, level uint32) (w, h, d int32) {
	w = maxInt32(1, int32(size.Width>>level))
	h = maxInt32(1, int32(size.Height>>level))
	d = int32(size.DepthOrArrayLayers)
	if target == gl.TEXTURE_3D {
		d = maxInt32(1, int32(size.DepthOrArrayLayers>>level))
	}
	return w, h, d
}

// writeLayeredTexture uploads data to a box of a 3D or 2D array texture, which
// must be bound to its target: the box starts at dst's mip level and origin,
// whose Z is the depth slice or the first array layer. Rows are
// layout.BytesPerRow apart and layers layout.RowsPerImage rows apart, as Rust
// wgpu-hal's gles/queue.rs uploads them (UNPACK_ROW_LENGTH,
// UNPACK_IMAGE_HEIGHT, tex_sub_image_3d).
func writeLayeredTexture(ctx *gl.Context, tex *Texture, dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D, format, dataType uint32) error {
	bpp := uint64(glPixelBytes(format, dataType))
	if bpp == 0 {
		return fmt.Errorf("gles: WriteTexture: no texel size for format %v", tex.format)
	}
	rowBytes := uint64(size.Width) * bpp
	stride := max(uint64(layout.BytesPerRow), rowBytes)
	rows := max(uint64(layout.RowsPerImage), uint64(size.Height))
	depth := max(uint64(size.DepthOrArrayLayers), 1)
	if size.Width == 0 || size.Height == 0 {
		return nil
	}
	required := layout.Offset + (depth-1)*stride*rows + (uint64(size.Height)-1)*stride + rowBytes
	if required > uint64(len(data)) {
		return fmt.Errorf("gles: WriteTexture: layout needs %d bytes, data has %d", required, len(data))
	}

	ctx.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	ctx.PixelStorei(gl.UNPACK_ROW_LENGTH, int32(stride/bpp))
	ctx.PixelStorei(gl.UNPACK_IMAGE_HEIGHT, int32(rows))
	ctx.TexSubImage3D(tex.target, int32(dst.MipLevel),
		int32(dst.Origin.X), int32(dst.Origin.Y), int32(dst.Origin.Z),
		int32(size.Width), int32(size.Height), int32(depth),
		format, dataType, unsafe.Pointer(&data[layout.Offset]))
	ctx.PixelStorei(gl.UNPACK_IMAGE_HEIGHT, 0)
	ctx.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	ctx.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	return nil
}

// colorAttachmentLayer returns the layer a color attachment renders into: the
// pass's depthSlice for a 3D texture, the view's base array layer for a 2D
// array texture (Rust wgpu-hal gles/command.rs begin_render_pass and
// queue.rs set_attachment).
func colorAttachmentLayer(view *TextureView, depthSlice *uint32) uint32 {
	if view.texture.target == gl.TEXTURE_3D {
		if depthSlice != nil {
			return *depthSlice
		}
		return 0
	}
	return view.baseLayer
}

// attachColorLayer attaches color attachment index of the bound framebuffer
// to one mip level of tex and, for a layered texture, to one layer of it: the
// depth slice of a 3D texture or an array layer. Rust wgpu-hal gles/queue.rs
// set_attachment does the same.
func attachColorLayer(ctx *gl.Context, fboTarget, attachment uint32, tex *Texture, mip, layer uint32) {
	if isLayeredTarget(tex.target) {
		ctx.FramebufferTextureLayer(fboTarget, attachment, tex.id, int32(mip), int32(layer))
		return
	}
	ctx.FramebufferTexture2D(fboTarget, attachment, tex.target, tex.id, int32(mip))
}
