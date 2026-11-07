// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package texcompress

import "encoding/binary"

// BC1-BC5 (S3TC and RGTC): Khronos Data Format Specification 1.3, sections
// "S3TC Compressed Texture Image Formats" and "RGTC Compressed Texture Image
// Formats". Block layout as in gitlab.freedesktop.org/mesa/mesa
// src/util/format/texcompress_s3tc_tmp.h and texcompress_rgtc_tmp.h (commit
// 27e63a95); the palette arithmetic is the GPUs' (package doc).

// expand565 widens a 5:6:5 color to 8 bits per channel by bit replication.
func expand565(c uint16) (r, g, b int) {
	r5, g6, b5 := int(c>>11), int(c>>5&0x3f), int(c&0x1f)
	return r5<<3 | r5>>2, g6<<2 | g6>>4, b5<<3 | b5>>2
}

// decodeBC1 returns texel (x, y) of the 8-byte color block. fourColor forces
// the four-color palette, as BC2 and BC3 color blocks always use; otherwise
// color0 <= color1 selects three colors and transparent black.
func decodeBC1(b []byte, x, y int, fourColor bool) [4]uint8 {
	c0, c1 := binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint16(b[2:])
	idx := b[4+y] >> (2 * x) & 3
	r0, g0, b0 := expand565(c0)
	r1, g1, b1 := expand565(c1)
	four := fourColor || c0 > c1
	var r, g, bl int
	switch {
	case idx == 0:
		r, g, bl = r0, g0, b0
	case idx == 1:
		r, g, bl = r1, g1, b1
	case idx == 2 && four:
		r, g, bl = mix256(r0, r1, 85), mix256(g0, g1, 85), mix256(b0, b1, 85)
	case idx == 2:
		r, g, bl = mix256(r0, r1, 128), mix256(g0, g1, 128), mix256(b0, b1, 128)
	case four:
		r, g, bl = mix256(r0, r1, 171), mix256(g0, g1, 171), mix256(b0, b1, 171)
	default:
		return [4]uint8{}
	}
	return [4]uint8{uint8(r), uint8(g), uint8(bl), 255}
}

// mix256 interpolates the 8-bit values a and b with weight w/256 of b,
// rounded. The thirds of the four-color palette are 85/256 and 171/256: the
// GPU readback of every BC1-BC3 block in testdata/gpu_readback.txt fits
// these weights and no exact third ((2a+b+1)/3 misses where 85/256 and 1/3
// round differently).
func mix256(a, b, w int) int {
	return ((256-w)*a + w*b + 128) >> 8
}

// decodeBC2 returns texel (x, y) of a BC2 block: 4-bit explicit alpha, then
// a BC1 color block.
func decodeBC2(b []byte, x, y int) [4]uint8 {
	c := decodeBC1(b[8:], x, y, true)
	i := 4*y + x
	a := b[i/2] >> (4 * (i & 1)) & 0xf
	c[3] = a<<4 | a
	return c
}

// bc3Weights7 and bc3Weights5 are the weights of a1, over 256, of the
// interpolated alpha values of the eight- and six-value BC3 modes:
// round(256k/7) and round(256k/5), the same rule as the color thirds of
// mix256.
var (
	bc3Weights7 = [6]int{37, 73, 110, 146, 183, 219}
	bc3Weights5 = [4]int{51, 102, 154, 205}
)

// decodeBC3 returns texel (x, y) of a BC3 block: a BC4-style alpha block,
// then a BC1 color block.
func decodeBC3(b []byte, x, y int) [4]uint8 {
	c := decodeBC1(b[8:], x, y, true)
	a0, a1 := int(b[0]), int(b[1])
	var a int
	switch idx := bc4Index(b, x, y); {
	case idx == 0:
		a = a0
	case idx == 1:
		a = a1
	case a0 > a1:
		a = mix256(a0, a1, bc3Weights7[idx-2])
	case idx == 6:
		a = 0
	case idx == 7:
		a = 255
	default:
		a = mix256(a0, a1, bc3Weights5[idx-2])
	}
	c[3] = uint8(a)
	return c
}

// bc4Index returns the 3-bit index of texel (x, y) of a BC4 block: 48 bits
// after the two endpoint bytes, texel 4y+x at bit 3(4y+x).
func bc4Index(b []byte, x, y int) int {
	bits := uint64(b[2]) | uint64(b[3])<<8 | uint64(b[4])<<16 |
		uint64(b[5])<<24 | uint64(b[6])<<32 | uint64(b[7])<<40
	return int(bits >> (3 * (4*y + x)) & 7)
}

// bc4Weights7 and bc4Weights5 are the palette weights of the eight- and
// six-value BC4 modes in 0.16 fixed point, round(65536k/n), indexed by k.
// An endpoint's weight is looked up on its own (e0 takes [n-k], e1 [k]), so
// the two need not sum to 65536: that is what the GPUs do. With the 8.8
// rounding and the expansion of bc4Palette, these weights reproduce every
// BC4 and BC5 texel of testdata/gpu_readback.txt, and weights of 15 or 17
// bits do not.
var (
	bc4Weights7 = [8]int{0, 9362, 18725, 28087, 37449, 46811, 56174, 65536}
	bc4Weights5 = [6]int{0, 13107, 26214, 39322, 52429, 65536}
)

// bc4Palette returns texel (x, y) of a BC4 block whose endpoints are e0 and
// e1, as an 8.8 fixed-point value (endpoints scaled by 256), signed when
// the endpoints are. eight selects the eight-value mode; in the six-value
// mode indices 6 and 7 are returned in min and max.
func bc4Palette(b []byte, x, y, e0, e1 int, eight bool, minV, maxV int) int {
	idx := bc4Index(b, x, y)
	w, n := bc4Weights7[:], 7
	if !eight {
		w, n = bc4Weights5[:], 5
		switch idx {
		case 6:
			return minV
		case 7:
			return maxV
		}
	}
	k := idx - 1
	switch idx {
	case 0:
		k = 0
	case 1:
		k = n
	}
	return (e0*w[n-k] + e1*w[k] + 128) >> 8
}

// bc4Unorm returns texel (x, y) of an unsigned BC4 block: the 8.8 palette
// value widened to 16 bits by replicating its integer byte, over 65535.
func bc4Unorm(b []byte, x, y int) float32 {
	v := bc4Palette(b, x, y, int(b[0]), int(b[1]), b[0] > b[1], 0, 255<<8)
	return float32(v*257>>8) / 65535
}

// bc4Snorm returns texel (x, y) of a signed BC4 block. An endpoint of -128
// is -127, as WebGPU's snorm rule says, and the mode is chosen after that
// clamp (-127, -128 is the six-value mode on the GPUs). The magnitude of
// the 8.8 palette value is widened to 15 bits by bit replication, over
// 32767.
func bc4Snorm(b []byte, x, y int) float32 {
	e0, e1 := max(int(int8(b[0])), -127), max(int(int8(b[1])), -127)
	v := bc4Palette(b, x, y, e0, e1, e0 > e1, -127<<8, 127<<8)
	m := v
	if m < 0 {
		m = -m
	}
	m += m>>7 + m>>14
	if v < 0 {
		m = -m
	}
	return float32(m) / 32767
}
