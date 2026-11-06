// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// rustTextureFormats is Rust wgpu-hal gles/conv.rs describe_texture_format
// (wgpu 14328f4) for every uncompressed format gputypes has, with the enum
// values of the Khronos GL headers written out, so a wrong constant in the gl
// package fails here as well as a wrong row.
var rustTextureFormats = map[gputypes.TextureFormat][3]uint32{
	gputypes.TextureFormatR8Unorm:              {0x8229, 0x1903, 0x1401}, // R8, RED, UNSIGNED_BYTE
	gputypes.TextureFormatR8Snorm:              {0x8F94, 0x1903, 0x1400}, // R8_SNORM, RED, BYTE
	gputypes.TextureFormatR8Uint:               {0x8232, 0x8D94, 0x1401}, // R8UI, RED_INTEGER
	gputypes.TextureFormatR8Sint:               {0x8231, 0x8D94, 0x1400}, // R8I
	gputypes.TextureFormatR16Uint:              {0x8234, 0x8D94, 0x1403}, // R16UI, UNSIGNED_SHORT
	gputypes.TextureFormatR16Sint:              {0x8233, 0x8D94, 0x1402}, // R16I, SHORT
	gputypes.TextureFormatR16Unorm:             {0x822A, 0x1903, 0x1403}, // R16
	gputypes.TextureFormatR16Snorm:             {0x8F98, 0x1903, 0x1402}, // R16_SNORM
	gputypes.TextureFormatR16Float:             {0x822D, 0x1903, 0x140B}, // R16F, HALF_FLOAT
	gputypes.TextureFormatRG8Unorm:             {0x822B, 0x8227, 0x1401}, // RG8, RG
	gputypes.TextureFormatRG8Snorm:             {0x8F95, 0x8227, 0x1400}, // RG8_SNORM
	gputypes.TextureFormatRG8Uint:              {0x8238, 0x8228, 0x1401}, // RG8UI, RG_INTEGER
	gputypes.TextureFormatRG8Sint:              {0x8237, 0x8228, 0x1400}, // RG8I
	gputypes.TextureFormatR32Uint:              {0x8236, 0x8D94, 0x1405}, // R32UI, UNSIGNED_INT
	gputypes.TextureFormatR32Sint:              {0x8235, 0x8D94, 0x1404}, // R32I, INT
	gputypes.TextureFormatR32Float:             {0x822E, 0x1903, 0x1406}, // R32F, FLOAT
	gputypes.TextureFormatRG16Uint:             {0x823A, 0x8228, 0x1403}, // RG16UI
	gputypes.TextureFormatRG16Sint:             {0x8239, 0x8228, 0x1402}, // RG16I
	gputypes.TextureFormatRG16Unorm:            {0x822C, 0x8227, 0x1403}, // RG16
	gputypes.TextureFormatRG16Snorm:            {0x8F99, 0x8227, 0x1402}, // RG16_SNORM
	gputypes.TextureFormatRG16Float:            {0x822F, 0x8227, 0x140B}, // RG16F
	gputypes.TextureFormatRGBA8Unorm:           {0x8058, 0x1908, 0x1401}, // RGBA8, RGBA
	gputypes.TextureFormatRGBA8UnormSrgb:       {0x8C43, 0x1908, 0x1401}, // SRGB8_ALPHA8
	gputypes.TextureFormatBGRA8UnormSrgb:       {0x8C43, 0x80E1, 0x1401}, // SRGB8_ALPHA8, BGRA
	gputypes.TextureFormatRGBA8Snorm:           {0x8F97, 0x1908, 0x1400}, // RGBA8_SNORM
	gputypes.TextureFormatBGRA8Unorm:           {0x8058, 0x80E1, 0x1401}, // RGBA8, BGRA
	gputypes.TextureFormatRGBA8Uint:            {0x8D7C, 0x8D99, 0x1401}, // RGBA8UI, RGBA_INTEGER
	gputypes.TextureFormatRGBA8Sint:            {0x8D8E, 0x8D99, 0x1400}, // RGBA8I
	gputypes.TextureFormatRGB10A2Uint:          {0x906F, 0x8D99, 0x8368}, // RGB10_A2UI, UNSIGNED_INT_2_10_10_10_REV
	gputypes.TextureFormatRGB10A2Unorm:         {0x8059, 0x1908, 0x8368}, // RGB10_A2
	gputypes.TextureFormatRG11B10Ufloat:        {0x8C3A, 0x1907, 0x8C3B}, // R11F_G11F_B10F, RGB, UNSIGNED_INT_10F_11F_11F_REV
	gputypes.TextureFormatRGB9E5Ufloat:         {0x8C3D, 0x1907, 0x8C3E}, // RGB9_E5, RGB, UNSIGNED_INT_5_9_9_9_REV
	gputypes.TextureFormatRG32Uint:             {0x823C, 0x8228, 0x1405}, // RG32UI
	gputypes.TextureFormatRG32Sint:             {0x823B, 0x8228, 0x1404}, // RG32I
	gputypes.TextureFormatRG32Float:            {0x8230, 0x8227, 0x1406}, // RG32F
	gputypes.TextureFormatRGBA16Uint:           {0x8D76, 0x8D99, 0x1403}, // RGBA16UI
	gputypes.TextureFormatRGBA16Sint:           {0x8D88, 0x8D99, 0x1402}, // RGBA16I
	gputypes.TextureFormatRGBA16Unorm:          {0x805B, 0x1908, 0x1403}, // RGBA16
	gputypes.TextureFormatRGBA16Snorm:          {0x8F9B, 0x1908, 0x1402}, // RGBA16_SNORM
	gputypes.TextureFormatRGBA16Float:          {0x881A, 0x1908, 0x140B}, // RGBA16F
	gputypes.TextureFormatRGBA32Uint:           {0x8D70, 0x8D99, 0x1405}, // RGBA32UI
	gputypes.TextureFormatRGBA32Sint:           {0x8D82, 0x8D99, 0x1404}, // RGBA32I
	gputypes.TextureFormatRGBA32Float:          {0x8814, 0x1908, 0x1406}, // RGBA32F
	gputypes.TextureFormatStencil8:             {0x8D48, 0x1901, 0x1401}, // STENCIL_INDEX8, STENCIL_INDEX
	gputypes.TextureFormatDepth16Unorm:         {0x81A5, 0x1902, 0x1403}, // DEPTH_COMPONENT16, DEPTH_COMPONENT
	gputypes.TextureFormatDepth32Float:         {0x8CAC, 0x1902, 0x1406}, // DEPTH_COMPONENT32F
	gputypes.TextureFormatDepth32FloatStencil8: {0x8CAD, 0x84F9, 0x8DAD}, // DEPTH32F_STENCIL8, DEPTH_STENCIL, FLOAT_32_UNSIGNED_INT_24_8_REV
	gputypes.TextureFormatDepth24Plus:          {0x81A6, 0x1902, 0x1405}, // DEPTH_COMPONENT24
	gputypes.TextureFormatDepth24PlusStencil8:  {0x88F0, 0x84F9, 0x84FA}, // DEPTH24_STENCIL8, UNSIGNED_INT_24_8
}

