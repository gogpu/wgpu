// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package texcompress

import (
	"encoding/binary"

	"github.com/gogpu/gputypes"
)

// ETC2 and EAC: Khronos Data Format Specification 1.3, section "ETC2
// Compressed Texture Image Formats" (the same text as OpenGL ES 3.0
// Appendix C.1). Blocks are big-endian 64-bit words; texel (x, y) is pixel
// number 4x+y (column-major).

// etcModifiers are the intensity modifiers of the individual and
// differential modes, by table codeword and pixel index.
var etcModifiers = [8][4]int{
	{2, 8, -2, -8},
	{5, 17, -5, -17},
	{9, 29, -9, -29},
	{13, 42, -13, -42},
	{18, 60, -18, -60},
	{24, 80, -24, -80},
	{33, 106, -33, -106},
	{47, 183, -47, -183},
}

// etcDistances are the distances of the T and H modes.
var etcDistances = [8]int{3, 6, 11, 16, 23, 32, 41, 64}

// eacModifiers are the modifiers of EAC blocks, by table index and pixel
// index.
var eacModifiers = [16][8]int{
	{-3, -6, -9, -15, 2, 5, 8, 14},
	{-3, -7, -10, -13, 2, 6, 9, 12},
	{-2, -5, -8, -13, 1, 4, 7, 12},
	{-2, -4, -6, -13, 1, 3, 5, 12},
	{-3, -6, -8, -12, 2, 5, 7, 11},
	{-3, -7, -9, -11, 2, 6, 8, 10},
	{-4, -7, -8, -11, 3, 6, 7, 10},
	{-3, -5, -8, -11, 2, 4, 7, 10},
	{-2, -6, -8, -10, 1, 5, 7, 9},
	{-2, -5, -8, -10, 1, 4, 7, 9},
	{-2, -4, -8, -10, 1, 3, 7, 9},
	{-2, -5, -7, -10, 1, 4, 6, 9},
	{-3, -4, -7, -10, 2, 3, 6, 9},
	{-1, -2, -3, -10, 0, 1, 2, 9},
	{-4, -6, -8, -9, 3, 5, 7, 8},
	{-3, -5, -7, -9, 2, 4, 6, 8},
}

// decodeETC decodes the ETC2 and EAC formats (Decode).
func decodeETC(f gputypes.TextureFormat, block []byte, x, y int) ([4]float32, bool) {
	switch f {
	case gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb:
		return unorm8(decodeETC2(block, x, y, false)), true
	case gputypes.TextureFormatETC2RGB8A1Unorm, gputypes.TextureFormatETC2RGB8A1UnormSrgb:
		return unorm8(decodeETC2(block, x, y, true)), true
	case gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb:
		c := decodeETC2(block[8:], x, y, false)
		c[3] = eacAlpha(block, x, y)
		return unorm8(c), true
	case gputypes.TextureFormatEACR11Unorm:
		return [4]float32{eacUnorm(block, x, y), 0, 0, 1}, true
	case gputypes.TextureFormatEACR11Snorm:
		return [4]float32{eacSnorm(block, x, y), 0, 0, 1}, true
	case gputypes.TextureFormatEACRG11Unorm:
		return [4]float32{eacUnorm(block, x, y), eacUnorm(block[8:], x, y), 0, 1}, true
	case gputypes.TextureFormatEACRG11Snorm:
		return [4]float32{eacSnorm(block, x, y), eacSnorm(block[8:], x, y), 0, 1}, true
	}
	return [4]float32{}, false
}

func clamp255(v int) int {
	return max(0, min(255, v))
}

func extend4(v uint64) int { return int(v&0xf) * 0x11 }

func extend5(v uint64) int {
	c := int(v & 0x1f)
	return c<<3 | c>>2
}

// decodeETC2 returns texel (x, y) of an ETC2 RGB block. punchThrough selects
// the RGB8A1 decoding: bit 33 is then the opaque bit instead of the diff bit,
// the individual mode does not exist, and with the bit clear pixel index 2
// of the differential, T and H modes is transparent black.
func decodeETC2(block []byte, x, y int, punchThrough bool) [4]uint8 {
	w := binary.BigEndian.Uint64(block)
	pixel := uint(4*x + y)
	index := int(w>>(pixel+16)&1)<<1 | int(w>>pixel&1)
	bit33 := w>>33&1 == 1
	opaque := !punchThrough || bit33
	if !punchThrough && !bit33 {
		return etcSubblock(w, x, y, index, false, true)
	}
	r, g, b := int(w>>59&0x1f), int(w>>51&0x1f), int(w>>43&0x1f)
	dr, dg, db := signExtend(int(w>>56&7), 3), signExtend(int(w>>48&7), 3), signExtend(int(w>>40&7), 3)
	switch {
	case r+dr < 0 || r+dr > 31:
		return etcT(w, index, opaque)
	case g+dg < 0 || g+dg > 31:
		return etcH(w, index, opaque)
	case b+db < 0 || b+db > 31:
		return etcPlanar(w, x, y)
	}
	return etcSubblock(w, x, y, index, true, opaque)
}

