// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package texcompress decodes single texels of the block-compressed texture
// formats the software backend samples: BC1-BC7 (texture-compression-bc) and
// ETC2/EAC (texture-compression-etc2).
//
// A texel is decoded from its own 4x4 block only, reading just the fields
// that texel needs, so a texture fetch costs one partial block decode and no
// decoded copy of the texture exists.
//
// The arithmetic is the one GPUs use, not only the one the format
// specifications allow: the Khronos Data Format Specification defines the
// BC1-BC5 palettes in real numbers, and hardware quantizes them. Decode
// returns what textureLoad returns on GPUs (see testdata/gpu_readback.txt,
// against which every format is tested):
//   - BC1-BC3 colors and BC3 alpha interpolate the 8-bit endpoints with
//     weights of 256, round(256k/n), rounding to an 8-bit value (Mesa's s3tc
//     decoder divides exactly and truncates; the two differ by one code).
//   - BC4 and BC5 interpolate with weights of 65536, round(65536k/n) per
//     endpoint, to an 8.8 fixed-point value, widened by bit replication to
//     16 bits of 65535 (unorm) or 15 bits of 32767 (snorm).
//   - BC6H and BC7 follow the bit-exact integer decoding of the format
//     specification (as Mesa's texcompress_bptc_tmp.h implements it).
//   - ETC2 and EAC are integer-exact by specification.
//
// The values of an sRGB format are its encoded values: the caller applies
// the sRGB transfer function after the block decode.
package texcompress

import "github.com/gogpu/gputypes"

// Decode returns texel (x, y) of block, one 4x4 block of format f, with
// 0 <= x, y < 4. The result is RGBA, the channels f lacks filled from
// (0, 0, 0, 1). block must hold f's block size (8 or 16 bytes). ok is false
// for a format this package does not decode.
func Decode(f gputypes.TextureFormat, block []byte, x, y int) (rgba [4]float32, ok bool) {
	switch f {
	case gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb:
		return unorm8(decodeBC1(block, x, y, false)), true
	case gputypes.TextureFormatBC2RGBAUnorm, gputypes.TextureFormatBC2RGBAUnormSrgb:
		return unorm8(decodeBC2(block, x, y)), true
	case gputypes.TextureFormatBC3RGBAUnorm, gputypes.TextureFormatBC3RGBAUnormSrgb:
		return unorm8(decodeBC3(block, x, y)), true
	case gputypes.TextureFormatBC4RUnorm:
		return [4]float32{bc4Unorm(block, x, y), 0, 0, 1}, true
	case gputypes.TextureFormatBC4RSnorm:
		return [4]float32{bc4Snorm(block, x, y), 0, 0, 1}, true
	case gputypes.TextureFormatBC5RGUnorm:
		return [4]float32{bc4Unorm(block, x, y), bc4Unorm(block[8:], x, y), 0, 1}, true
	case gputypes.TextureFormatBC5RGSnorm:
		return [4]float32{bc4Snorm(block, x, y), bc4Snorm(block[8:], x, y), 0, 1}, true
	case gputypes.TextureFormatBC6HRGBUfloat:
		return decodeBC6H(block, x, y, false), true
	case gputypes.TextureFormatBC6HRGBFloat:
		return decodeBC6H(block, x, y, true), true
	case gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb:
		return unorm8(decodeBC7(block, x, y)), true
	}
	return decodeETC(f, block, x, y)
}

// Decodes reports whether Decode decodes format f.
func Decodes(f gputypes.TextureFormat) bool {
	switch f {
	case gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb,
		gputypes.TextureFormatBC2RGBAUnorm, gputypes.TextureFormatBC2RGBAUnormSrgb,
		gputypes.TextureFormatBC3RGBAUnorm, gputypes.TextureFormatBC3RGBAUnormSrgb,
		gputypes.TextureFormatBC4RUnorm, gputypes.TextureFormatBC4RSnorm,
		gputypes.TextureFormatBC5RGUnorm, gputypes.TextureFormatBC5RGSnorm,
		gputypes.TextureFormatBC6HRGBUfloat, gputypes.TextureFormatBC6HRGBFloat,
		gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb,
		gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb,
		gputypes.TextureFormatETC2RGB8A1Unorm, gputypes.TextureFormatETC2RGB8A1UnormSrgb,
		gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb,
		gputypes.TextureFormatEACR11Unorm, gputypes.TextureFormatEACR11Snorm,
		gputypes.TextureFormatEACRG11Unorm, gputypes.TextureFormatEACRG11Snorm:
		return true
	}
	return false
}

// unorm8 converts 8-bit unorm channels to floats.
func unorm8(c [4]uint8) [4]float32 {
	return [4]float32{float32(c[0]) / 255, float32(c[1]) / 255, float32(c[2]) / 255, float32(c[3]) / 255}
}
