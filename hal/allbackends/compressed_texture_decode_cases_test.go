// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"github.com/gogpu/gputypes"
)

// decodeCase is a block-compressed format and the blocks a readback decodes:
// the texture is one row of len(blocks) 4x4 blocks.
type decodeCase struct {
	format  gputypes.TextureFormat
	feature gputypes.Feature
	blocks  [][]byte
}

// lcgBlock returns n pseudo-random bytes of seed: the payload bits of the
// blocks below, so every mode is decoded with arbitrary endpoints, indices
// and partitions rather than hand-picked ones.
func lcgBlock(seed uint32, n int) []byte {
	b := make([]byte, n)
	x := seed*2654435761 + 1
	for i := range b {
		x = x*1664525 + 1013904223
		b[i] = byte(x >> 24)
	}
	return b
}

// withLowBits returns block with its lowest n bits (bit 0 of byte 0 first)
// replaced by the low n bits of v: the mode field of BC6H and BC7.
func withLowBits(block []byte, v uint32, n int) []byte {
	for i := range n {
		byteIdx, bit := i/8, uint(i%8)
		block[byteIdx] = block[byteIdx]&^(1<<bit) | byte(v>>i&1)<<bit
	}
	return block
}

// bc7Blocks holds one block of each BC7 mode (mode m is m zero bits and a
// one) and the reserved mode 8 (a zero first byte).
func bc7Blocks() [][]byte {
	blocks := make([][]byte, 0, 9)
	for m := range 8 {
		blocks = append(blocks, withLowBits(lcgBlock(uint32(700+m), 16), 1<<m, m+1))
	}
	return append(blocks, withLowBits(lcgBlock(708, 16), 0, 8))
}

// bc6hModes are the BC6H mode fields, low bits first in the block: the two
// 2-bit modes, the twelve 5-bit modes and one reserved 5-bit mode.
var bc6hModes = []struct {
	value uint32
	bits  int
}{
	{0x00, 2}, {0x01, 2},
	{0x02, 5}, {0x06, 5}, {0x0A, 5}, {0x0E, 5}, {0x12, 5}, {0x16, 5}, {0x1A, 5}, {0x1E, 5},
	{0x03, 5}, {0x07, 5}, {0x0B, 5}, {0x0F, 5},
	{0x13, 5},
}

func bc6hBlocks(seed uint32) [][]byte {
	blocks := make([][]byte, 0, len(bc6hModes))
	for i, m := range bc6hModes {
		blocks = append(blocks, withLowBits(lcgBlock(seed+uint32(i), 16), m.value, m.bits))
	}
	return blocks
}

// lcgBlocks returns count pseudo-random blocks of n bytes.
func lcgBlocks(seed uint32, count, n int) [][]byte {
	blocks := make([][]byte, count)
	for i := range blocks {
		blocks[i] = lcgBlock(seed+uint32(i), n)
	}
	return blocks
}

// The Mesa blocks are the DXT1/3/5 blocks of
// gitlab.freedesktop.org/mesa/mesa src/util/format/u_format_tests.c
// (commit 27e63a95).
var (
	mesaDXT1RGB  = []byte{0xf2, 0xd7, 0xb0, 0x20, 0xae, 0x2c, 0x6f, 0x97}
	mesaDXT1RGBA = []byte{0xff, 0x2f, 0xa4, 0x72, 0xeb, 0xb2, 0xbd, 0xbe}
	mesaDXT3     = []byte{0xe7, 0x4a, 0x8f, 0x96, 0x5b, 0xc1, 0x1c, 0x84, 0xf6, 0x8f, 0xab, 0x32, 0x2a, 0x9a, 0x95, 0x5a}
	mesaDXT5     = []byte{0xf8, 0x11, 0xc5, 0x0c, 0x9a, 0x73, 0xb4, 0x9c, 0xf6, 0x8f, 0xab, 0x32, 0x2a, 0x9a, 0x95, 0x5a}
)