// rustCompressedFormats are the compressed rows of Rust's
// describe_texture_format: the GL compressed internal format, the base format
// and no transfer type. ASTC HDR shares the LDR enums in Rust; gputypes has
// no HDR channel.
var rustCompressedFormats = map[gputypes.TextureFormat][3]uint32{
	gputypes.TextureFormatBC1RGBAUnorm:        {0x83F1, 0x1908, 0}, // COMPRESSED_RGBA_S3TC_DXT1_EXT, RGBA
	gputypes.TextureFormatBC1RGBAUnormSrgb:    {0x8C4D, 0x1908, 0}, // COMPRESSED_SRGB_ALPHA_S3TC_DXT1_EXT
	gputypes.TextureFormatBC2RGBAUnorm:        {0x83F2, 0x1908, 0},
	gputypes.TextureFormatBC2RGBAUnormSrgb:    {0x8C4E, 0x1908, 0},
	gputypes.TextureFormatBC3RGBAUnorm:        {0x83F3, 0x1908, 0},
	gputypes.TextureFormatBC3RGBAUnormSrgb:    {0x8C4F, 0x1908, 0},
	gputypes.TextureFormatBC4RUnorm:           {0x8DBB, 0x1903, 0}, // COMPRESSED_RED_RGTC1, RED
	gputypes.TextureFormatBC4RSnorm:           {0x8DBC, 0x1903, 0},
	gputypes.TextureFormatBC5RGUnorm:          {0x8DBD, 0x8227, 0}, // COMPRESSED_RG_RGTC2, RG
	gputypes.TextureFormatBC5RGSnorm:          {0x8DBE, 0x8227, 0},
	gputypes.TextureFormatBC6HRGBUfloat:       {0x8E8F, 0x1907, 0}, // COMPRESSED_RGB_BPTC_UNSIGNED_FLOAT, RGB
	gputypes.TextureFormatBC6HRGBFloat:        {0x8E8E, 0x1907, 0},
	gputypes.TextureFormatBC7RGBAUnorm:        {0x8E8C, 0x1908, 0},
	gputypes.TextureFormatBC7RGBAUnormSrgb:    {0x8E8D, 0x1908, 0},
	gputypes.TextureFormatETC2RGB8Unorm:       {0x9274, 0x1907, 0},
	gputypes.TextureFormatETC2RGB8UnormSrgb:   {0x9275, 0x1907, 0},
	gputypes.TextureFormatETC2RGB8A1Unorm:     {0x9276, 0x1908, 0},
	gputypes.TextureFormatETC2RGB8A1UnormSrgb: {0x9277, 0x1908, 0},
	gputypes.TextureFormatETC2RGBA8Unorm:      {0x9278, 0x1908, 0},
	gputypes.TextureFormatETC2RGBA8UnormSrgb:  {0x9279, 0x1908, 0},
	gputypes.TextureFormatEACR11Unorm:         {0x9270, 0x1903, 0},
	gputypes.TextureFormatEACR11Snorm:         {0x9271, 0x1903, 0},
	gputypes.TextureFormatEACRG11Unorm:        {0x9272, 0x8227, 0},
	gputypes.TextureFormatEACRG11Snorm:        {0x9273, 0x8227, 0},
	gputypes.TextureFormatASTC4x4Unorm:        {0x93B0, 0x1908, 0},
	gputypes.TextureFormatASTC4x4UnormSrgb:    {0x93D0, 0x1908, 0},
	gputypes.TextureFormatASTC5x4Unorm:        {0x93B1, 0x1908, 0},
	gputypes.TextureFormatASTC5x4UnormSrgb:    {0x93D1, 0x1908, 0},
	gputypes.TextureFormatASTC5x5Unorm:        {0x93B2, 0x1908, 0},
	gputypes.TextureFormatASTC5x5UnormSrgb:    {0x93D2, 0x1908, 0},
	gputypes.TextureFormatASTC6x5Unorm:        {0x93B3, 0x1908, 0},
	gputypes.TextureFormatASTC6x5UnormSrgb:    {0x93D3, 0x1908, 0},
	gputypes.TextureFormatASTC6x6Unorm:        {0x93B4, 0x1908, 0},
	gputypes.TextureFormatASTC6x6UnormSrgb:    {0x93D4, 0x1908, 0},
	gputypes.TextureFormatASTC8x5Unorm:        {0x93B5, 0x1908, 0},
	gputypes.TextureFormatASTC8x5UnormSrgb:    {0x93D5, 0x1908, 0},
	gputypes.TextureFormatASTC8x6Unorm:        {0x93B6, 0x1908, 0},
	gputypes.TextureFormatASTC8x6UnormSrgb:    {0x93D6, 0x1908, 0},
	gputypes.TextureFormatASTC8x8Unorm:        {0x93B7, 0x1908, 0},
	gputypes.TextureFormatASTC8x8UnormSrgb:    {0x93D7, 0x1908, 0},
	gputypes.TextureFormatASTC10x5Unorm:       {0x93B8, 0x1908, 0},
	gputypes.TextureFormatASTC10x5UnormSrgb:   {0x93D8, 0x1908, 0},
	gputypes.TextureFormatASTC10x6Unorm:       {0x93B9, 0x1908, 0},
	gputypes.TextureFormatASTC10x6UnormSrgb:   {0x93D9, 0x1908, 0},
	gputypes.TextureFormatASTC10x8Unorm:       {0x93BA, 0x1908, 0},
	gputypes.TextureFormatASTC10x8UnormSrgb:   {0x93DA, 0x1908, 0},
	gputypes.TextureFormatASTC10x10Unorm:      {0x93BB, 0x1908, 0},
	gputypes.TextureFormatASTC10x10UnormSrgb:  {0x93DB, 0x1908, 0},
	gputypes.TextureFormatASTC12x10Unorm:      {0x93BC, 0x1908, 0},
	gputypes.TextureFormatASTC12x10UnormSrgb:  {0x93DC, 0x1908, 0},
	gputypes.TextureFormatASTC12x12Unorm:      {0x93BD, 0x1908, 0},
	gputypes.TextureFormatASTC12x12UnormSrgb:  {0x93DD, 0x1908, 0},
}

