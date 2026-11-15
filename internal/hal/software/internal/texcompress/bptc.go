// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package texcompress

import "math"

// BC6H and BC7 (BPTC): Khronos Data Format Specification 1.3, section "BPTC
// Compressed Texture Image Formats". The mode tables, partition tables,
// anchor indices and decoding steps are ported from
// gitlab.freedesktop.org/mesa/mesa src/util/format/texcompress_bptc_tmp.h
// (commit 27e63a95, MIT), fetch_rgba_unorm_from_block and
// fetch_rgb_float_from_block.

type bc7Mode struct {
	subsets, partitionBits    int
	rotation, indexSelection  bool
	colorBits, alphaBits      int
	endpointPBits, sharedPBit bool
	indexBits, secondaryBits  int
}

var bc7Modes = [8]bc7Mode{
	{3, 4, false, false, 4, 0, true, false, 3, 0},
	{2, 6, false, false, 6, 0, false, true, 3, 0},
	{3, 6, false, false, 5, 0, false, false, 2, 0},
	{2, 6, false, false, 7, 0, true, false, 2, 0},
	{1, 0, true, true, 5, 6, false, false, 2, 3},
	{1, 0, true, false, 7, 8, false, false, 2, 2},
	{1, 0, false, false, 7, 7, true, false, 4, 0},
	{2, 6, false, false, 5, 5, true, false, 2, 0},
}

// bc6hField places nBits of the block into bit offset of endpoint's
// component, bit-reversed when reverse is set.
type bc6hField struct {
	endpoint, component, offset, nBits int8
	reverse                            bool
}

type bc6hMode struct {
	reserved      bool
	transformed   bool
	partitionBits int
	endpointBits  int
	indexBits     int
	deltaBits     [3]int
	fields        []bc6hField
}

