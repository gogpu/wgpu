// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// glTextureFormat is how GL stores one WebGPU texture format and how pixels
// of it are transferred (Rust wgpu-hal gles TextureFormatDesc).
type glTextureFormat struct {
	// internal is the sized internal format of glTexImage, glTexStorage,
	// glRenderbufferStorage and glBindImageTexture.
	internal uint32
	// external is the pixel transfer format of glTexSubImage and glReadPixels.
	external uint32
	// dataType is the pixel transfer type.
	dataType uint32
}

// glTextureFormats is Rust wgpu-hal gles/conv.rs describe_texture_format,
// row by row. A format missing here has no GL texture: describeTextureFormat
// reports it, so nothing is created or uploaded as a different format.
var glTextureFormats = map[gputypes.TextureFormat]glTextureFormat{
	gputypes.TextureFormatR8Unorm:  {gl.R8, gl.RED, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatR8Snorm:  {gl.R8_SNORM, gl.RED, gl.BYTE},
	gputypes.TextureFormatR8Uint:   {gl.R8UI, gl.RED_INTEGER, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatR8Sint:   {gl.R8I, gl.RED_INTEGER, gl.BYTE},
	gputypes.TextureFormatR16Uint:  {gl.R16UI, gl.RED_INTEGER, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatR16Sint:  {gl.R16I, gl.RED_INTEGER, gl.SHORT},
	gputypes.TextureFormatR16Unorm: {gl.R16, gl.RED, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatR16Snorm: {gl.R16_SNORM, gl.RED, gl.SHORT},
	gputypes.TextureFormatR16Float: {gl.R16F, gl.RED, gl.HALF_FLOAT},

	gputypes.TextureFormatRG8Unorm: {gl.RG8, gl.RG, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatRG8Snorm: {gl.RG8_SNORM, gl.RG, gl.BYTE},
	gputypes.TextureFormatRG8Uint:  {gl.RG8UI, gl.RG_INTEGER, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatRG8Sint:  {gl.RG8I, gl.RG_INTEGER, gl.BYTE},

	gputypes.TextureFormatR32Uint:   {gl.R32UI, gl.RED_INTEGER, gl.UNSIGNED_INT},
	gputypes.TextureFormatR32Sint:   {gl.R32I, gl.RED_INTEGER, gl.INT},
	gputypes.TextureFormatR32Float:  {gl.R32F, gl.RED, gl.FLOAT},
	gputypes.TextureFormatRG16Uint:  {gl.RG16UI, gl.RG_INTEGER, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatRG16Sint:  {gl.RG16I, gl.RG_INTEGER, gl.SHORT},
	gputypes.TextureFormatRG16Unorm: {gl.RG16, gl.RG, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatRG16Snorm: {gl.RG16_SNORM, gl.RG, gl.SHORT},
	gputypes.TextureFormatRG16Float: {gl.RG16F, gl.RG, gl.HALF_FLOAT},

	gputypes.TextureFormatRGBA8Unorm:     {gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatRGBA8UnormSrgb: {gl.SRGB8_ALPHA8, gl.RGBA, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatBGRA8UnormSrgb: {gl.SRGB8_ALPHA8, gl.BGRA, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatRGBA8Snorm:     {gl.RGBA8_SNORM, gl.RGBA, gl.BYTE},
	gputypes.TextureFormatBGRA8Unorm:     {gl.RGBA8, gl.BGRA, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatRGBA8Uint:      {gl.RGBA8UI, gl.RGBA_INTEGER, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatRGBA8Sint:      {gl.RGBA8I, gl.RGBA_INTEGER, gl.BYTE},

	gputypes.TextureFormatRGB10A2Uint:   {gl.RGB10_A2UI, gl.RGBA_INTEGER, gl.UNSIGNED_INT_2_10_10_10_REV},
	gputypes.TextureFormatRGB10A2Unorm:  {gl.RGB10_A2, gl.RGBA, gl.UNSIGNED_INT_2_10_10_10_REV},
	gputypes.TextureFormatRG11B10Ufloat: {gl.R11F_G11F_B10F, gl.RGB, gl.UNSIGNED_INT_10F_11F_11F_REV},
	gputypes.TextureFormatRGB9E5Ufloat:  {gl.RGB9_E5, gl.RGB, gl.UNSIGNED_INT_5_9_9_9_REV},
	gputypes.TextureFormatRG32Uint:      {gl.RG32UI, gl.RG_INTEGER, gl.UNSIGNED_INT},
	gputypes.TextureFormatRG32Sint:      {gl.RG32I, gl.RG_INTEGER, gl.INT},
	gputypes.TextureFormatRG32Float:     {gl.RG32F, gl.RG, gl.FLOAT},
	gputypes.TextureFormatRGBA16Uint:    {gl.RGBA16UI, gl.RGBA_INTEGER, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatRGBA16Sint:    {gl.RGBA16I, gl.RGBA_INTEGER, gl.SHORT},
	gputypes.TextureFormatRGBA16Unorm:   {gl.RGBA16, gl.RGBA, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatRGBA16Snorm:   {gl.RGBA16_SNORM, gl.RGBA, gl.SHORT},
	gputypes.TextureFormatRGBA16Float:   {gl.RGBA16F, gl.RGBA, gl.HALF_FLOAT},
	gputypes.TextureFormatRGBA32Uint:    {gl.RGBA32UI, gl.RGBA_INTEGER, gl.UNSIGNED_INT},
	gputypes.TextureFormatRGBA32Sint:    {gl.RGBA32I, gl.RGBA_INTEGER, gl.INT},
	gputypes.TextureFormatRGBA32Float:   {gl.RGBA32F, gl.RGBA, gl.FLOAT},
	gputypes.TextureFormatStencil8:      {gl.STENCIL_INDEX8, gl.STENCIL_INDEX, gl.UNSIGNED_BYTE},
	gputypes.TextureFormatDepth16Unorm:  {gl.DEPTH_COMPONENT16, gl.DEPTH_COMPONENT, gl.UNSIGNED_SHORT},
	gputypes.TextureFormatDepth24Plus:   {gl.DEPTH_COMPONENT24, gl.DEPTH_COMPONENT, gl.UNSIGNED_INT},
	gputypes.TextureFormatDepth32Float:  {gl.DEPTH_COMPONENT32F, gl.DEPTH_COMPONENT, gl.FLOAT},
	gputypes.TextureFormatDepth24PlusStencil8: {
		gl.DEPTH24_STENCIL8, gl.DEPTH_STENCIL, gl.UNSIGNED_INT_24_8,
	},
	gputypes.TextureFormatDepth32FloatStencil8: {
		gl.DEPTH32F_STENCIL8, gl.DEPTH_STENCIL, gl.FLOAT_32_UNSIGNED_INT_24_8_REV,
	},

	// Compressed formats carry no pixel transfer type: they are allocated with
	// glTexStorage and written with glCompressedTexSubImage (compressed_texture.go).
	gputypes.TextureFormatBC1RGBAUnorm:     {gl.COMPRESSED_RGBA_S3TC_DXT1_EXT, gl.RGBA, 0},
	gputypes.TextureFormatBC1RGBAUnormSrgb: {gl.COMPRESSED_SRGB_ALPHA_S3TC_DXT1_EXT, gl.RGBA, 0},
	gputypes.TextureFormatBC2RGBAUnorm:     {gl.COMPRESSED_RGBA_S3TC_DXT3_EXT, gl.RGBA, 0},
	gputypes.TextureFormatBC2RGBAUnormSrgb: {gl.COMPRESSED_SRGB_ALPHA_S3TC_DXT3_EXT, gl.RGBA, 0},
	gputypes.TextureFormatBC3RGBAUnorm:     {gl.COMPRESSED_RGBA_S3TC_DXT5_EXT, gl.RGBA, 0},
	gputypes.TextureFormatBC3RGBAUnormSrgb: {gl.COMPRESSED_SRGB_ALPHA_S3TC_DXT5_EXT, gl.RGBA, 0},
	gputypes.TextureFormatBC4RUnorm:        {gl.COMPRESSED_RED_RGTC1, gl.RED, 0},
	gputypes.TextureFormatBC4RSnorm:        {gl.COMPRESSED_SIGNED_RED_RGTC1, gl.RED, 0},
	gputypes.TextureFormatBC5RGUnorm:       {gl.COMPRESSED_RG_RGTC2, gl.RG, 0},
	gputypes.TextureFormatBC5RGSnorm:       {gl.COMPRESSED_SIGNED_RG_RGTC2, gl.RG, 0},
	gputypes.TextureFormatBC6HRGBUfloat:    {gl.COMPRESSED_RGB_BPTC_UNSIGNED_FLOAT, gl.RGB, 0},
	gputypes.TextureFormatBC6HRGBFloat:     {gl.COMPRESSED_RGB_BPTC_SIGNED_FLOAT, gl.RGB, 0},
	gputypes.TextureFormatBC7RGBAUnorm:     {gl.COMPRESSED_RGBA_BPTC_UNORM, gl.RGBA, 0},
	gputypes.TextureFormatBC7RGBAUnormSrgb: {gl.COMPRESSED_SRGB_ALPHA_BPTC_UNORM, gl.RGBA, 0},

	gputypes.TextureFormatETC2RGB8Unorm:       {gl.COMPRESSED_RGB8_ETC2, gl.RGB, 0},
	gputypes.TextureFormatETC2RGB8UnormSrgb:   {gl.COMPRESSED_SRGB8_ETC2, gl.RGB, 0},
	gputypes.TextureFormatETC2RGB8A1Unorm:     {gl.COMPRESSED_RGB8_PUNCHTHROUGH_ALPHA1_ETC2, gl.RGBA, 0},
	gputypes.TextureFormatETC2RGB8A1UnormSrgb: {gl.COMPRESSED_SRGB8_PUNCHTHROUGH_ALPHA1_ETC2, gl.RGBA, 0},
	gputypes.TextureFormatETC2RGBA8Unorm:      {gl.COMPRESSED_RGBA8_ETC2_EAC, gl.RGBA, 0},
	gputypes.TextureFormatETC2RGBA8UnormSrgb:  {gl.COMPRESSED_SRGB8_ALPHA8_ETC2_EAC, gl.RGBA, 0},
	gputypes.TextureFormatEACR11Unorm:         {gl.COMPRESSED_R11_EAC, gl.RED, 0},
	gputypes.TextureFormatEACR11Snorm:         {gl.COMPRESSED_SIGNED_R11_EAC, gl.RED, 0},
	gputypes.TextureFormatEACRG11Unorm:        {gl.COMPRESSED_RG11_EAC, gl.RG, 0},
	gputypes.TextureFormatEACRG11Snorm:        {gl.COMPRESSED_SIGNED_RG11_EAC, gl.RG, 0},

	gputypes.TextureFormatASTC4x4Unorm:       {gl.COMPRESSED_RGBA_ASTC_4x4_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC4x4UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_4x4_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC5x4Unorm:       {gl.COMPRESSED_RGBA_ASTC_5x4_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC5x4UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_5x4_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC5x5Unorm:       {gl.COMPRESSED_RGBA_ASTC_5x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC5x5UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_5x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC6x5Unorm:       {gl.COMPRESSED_RGBA_ASTC_6x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC6x5UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_6x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC6x6Unorm:       {gl.COMPRESSED_RGBA_ASTC_6x6_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC6x6UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_6x6_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC8x5Unorm:       {gl.COMPRESSED_RGBA_ASTC_8x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC8x5UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_8x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC8x6Unorm:       {gl.COMPRESSED_RGBA_ASTC_8x6_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC8x6UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_8x6_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC8x8Unorm:       {gl.COMPRESSED_RGBA_ASTC_8x8_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC8x8UnormSrgb:   {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_8x8_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x5Unorm:      {gl.COMPRESSED_RGBA_ASTC_10x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x5UnormSrgb:  {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_10x5_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x6Unorm:      {gl.COMPRESSED_RGBA_ASTC_10x6_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x6UnormSrgb:  {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_10x6_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x8Unorm:      {gl.COMPRESSED_RGBA_ASTC_10x8_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x8UnormSrgb:  {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_10x8_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x10Unorm:     {gl.COMPRESSED_RGBA_ASTC_10x10_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC10x10UnormSrgb: {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_10x10_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC12x10Unorm:     {gl.COMPRESSED_RGBA_ASTC_12x10_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC12x10UnormSrgb: {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_12x10_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC12x12Unorm:     {gl.COMPRESSED_RGBA_ASTC_12x12_KHR, gl.RGBA, 0},
	gputypes.TextureFormatASTC12x12UnormSrgb: {gl.COMPRESSED_SRGB8_ALPHA8_ASTC_12x12_KHR, gl.RGBA, 0},
}

// UnsupportedTextureFormatError reports a texture format the GLES backend has
// no GL storage for.
type UnsupportedTextureFormatError struct {
	Format gputypes.TextureFormat
}

func (e *UnsupportedTextureFormatError) Error() string {
	return fmt.Sprintf("gles: texture format %v has no GL storage in this backend", e.Format)
}

// describeTextureFormat returns how GL stores and transfers format, or an
// *UnsupportedTextureFormatError. It never substitutes another format: a
// texture created as RGBA8 in place of an integer or packed format read back
// as zeros or as the wrong bytes.
func describeTextureFormat(format gputypes.TextureFormat) (glTextureFormat, error) {
	desc, ok := glTextureFormats[format]
	if !ok {
		return glTextureFormat{}, &UnsupportedTextureFormatError{Format: format}
	}
	return desc, nil
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