func TestDescribeTextureFormatMatchesRust(t *testing.T) {
	for format, want := range rustCompressedFormats {
		if got, err := describeTextureFormat(format); err != nil {
			t.Errorf("%v: %v", format, err)
		} else if g := [3]uint32{got.internal, got.external, got.dataType}; g != want {
			t.Errorf("%v = %#x, want %#x (Rust describe_texture_format)", format, g, want)
		}
	}
	for format, want := range rustTextureFormats {
		got, err := describeTextureFormat(format)
		if err != nil {
			t.Errorf("%v: %v", format, err)
			continue
		}
		if g := [3]uint32{got.internal, got.external, got.dataType}; g != want {
			t.Errorf("%v = %#x, want %#x (Rust describe_texture_format)", format, g, want)
		}
	}
	if want := len(rustTextureFormats) + len(rustCompressedFormats); len(glTextureFormats) != want {
		t.Errorf("table has %d formats, Rust's rows %d", len(glTextureFormats), want)
	}
}

// TestDescribeTextureFormatRefusesUnknown: a format without a row is an
// error, never another format's storage: Undefined and values past the enum.
func TestDescribeTextureFormatRefusesUnknown(t *testing.T) {
	for _, format := range []gputypes.TextureFormat{
		gputypes.TextureFormatUndefined,
		gputypes.TextureFormat(9999),
	} {
		_, err := describeTextureFormat(format)
		var unsupported *UnsupportedTextureFormatError
		if !errors.As(err, &unsupported) || unsupported.Format != format {
			t.Errorf("%v: err = %v, want *UnsupportedTextureFormatError", format, err)
		}
	}
}