// bc6hModes is indexed by the mode number Mesa computes from the mode bits
// (bc6hModeNumber).
//
//nolint:dupl // Table data: dupl matches the syntax of modes 1 and 17 (same field count), not their values.
var bc6hModes = [18]bc6hMode{
	{false, true, 5, 10, 3, [3]int{5, 5, 5}, []bc6hField{
		{2, 1, 4, 1, false}, {2, 2, 4, 1, false}, {3, 2, 4, 1, false},
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 5, false}, {3, 1, 4, 1, false}, {2, 1, 0, 4, false},
		{1, 1, 0, 5, false}, {3, 2, 0, 1, false}, {3, 1, 0, 4, false},
		{1, 2, 0, 5, false}, {3, 2, 1, 1, false}, {2, 2, 0, 4, false},
		{2, 0, 0, 5, false}, {3, 2, 2, 1, false}, {3, 0, 0, 5, false},
		{3, 2, 3, 1, false}}},
	{false, true, 5, 7, 3, [3]int{6, 6, 6}, []bc6hField{
		{2, 1, 5, 1, false}, {3, 1, 4, 1, false}, {3, 1, 5, 1, false},
		{0, 0, 0, 7, false}, {3, 2, 0, 1, false}, {3, 2, 1, 1, false},
		{2, 2, 4, 1, false}, {0, 1, 0, 7, false}, {2, 2, 5, 1, false},
		{3, 2, 2, 1, false}, {2, 1, 4, 1, false}, {0, 2, 0, 7, false},
		{3, 2, 3, 1, false}, {3, 2, 5, 1, false}, {3, 2, 4, 1, false},
		{1, 0, 0, 6, false}, {2, 1, 0, 4, false}, {1, 1, 0, 6, false},
		{3, 1, 0, 4, false}, {1, 2, 0, 6, false}, {2, 2, 0, 4, false},
		{2, 0, 0, 6, false},
		{3, 0, 0, 6, false}}},
	// 00010
	{false, true, 5, 11, 3, [3]int{5, 4, 4}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 5, false}, {0, 0, 10, 1, false}, {2, 1, 0, 4, false},
		{1, 1, 0, 4, false}, {0, 1, 10, 1, false}, {3, 2, 0, 1, false},
		{3, 1, 0, 4, false}, {1, 2, 0, 4, false}, {0, 2, 10, 1, false},
		{3, 2, 1, 1, false}, {2, 2, 0, 4, false}, {2, 0, 0, 5, false},
		{3, 2, 2, 1, false}, {3, 0, 0, 5, false}, {3, 2, 3, 1, false}}},
	// 00011
	{false, false, 0, 10, 4, [3]int{10, 10, 10}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 10, false}, {1, 1, 0, 10, false}, {1, 2, 0, 10, false}}},
	// 00110
	{false, true, 5, 11, 3, [3]int{4, 5, 4}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 4, false}, {0, 0, 10, 1, false}, {3, 1, 4, 1, false},
		{2, 1, 0, 4, false}, {1, 1, 0, 5, false}, {0, 1, 10, 1, false},
		{3, 1, 0, 4, false}, {1, 2, 0, 4, false}, {0, 2, 10, 1, false},
		{3, 2, 1, 1, false}, {2, 2, 0, 4, false}, {2, 0, 0, 4, false},
		{3, 2, 0, 1, false}, {3, 2, 2, 1, false}, {3, 0, 0, 4, false},
		{2, 1, 4, 1, false}, {3, 2, 3, 1, false}}},
	// 00111
	{false, true, 0, 11, 4, [3]int{9, 9, 9}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 9, false}, {0, 0, 10, 1, false}, {1, 1, 0, 9, false},
		{0, 1, 10, 1, false}, {1, 2, 0, 9, false}, {0, 2, 10, 1, false}}},
	// 01010
	{false, true, 5, 11, 3, [3]int{4, 4, 5}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 4, false}, {0, 0, 10, 1, false}, {2, 2, 4, 1, false},
		{2, 1, 0, 4, false}, {1, 1, 0, 4, false}, {0, 1, 10, 1, false},
		{3, 2, 0, 1, false}, {3, 1, 0, 4, false}, {1, 2, 0, 5, false},
		{0, 2, 10, 1, false}, {2, 2, 0, 4, false}, {2, 0, 0, 4, false},
		{3, 2, 1, 1, false}, {3, 2, 2, 1, false}, {3, 0, 0, 4, false},
		{3, 2, 4, 1, false}, {3, 2, 3, 1, false}}},
	// 01011
	{false, true, 0, 12, 4, [3]int{8, 8, 8}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 8, false}, {0, 0, 10, 2, true}, {1, 1, 0, 8, false},
		{0, 1, 10, 2, true}, {1, 2, 0, 8, false}, {0, 2, 10, 2, true}}},
	// 01110
	{false, true, 5, 9, 3, [3]int{5, 5, 5}, []bc6hField{
		{0, 0, 0, 9, false}, {2, 2, 4, 1, false}, {0, 1, 0, 9, false},
		{2, 1, 4, 1, false}, {0, 2, 0, 9, false}, {3, 2, 4, 1, false},
		{1, 0, 0, 5, false}, {3, 1, 4, 1, false}, {2, 1, 0, 4, false},
		{1, 1, 0, 5, false}, {3, 2, 0, 1, false}, {3, 1, 0, 4, false},
		{1, 2, 0, 5, false}, {3, 2, 1, 1, false}, {2, 2, 0, 4, false},
		{2, 0, 0, 5, false}, {3, 2, 2, 1, false}, {3, 0, 0, 5, false},
		{3, 2, 3, 1, false}}},
	// 01111
	{false, true, 0, 16, 4, [3]int{4, 4, 4}, []bc6hField{
		{0, 0, 0, 10, false}, {0, 1, 0, 10, false}, {0, 2, 0, 10, false},
		{1, 0, 0, 4, false}, {0, 0, 10, 6, true}, {1, 1, 0, 4, false},
		{0, 1, 10, 6, true}, {1, 2, 0, 4, false}, {0, 2, 10, 6, true}}},
	// 10010
	{false, true, 5, 8, 3, [3]int{6, 5, 5}, []bc6hField{
		{0, 0, 0, 8, false}, {3, 1, 4, 1, false}, {2, 2, 4, 1, false},
		{0, 1, 0, 8, false}, {3, 2, 2, 1, false}, {2, 1, 4, 1, false},
		{0, 2, 0, 8, false}, {3, 2, 3, 1, false}, {3, 2, 4, 1, false},
		{1, 0, 0, 6, false}, {2, 1, 0, 4, false}, {1, 1, 0, 5, false},
		{3, 2, 0, 1, false}, {3, 1, 0, 4, false}, {1, 2, 0, 5, false},
		{3, 2, 1, 1, false}, {2, 2, 0, 4, false}, {2, 0, 0, 6, false},
		{3, 0, 0, 6, false}}},
	// 10011
	{reserved: true},
	// 10110
	{false, true, 5, 8, 3, [3]int{5, 6, 5}, []bc6hField{
		{0, 0, 0, 8, false}, {3, 2, 0, 1, false}, {2, 2, 4, 1, false},
		{0, 1, 0, 8, false}, {2, 1, 5, 1, false}, {2, 1, 4, 1, false},
		{0, 2, 0, 8, false}, {3, 1, 5, 1, false}, {3, 2, 4, 1, false},
		{1, 0, 0, 5, false}, {3, 1, 4, 1, false}, {2, 1, 0, 4, false},
		{1, 1, 0, 6, false}, {3, 1, 0, 4, false}, {1, 2, 0, 5, false},
		{3, 2, 1, 1, false}, {2, 2, 0, 4, false}, {2, 0, 0, 5, false},
		{3, 2, 2, 1, false}, {3, 0, 0, 5, false}, {3, 2, 3, 1, false}}},
	// 10111
	{reserved: true},
	// 11010
	{false, true, 5, 8, 3, [3]int{5, 5, 6}, []bc6hField{
		{0, 0, 0, 8, false}, {3, 2, 1, 1, false}, {2, 2, 4, 1, false},
		{0, 1, 0, 8, false}, {2, 2, 5, 1, false}, {2, 1, 4, 1, false},
		{0, 2, 0, 8, false}, {3, 2, 5, 1, false}, {3, 2, 4, 1, false},
		{1, 0, 0, 5, false}, {3, 1, 4, 1, false}, {2, 1, 0, 4, false},
		{1, 1, 0, 5, false}, {3, 2, 0, 1, false}, {3, 1, 0, 4, false},
		{1, 2, 0, 6, false}, {2, 2, 0, 4, false}, {2, 0, 0, 5, false},
		{3, 2, 2, 1, false}, {3, 0, 0, 5, false}, {3, 2, 3, 1, false}}},
	// 11011
	{reserved: true},
	// 11110
	{false, false, 5, 6, 3, [3]int{6, 6, 6}, []bc6hField{
		{0, 0, 0, 6, false}, {3, 1, 4, 1, false}, {3, 2, 0, 1, false},
		{3, 2, 1, 1, false}, {2, 2, 4, 1, false}, {0, 1, 0, 6, false},
		{2, 1, 5, 1, false}, {2, 2, 5, 1, false}, {3, 2, 2, 1, false},
		{2, 1, 4, 1, false}, {0, 2, 0, 6, false}, {3, 1, 5, 1, false},
		{3, 2, 3, 1, false}, {3, 2, 5, 1, false}, {3, 2, 4, 1, false},
		{1, 0, 0, 6, false}, {2, 1, 0, 4, false}, {1, 1, 0, 6, false},
		{3, 1, 0, 4, false}, {1, 2, 0, 6, false}, {2, 2, 0, 4, false},
		{2, 0, 0, 6, false}, {3, 0, 0, 6, false}}},
	// 11111
	{reserved: true},
}