// bc4Blocks holds BC4 blocks of both interpolation modes: red0 > red1 (eight
// values) and red0 <= red1 (six values plus 0 and 1), with endpoints at the
// range ends, the snorm -128 among them.
func bc4Blocks(seed uint32) [][]byte {
	return append([][]byte{
		{0xF0, 0x10, 0x88, 0xC6, 0xFA, 0x05, 0x39, 0x77},
		{0x10, 0xF0, 0x88, 0xC6, 0xFA, 0x05, 0x39, 0x77},
		{0xFF, 0x00, 0x24, 0x92, 0x49, 0x24, 0x92, 0x49},
		{0x80, 0x7F, 0x24, 0x92, 0x49, 0x24, 0x92, 0x49},
		{0x81, 0x80, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00},
	}, lcgBlocks(seed, 3, 8)...)
}

func bc5Blocks(seed uint32) [][]byte {
	single := bc4Blocks(seed)
	blocks := make([][]byte, 0, len(single))
	for i := range single {
		blocks = append(blocks, append(append([]byte{}, single[i]...), single[(i+3)%len(single)]...))
	}
	return blocks
}

// etc2Blocks holds an ETC2 RGB block of each mode. The first word's bit 33
// (byte 3, bit 1) is the diff bit; with it set, an overflow of red + dred
// selects T, of green + dgreen H, of blue + dblue planar, and none the
// differential mode. The flip bit and the tables come from the payload.
func etc2Blocks(seed uint32) [][]byte {
	base := lcgBlocks(seed, 6, 8)
	set := func(b []byte, r, g, bl byte, diff bool) []byte {
		b[0], b[1], b[2] = r, g, bl
		if diff {
			b[3] |= 2
		} else {
			b[3] &^= 2
		}
		return b
	}
	return [][]byte{
		set(base[0], 0xA5, 0x3C, 0x96, false), // individual
		set(base[1], 0x81, 0x42, 0xC3, true),  // differential: 16+1, 8+2, 24+3
		set(base[2], 0xFB, 0x42, 0xC3, true),  // T: red 31+3 overflows
		set(base[3], 0x81, 0xF9, 0xC3, true),  // H: green 31+1 overflows
		set(base[4], 0x81, 0x42, 0x04, true),  // planar: blue 0-4 underflows
		base[5],
	}
}

// etc2A1Blocks holds ETC2 punch-through blocks: bit 33 is the opaque bit, and
// with it clear the differential, T and H modes mark index 2 transparent.
func etc2A1Blocks(seed uint32) [][]byte {
	blocks := etc2Blocks(seed)
	for _, b := range etc2Blocks(seed + 50) {
		c := append([]byte{}, b...)
		c[3] &^= 2
		blocks = append(blocks, c)
	}
	return blocks
}

// eacBlocks holds EAC blocks: pseudo-random ones, one with multiplier 0 and
// one with base codeword 0x80 (-128 in the snorm formats).
func eacBlocks(seed uint32) [][]byte {
	blocks := lcgBlocks(seed, 6, 8)
	blocks[4][1] &^= 0xF0
	blocks[5][0] = 0x80
	blocks[5][1] = blocks[5][1]&0x0F | 0x30
	return blocks
}

func pairBlocks(a, b [][]byte) [][]byte {
	blocks := make([][]byte, len(a))
	for i := range a {
		blocks[i] = append(append([]byte{}, a[i]...), b[(i+1)%len(b)]...)
	}
	return blocks
}