// TestUploadTexelBytesOfEveryFormat: every format with a row has a texel size
// GL reads (UNPACK_ROW_LENGTH is derived from it), except the combined
// depth/stencil formats, whose copies move one aspect, and the compressed
// formats, which are written in blocks (compressedUpload).
func TestUploadTexelBytesOfEveryFormat(t *testing.T) {
	want := map[gputypes.TextureFormat]uint64{
		gputypes.TextureFormatR8Uint:               1,
		gputypes.TextureFormatRG16Sint:             4,
		gputypes.TextureFormatRGB10A2Unorm:         4,
		gputypes.TextureFormatRGB10A2Uint:          4,
		gputypes.TextureFormatRG11B10Ufloat:        4,
		gputypes.TextureFormatRGB9E5Ufloat:         4,
		gputypes.TextureFormatRGBA16Unorm:          8,
		gputypes.TextureFormatRGBA32Sint:           16,
		gputypes.TextureFormatDepth24PlusStencil8:  0,
		gputypes.TextureFormatDepth32FloatStencil8: 0,
	}
	for format, desc := range glTextureFormats {
		if format.IsCompressed() {
			continue
		}
		got := uploadTexelBytes(desc.external, desc.dataType)
		if w, ok := want[format]; ok {
			if got != w {
				t.Errorf("%v: %d bytes, want %d", format, got, w)
			}
			continue
		}
		if got == 0 {
			t.Errorf("%v: no texel size", format)
		}
	}
}