// partitions2 gives, 2 bits per texel, the subset of each texel in the 64
// two-subset partitions; partitions3 the same for three subsets.
var partitions2 = [64]uint32{
	0x50505050, 0x40404040, 0x54545454, 0x54505040,
	0x50404000, 0x55545450, 0x55545040, 0x54504000,
	0x50400000, 0x55555450, 0x55544000, 0x54400000,
	0x55555440, 0x55550000, 0x55555500, 0x55000000,
	0x55150100, 0x00004054, 0x15010000, 0x00405054,
	0x00004050, 0x15050100, 0x05010000, 0x40505054,
	0x00404050, 0x05010100, 0x14141414, 0x05141450,
	0x01155440, 0x00555500, 0x15014054, 0x05414150,
	0x44444444, 0x55005500, 0x11441144, 0x05055050,
	0x05500550, 0x11114444, 0x41144114, 0x44111144,
	0x15055054, 0x01055040, 0x05041050, 0x05455150,
	0x14414114, 0x50050550, 0x41411414, 0x00141400,
	0x00041504, 0x00105410, 0x10541000, 0x04150400,
	0x50410514, 0x41051450, 0x05415014, 0x14054150,
	0x41050514, 0x41505014, 0x40011554, 0x54150140,
	0x50505500, 0x00555050, 0x15151010, 0x54540404,
}

