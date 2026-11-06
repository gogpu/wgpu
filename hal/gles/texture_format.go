// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// textureFormatToGL converts a WebGPU texture format to its GL internal
// format and its pixel transfer format and type (Rust wgpu-hal
// gles/conv.rs describe_texture_format).
func textureFormatToGL(format gputypes.TextureFormat) (internalFormat, dataFormat, dataType uint32) {
	switch format {
	case gputypes.TextureFormatR8Unorm:
		return gl.R8, gl.RED, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatRG8Unorm:
		return gl.RG8, gl.RG, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatRGBA8Unorm:
		return gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatRGBA8UnormSrgb:
		return gl.SRGB8_ALPHA8, gl.RGBA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatBGRA8Unorm:
		return gl.RGBA8, gl.BGRA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatBGRA8UnormSrgb:
		return gl.SRGB8_ALPHA8, gl.BGRA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatR16Float:
		return gl.R16F, gl.RED, gl.HALF_FLOAT
	case gputypes.TextureFormatRG16Float:
		return gl.RG16F, gl.RG, gl.HALF_FLOAT
	case gputypes.TextureFormatRGBA16Float:
		return gl.RGBA16F, gl.RGBA, gl.HALF_FLOAT
	case gputypes.TextureFormatR32Float:
		return gl.R32F, gl.RED, gl.FLOAT
	case gputypes.TextureFormatRG32Float:
		return gl.RG32F, gl.RG, gl.FLOAT
	case gputypes.TextureFormatRGBA32Float:
		return gl.RGBA32F, gl.RGBA, gl.FLOAT
	case gputypes.TextureFormatStencil8:
		return gl.STENCIL_INDEX8, gl.STENCIL_INDEX, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatDepth16Unorm:
		return gl.DEPTH_COMPONENT16, gl.DEPTH_COMPONENT, gl.UNSIGNED_SHORT
	case gputypes.TextureFormatDepth24Plus:
		return gl.DEPTH_COMPONENT24, gl.DEPTH_COMPONENT, gl.UNSIGNED_INT
	case gputypes.TextureFormatDepth24PlusStencil8:
		return gl.DEPTH24_STENCIL8, gl.DEPTH_STENCIL, gl.UNSIGNED_INT_24_8
	case gputypes.TextureFormatDepth32Float:
		// DEPTH_COMPONENT32 is a 32-bit normalized format of desktop GL that
		// ES lacks; depth32float is a float format.
		return gl.DEPTH_COMPONENT32F, gl.DEPTH_COMPONENT, gl.FLOAT
	case gputypes.TextureFormatDepth32FloatStencil8:
		// DEPTH_STENCIL takes UNSIGNED_INT_24_8 or
		// FLOAT_32_UNSIGNED_INT_24_8_REV; FLOAT is an invalid operation.
		return gl.DEPTH32F_STENCIL8, gl.DEPTH_STENCIL, gl.FLOAT_32_UNSIGNED_INT_24_8_REV
	default:
		// Default to RGBA8
		return gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE
	}
}

// uploadTexelBytes returns the size of one texel that glTexSubImage reads
// for a pixel transfer format and type: a color pixel (glPixelBytes), a
// depth value or a stencil index. It returns 0 for a combined depth/stencil
// transfer, whose buffer layout (one aspect) differs from what GL reads.
func uploadTexelBytes(dataFormat, dataType uint32) uint64 {
	if bpp := glPixelBytes(dataFormat, dataType); bpp != 0 {
		return uint64(bpp)
	}
	if dataFormat != gl.DEPTH_COMPONENT && dataFormat != gl.STENCIL_INDEX {
		return 0
	}
	switch dataType {
	case gl.UNSIGNED_BYTE:
		return 1
	case gl.UNSIGNED_SHORT:
		return 2
	case gl.UNSIGNED_INT, gl.FLOAT:
		return 4
	}
	return 0
}