var decodeCases = []decodeCase{
	{gputypes.TextureFormatBC1RGBAUnorm, gputypes.FeatureTextureCompressionBC,
		append([][]byte{mesaDXT1RGB, mesaDXT1RGBA}, lcgBlocks(100, 4, 8)...)},
	{gputypes.TextureFormatBC1RGBAUnormSrgb, gputypes.FeatureTextureCompressionBC,
		append([][]byte{mesaDXT1RGB, mesaDXT1RGBA}, lcgBlocks(110, 2, 8)...)},
	{gputypes.TextureFormatBC2RGBAUnorm, gputypes.FeatureTextureCompressionBC,
		append([][]byte{mesaDXT3}, lcgBlocks(200, 3, 16)...)},
	{gputypes.TextureFormatBC2RGBAUnormSrgb, gputypes.FeatureTextureCompressionBC,
		append([][]byte{mesaDXT3}, lcgBlocks(210, 1, 16)...)},
	{gputypes.TextureFormatBC3RGBAUnorm, gputypes.FeatureTextureCompressionBC,
		append(append([][]byte{mesaDXT5}, lcgBlocks(300, 3, 16)...), pairBlocks(bc4Blocks(305)[:3], lcgBlocks(306, 3, 8))...)},
	{gputypes.TextureFormatBC3RGBAUnormSrgb, gputypes.FeatureTextureCompressionBC,
		append(append([][]byte{mesaDXT5}, lcgBlocks(310, 1, 16)...), pairBlocks(bc4Blocks(315)[1:2], lcgBlocks(316, 1, 8))...)},
	{gputypes.TextureFormatBC4RUnorm, gputypes.FeatureTextureCompressionBC, bc4Blocks(400)},
	{gputypes.TextureFormatBC4RSnorm, gputypes.FeatureTextureCompressionBC, bc4Blocks(410)},
	{gputypes.TextureFormatBC5RGUnorm, gputypes.FeatureTextureCompressionBC, bc5Blocks(500)},
	{gputypes.TextureFormatBC5RGSnorm, gputypes.FeatureTextureCompressionBC, bc5Blocks(510)},
	{gputypes.TextureFormatBC6HRGBUfloat, gputypes.FeatureTextureCompressionBC, bc6hBlocks(600)},
	{gputypes.TextureFormatBC6HRGBFloat, gputypes.FeatureTextureCompressionBC, bc6hBlocks(650)},
	{gputypes.TextureFormatBC7RGBAUnorm, gputypes.FeatureTextureCompressionBC, bc7Blocks()},
	{gputypes.TextureFormatBC7RGBAUnormSrgb, gputypes.FeatureTextureCompressionBC, bc7Blocks()[:8]},
	{gputypes.TextureFormatETC2RGB8Unorm, gputypes.FeatureTextureCompressionETC2, etc2Blocks(800)},
	{gputypes.TextureFormatETC2RGB8UnormSrgb, gputypes.FeatureTextureCompressionETC2, etc2Blocks(810)},
	{gputypes.TextureFormatETC2RGB8A1Unorm, gputypes.FeatureTextureCompressionETC2, etc2A1Blocks(820)},
	{gputypes.TextureFormatETC2RGB8A1UnormSrgb, gputypes.FeatureTextureCompressionETC2, etc2A1Blocks(830)},
	{gputypes.TextureFormatETC2RGBA8Unorm, gputypes.FeatureTextureCompressionETC2,
		pairBlocks(eacBlocks(840), etc2Blocks(845))},
	{gputypes.TextureFormatETC2RGBA8UnormSrgb, gputypes.FeatureTextureCompressionETC2,
		pairBlocks(eacBlocks(850), etc2Blocks(855))},
	{gputypes.TextureFormatEACR11Unorm, gputypes.FeatureTextureCompressionETC2, eacBlocks(900)},
	{gputypes.TextureFormatEACR11Snorm, gputypes.FeatureTextureCompressionETC2, eacBlocks(910)},
	{gputypes.TextureFormatEACRG11Unorm, gputypes.FeatureTextureCompressionETC2, pairBlocks(eacBlocks(920), eacBlocks(925))},
	{gputypes.TextureFormatEACRG11Snorm, gputypes.FeatureTextureCompressionETC2, pairBlocks(eacBlocks(930), eacBlocks(935))},
}