var partitions3 = [64]uint32{
	0xaa685050, 0x6a5a5040, 0x5a5a4200, 0x5450a0a8,
	0xa5a50000, 0xa0a05050, 0x5555a0a0, 0x5a5a5050,
	0xaa550000, 0xaa555500, 0xaaaa5500, 0x90909090,
	0x94949494, 0xa4a4a4a4, 0xa9a59450, 0x2a0a4250,
	0xa5945040, 0x0a425054, 0xa5a5a500, 0x55a0a0a0,
	0xa8a85454, 0x6a6a4040, 0xa4a45000, 0x1a1a0500,
	0x0050a4a4, 0xaaa59090, 0x14696914, 0x69691400,
	0xa08585a0, 0xaa821414, 0x50a4a450, 0x6a5a0200,
	0xa9a58000, 0x5090a0a8, 0xa8a09050, 0x24242424,
	0x00aa5500, 0x24924924, 0x24499224, 0x50a50a50,
	0x500aa550, 0xaaaa4444, 0x66660000, 0xa5a0a5a0,
	0x50a050a0, 0x69286928, 0x44aaaa44, 0x66666600,
	0xaa444444, 0x54a854a8, 0x95809580, 0x96969600,
	0xa85454a8, 0x80959580, 0xaa141414, 0x96960000,
	0xaaaa1414, 0xa05050a0, 0xa0a5a5a0, 0x96000000,
	0x40804080, 0xa9a8a9a8, 0xaaaaaa44, 0x2a4a5254,
}

// anchors holds the anchor texel of the second subset of the two-subset
// partitions, then of the second and third subsets of the three-subset
// partitions. Texel 0 anchors the first subset in every partition.
var anchors = [3][64]uint8{
	{
		15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
		15, 2, 8, 2, 2, 8, 8, 15, 2, 8, 2, 2, 8, 8, 2, 2,
		15, 15, 6, 8, 2, 8, 15, 15, 2, 8, 2, 2, 2, 15, 15, 6,
		6, 2, 6, 8, 15, 15, 2, 2, 15, 15, 15, 15, 15, 2, 2, 15,
	},
	{
		3, 3, 15, 15, 8, 3, 15, 15, 8, 8, 6, 6, 6, 5, 3, 3,
		3, 3, 8, 15, 3, 3, 6, 10, 5, 8, 8, 6, 8, 5, 15, 15,
		8, 15, 3, 5, 6, 10, 8, 15, 15, 3, 15, 5, 15, 15, 15, 15,
		3, 15, 5, 5, 5, 8, 5, 10, 5, 10, 8, 13, 15, 12, 3, 3,
	},
	{
		15, 8, 8, 3, 15, 15, 3, 8, 15, 15, 15, 15, 15, 15, 15, 8,
		15, 8, 15, 3, 15, 8, 15, 8, 3, 15, 6, 10, 15, 15, 10, 8,
		15, 3, 15, 10, 10, 8, 9, 10, 6, 15, 8, 15, 3, 6, 6, 8,
		15, 3, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 3, 15, 15, 8,
	},
}

// bits returns the n <= 32 bits of block starting at bit offset (bit 0 is
// the lowest bit of byte 0).
func bits(block []byte, offset, n int) int {
	v := 0
	for i := 0; i < n; {
		byteIdx, bit := (offset+i)/8, (offset+i)%8
		take := min(n-i, 8-bit)
		v |= int(block[byteIdx]>>bit&(1<<take-1)) << i
		i += take
	}
	return v
}

// anchor reports whether texel anchors its subset, and counts the anchors
// before it: each anchor stores its index with one bit less.
func anchor(subsets, partition, texel int) (isAnchor bool, before int) {
	if texel == 0 {
		return true, 0
	}
	before = 1
	switch subsets {
	case 2:
		a := int(anchors[0][partition])
		isAnchor = texel == a
		if texel > a {
			before++
		}
	case 3:
		a1, a2 := int(anchors[1][partition]), int(anchors[2][partition])
		isAnchor = texel == a1 || texel == a2
		if texel > a1 {
			before++
		}
		if texel > a2 {
			before++
		}
	}
	return isAnchor, before
}

var bptcWeights = [5][]int{
	2: {0, 21, 43, 64},
	3: {0, 9, 18, 27, 37, 46, 55, 64},
	4: {0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64},
}

func bptcInterpolate(a, b, index, indexBits int) int {
	w := bptcWeights[indexBits][index]
	return ((64-w)*a + w*b + 32) >> 6
}