// etcSubblock decodes the individual (differential false) and differential
// modes: a base color and modifier table per 2x4 or 4x2 subblock.
func etcSubblock(w uint64, x, y, index int, differential, opaque bool) [4]uint8 {
	flip := w>>32&1 == 1
	second := (!flip && x >= 2) || (flip && y >= 2)
	var r, g, b int
	switch {
	case !differential && !second:
		r, g, b = extend4(w>>60), extend4(w>>52), extend4(w>>44)
	case !differential:
		r, g, b = extend4(w>>56), extend4(w>>48), extend4(w>>40)
	case !second:
		r, g, b = extend5(w>>59), extend5(w>>51), extend5(w>>43)
	default:
		r = extend5(uint64(int(w>>59&0x1f) + signExtend(int(w>>56&7), 3)))
		g = extend5(uint64(int(w>>51&0x1f) + signExtend(int(w>>48&7), 3)))
		b = extend5(uint64(int(w>>43&0x1f) + signExtend(int(w>>40&7), 3)))
	}
	table := w >> 37 & 7
	if second {
		table = w >> 34 & 7
	}
	if !opaque {
		switch index {
		case 0:
			return [4]uint8{uint8(r), uint8(g), uint8(b), 255}
		case 2:
			return [4]uint8{}
		}
	}
	m := etcModifiers[table][index]
	return [4]uint8{uint8(clamp255(r + m)), uint8(clamp255(g + m)), uint8(clamp255(b + m)), 255}
}

// etcPaint returns paint color c plus d on every channel, clamped.
func etcPaint(c [3]int, d int) [4]uint8 {
	return [4]uint8{uint8(clamp255(c[0] + d)), uint8(clamp255(c[1] + d)), uint8(clamp255(c[2] + d)), 255}
}

// etcT decodes the T mode: paint colors C1, C2+d, C2, C2-d.
func etcT(w uint64, index int, opaque bool) [4]uint8 {
	c1 := [3]int{extend4(w>>57&0xc | w>>56&3), extend4(w >> 52), extend4(w >> 48)}
	c2 := [3]int{extend4(w >> 44), extend4(w >> 40), extend4(w >> 36)}
	d := etcDistances[w>>33&6|w>>32&1]
	if !opaque && index == 2 {
		return [4]uint8{}
	}
	switch index {
	case 0:
		return etcPaint(c1, 0)
	case 1:
		return etcPaint(c2, d)
	case 2:
		return etcPaint(c2, 0)
	}
	return etcPaint(c2, -d)
}

// etcH decodes the H mode: paint colors C1+d, C1-d, C2+d, C2-d, the low bit
// of the distance index set when C1 >= C2 as 12-bit RGB444 values.
func etcH(w uint64, index int, opaque bool) [4]uint8 {
	r1, g1, b1 := w>>59&0xf, w>>55&0xe|w>>52&1, w>>48&8|w>>47&7
	r2, g2, b2 := w>>43&0xf, w>>39&0xf, w>>35&0xf
	di := w>>32&4 | w>>31&2
	if r1<<8|g1<<4|b1 >= r2<<8|g2<<4|b2 {
		di |= 1
	}
	d := etcDistances[di]
	c1 := [3]int{extend4(r1), extend4(g1), extend4(b1)}
	c2 := [3]int{extend4(r2), extend4(g2), extend4(b2)}
	if !opaque && index == 2 {
		return [4]uint8{}
	}
	switch index {
	case 0:
		return etcPaint(c1, d)
	case 1:
		return etcPaint(c1, -d)
	case 2:
		return etcPaint(c2, d)
	}
	return etcPaint(c2, -d)
}

func extend6(v uint64) int {
	c := int(v & 0x3f)
	return c<<2 | c>>4
}

func extend7(v uint64) int {
	c := int(v & 0x7f)
	return c<<1 | c>>6
}

// etcPlanar decodes the planar mode: colors O, H and V at texels (0, 0),
// (4, 0) and (0, 4), interpolated linearly. It is always opaque.
func etcPlanar(w uint64, x, y int) [4]uint8 {
	o := [3]int{extend6(w >> 57), extend7(w>>50&0x40 | w>>49&0x3f), extend6(w>>43&0x20 | w>>40&0x18 | w>>39&7)}
	h := [3]int{extend6(w>>33&0x3e | w>>32&1), extend7(w >> 25), extend6(w >> 19)}
	v := [3]int{extend6(w >> 13), extend7(w >> 6), extend6(w)}
	var out [4]uint8
	for c := range 3 {
		out[c] = uint8(clamp255((x*(h[c]-o[c]) + y*(v[c]-o[c]) + 4*o[c] + 2) >> 2))
	}
	out[3] = 255
	return out
}

// eacTexel returns the base codeword, multiplier and modifier of texel
// (x, y) of an EAC block.
func eacTexel(block []byte, x, y int) (base, multiplier, modifier int) {
	w := binary.BigEndian.Uint64(block)
	pixel := uint(4*x + y)
	index := int(w >> (45 - 3*pixel) & 7)
	return int(w >> 56), int(w >> 52 & 0xf), eacModifiers[w>>48&0xf][index]
}

// eacAlpha returns the 8-bit alpha of texel (x, y) of an ETC2 RGBA8 alpha
// block.
func eacAlpha(block []byte, x, y int) uint8 {
	base, mul, mod := eacTexel(block, x, y)
	return uint8(clamp255(base + mod*mul))
}

// eacUnorm returns texel (x, y) of an unsigned R11 EAC block: an 11-bit
// value of 2047.
func eacUnorm(block []byte, x, y int) float32 {
	base, mul, mod := eacTexel(block, x, y)
	v := base*8 + 4
	if mul == 0 {
		v += mod
	} else {
		v += mod * mul * 8
	}
	return float32(max(0, min(2047, v))) / 2047
}

// eacSnorm returns texel (x, y) of a signed R11 EAC block: a value of
// [-1023, 1023] over 1023. A base codeword of -128 decodes as -127.
func eacSnorm(block []byte, x, y int) float32 {
	base, mul, mod := eacTexel(block, x, y)
	b := max(int(int8(base)), -127)
	v := b * 8
	if mul == 0 {
		v += mod
	} else {
		v += mod * mul * 8
	}
	return float32(max(-1023, min(1023, v))) / 1023
}