// expandBits widens an n-bit value, 4 <= n <= 8, to 8 bits by replicating
// its high bits.
func expandBits(v, n int) int {
	v <<= 8 - n
	return (v | v>>n) & 0xff
}

// bc7Endpoints reads the endpoints of mode m from bit offset: per channel,
// per subset, both endpoints; then alpha; then the p-bits. It returns them
// widened to 8 bits and the offset after them.
func bc7Endpoints(m *bc7Mode, block []byte, offset int) (e [6][4]int, next int) {
	n := 2 * m.subsets
	for c := range 3 {
		for i := range n {
			e[i][c] = bits(block, offset, m.colorBits)
			offset += m.colorBits
		}
	}
	channels := 3
	if m.alphaBits > 0 {
		channels = 4
		for i := range n {
			e[i][3] = bits(block, offset, m.alphaBits)
			offset += m.alphaBits
		}
	}
	pbit := 0
	switch {
	case m.endpointPBits:
		pbit = 1
		for i := range n {
			p := bits(block, offset, 1)
			offset++
			for c := range channels {
				e[i][c] = e[i][c]<<1 | p
			}
		}
	case m.sharedPBit:
		pbit = 1
		for s := range m.subsets {
			p := bits(block, offset, 1)
			offset++
			for i := 2 * s; i < 2*s+2; i++ {
				for c := range channels {
					e[i][c] = e[i][c]<<1 | p
				}
			}
		}
	}
	for i := range n {
		for c := range 3 {
			e[i][c] = expandBits(e[i][c], m.colorBits+pbit)
		}
		if m.alphaBits > 0 {
			e[i][3] = expandBits(e[i][3], m.alphaBits+pbit)
		} else {
			e[i][3] = 255
		}
	}
	return e, offset
}

// decodeBC7 returns texel (x, y) of a BC7 block. The reserved mode (a zero
// first byte) decodes as transparent black.
func decodeBC7(block []byte, x, y int) [4]uint8 {
	if block[0] == 0 {
		return [4]uint8{}
	}
	modeNum := 0
	for block[0]>>modeNum&1 == 0 {
		modeNum++
	}
	m := &bc7Modes[modeNum]
	offset := modeNum + 1
	partition := bits(block, offset, m.partitionBits)
	offset += m.partitionBits
	var subsetMap uint32
	switch m.subsets {
	case 2:
		subsetMap = partitions2[partition]
	case 3:
		subsetMap = partitions3[partition]
	}
	rotation, indexSel := 0, 0
	if m.rotation {
		rotation = bits(block, offset, 2)
		offset += 2
	}
	if m.indexSelection {
		indexSel = bits(block, offset, 1)
		offset++
	}
	e, offset := bc7Endpoints(m, block, offset)

	texel := 4*y + x
	isAnchor, before := anchor(m.subsets, partition, texel)
	secondary := offset + 16*m.indexBits - m.subsets + m.secondaryBits*texel - before
	offset += m.indexBits*texel - before
	drop := 0
	if isAnchor {
		drop = 1
	}
	var idx [2]int
	idx[0] = bits(block, offset, m.indexBits-drop)
	if m.secondaryBits > 0 {
		idx[1] = bits(block, secondary, m.secondaryBits-drop)
	}
	subset := int(subsetMap >> (2 * texel) & 3)
	e0, e1 := e[2*subset], e[2*subset+1]

	colorIdx, colorBits := idx[0], m.indexBits
	alphaIdx, alphaBits := idx[0], m.indexBits
	if m.secondaryBits > 0 {
		if indexSel == 1 {
			colorIdx, colorBits = idx[1], m.secondaryBits
		} else {
			alphaIdx, alphaBits = idx[1], m.secondaryBits
		}
	}
	var out [4]uint8
	for c := range 3 {
		out[c] = uint8(bptcInterpolate(e0[c], e1[c], colorIdx, colorBits))
	}
	out[3] = uint8(bptcInterpolate(e0[3], e1[3], alphaIdx, alphaBits))
	if rotation > 0 {
		out[rotation-1], out[3] = out[3], out[rotation-1]
	}
	return out
}

// bc6hModeNumber returns the index into bc6hModes of the block's mode and
// the bit offset after the mode bits: two mode bits when bit 1 is clear,
// five otherwise.
func bc6hModeNumber(block []byte) (mode, offset int) {
	b := int(block[0])
	if b&2 != 0 {
		return (b>>1&0xe | b&1) + 2, 5
	}
	return b & 3, 2
}

// signExtend sign-extends the low n bits of v.
func signExtend(v, n int) int {
	shift := 64 - n
	return int(int64(v) << shift >> shift)
}

func bc6hUnquantize(v, n int, signed bool) int {
	if signed {
		if n >= 16 || v == 0 {
			return v
		}
		neg := v < 0
		if neg {
			v = -v
		}
		if v >= 1<<(n-1)-1 {
			v = 0x7fff
		} else {
			v = (v<<15 + 0x4000) >> (n - 1)
		}
		if neg {
			return -v
		}
		return v
	}
	if n >= 15 || v == 0 {
		return v
	}
	if v == 1<<n-1 {
		return 0xffff
	}
	return (v<<15 + 0x4000) >> (n - 1)
}

// bc6hEndpoints reads and unquantizes the endpoints of mode m from bit
// offset, returning the offset after them.
func bc6hEndpoints(m *bc6hMode, block []byte, offset int, signed bool) (e [4][3]int, next int) {
	for _, f := range m.fields {
		v := bits(block, offset, int(f.nBits))
		offset += int(f.nBits)
		if f.reverse {
			r := 0
			for i := range int(f.nBits) {
				if v&(1<<i) != 0 {
					r |= 1 << (int(f.nBits) - 1 - i)
				}
			}
			v = r
		}
		e[f.endpoint][f.component] |= v << f.offset
	}
	n := 2
	if m.partitionBits > 0 {
		n = 4
	}
	mask := 1<<m.endpointBits - 1
	if m.transformed {
		for i := 1; i < n; i++ {
			for c := range 3 {
				e[i][c] = (e[0][c] + signExtend(e[i][c], m.deltaBits[c])) & mask
			}
		}
	}
	for i := range n {
		for c := range 3 {
			v := e[i][c]
			if signed {
				v = signExtend(v, m.endpointBits)
			}
			e[i][c] = bc6hUnquantize(v, m.endpointBits, signed)
		}
	}
	return e, offset
}

// decodeBC6H returns texel (x, y) of a BC6H block as floats, alpha 1. A
// reserved mode decodes as (0, 0, 0, 1).
func decodeBC6H(block []byte, x, y int, signed bool) [4]float32 {
	modeNum, offset := bc6hModeNumber(block)
	m := &bc6hModes[modeNum]
	if m.reserved {
		return [4]float32{0, 0, 0, 1}
	}
	e, offset := bc6hEndpoints(m, block, offset, signed)
	subsets, partition := 1, 0
	var subsetMap uint32
	if m.partitionBits > 0 {
		partition = bits(block, offset, m.partitionBits)
		offset += m.partitionBits
		subsets, subsetMap = 2, partitions2[partition]
	}
	texel := 4*y + x
	isAnchor, before := anchor(subsets, partition, texel)
	offset += m.indexBits*texel - before
	n := m.indexBits
	if isAnchor {
		n--
	}
	index := bits(block, offset, n)
	subset := int(subsetMap >> (2 * texel) & 3)
	out := [4]float32{3: 1}
	for c := range 3 {
		v := bptcInterpolate(e[2*subset][c], e[2*subset+1][c], index, m.indexBits)
		var h uint16
		switch {
		case !signed:
			h = uint16(v * 31 / 64)
		case v < 0:
			h = uint16(-v*31/32) | 0x8000
		default:
			h = uint16(v * 31 / 32)
		}
		out[c] = halfToFloat(h)
	}
	return out
}

// halfToFloat converts IEEE 754 binary16 bits to a float32.
func halfToFloat(h uint16) float32 {
	sign := uint32(h>>15) << 31
	exp := int(h >> 10 & 0x1f)
	mant := uint32(h & 0x3ff)
	switch {
	case exp == 0x1f:
		return math.Float32frombits(sign | 0x7f800000 | mant<<13)
	case exp == 0 && mant == 0:
		return math.Float32frombits(sign)
	case exp == 0:
		// Subnormal: normalize the mantissa.
		for mant&0x400 == 0 {
			mant <<= 1
			exp--
		}
		exp++
		mant &= 0x3ff
	}
	return math.Float32frombits(sign | uint32(exp+127-15)<<23 | mant<<13)
}
