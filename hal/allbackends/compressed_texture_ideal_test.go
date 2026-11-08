// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"

	"github.com/gogpu/gputypes"
)

// The ideal decode of the BC1-BC5 formats, and how far two decoders of a
// texel may differ.
//
// The ideal values are the formulas of the Khronos Data Format Specification
// 1.3 ("S3TC Compressed Texture Image Formats", "RGTC Compressed Texture
// Image Formats"), computed in float64. They classify each texel (constant,
// endpoint or interpolated, and its endpoints) and give the second value of
// the decodes the specification leaves to the implementation; texels are
// compared with the software decoder, not with the ideal:
//   - The Direct3D 11.3 Functional Specification (section 19.5, "BC Formats")
//     bounds BC1 palette entries, BC2 and BC3 colors and BC2 alpha by
//     absolute_error = 1.0/255.0, and "Values specified as 0.0f or 1.0f must
//     be exact". GPUs round the interpolated entries differently: WARP, Intel
//     Iris Xe and the software decoder (bit-exact with Intel) differ by one
//     8-bit code.
//   - It bounds BC4U, BC5U and BC3 alpha by 1.0/65535.0 and BC4S, BC5S by
//     1.0/32767.0 against the ideal, with the endpoints exact. Intel Iris Xe,
//     a conformant GPU, does not meet that (its BC3 alpha is decoded to 8
//     bits, 0.28/255 off the ideal; its BC4 unorm entries are up to 1.8e-5
//     off, its BC4 snorm endpoints 4e-6), so the test cannot hold GPUs to
//     it. These formats use one code of their 8-bit endpoints as well:
//     1/255 unorm, 1/127 snorm.
// -1, 0 and 1 compare exactly.

const (
	// bcUnorm8Error is one 8-bit unorm code.
	bcUnorm8Error = 1.0 / 255
	// bcSnorm8Error is one 8-bit snorm code.
	bcSnorm8Error = 1.0 / 127
	// bcExactError is what "must be exact" allows of a float32 result: the
	// nearest float32 of the value, or one computed as k * (1/n), two ulps
	// at most below 1.
	bcExactError = 2.5e-7
)

// idealChannel is the ideal value of one channel of a texel.
type idealChannel struct {
	want float64
	tol  float64
	// alt, when hasAlt is set, is a second value the specification allows
	// (a decode the specification leaves to the implementation).
	alt    float64
	hasAlt bool
	// interp marks a value interpolated between the endpoints lo and hi,
	// as opposed to an endpoint or a constant.
	interp bool
	lo, hi float64
}

// idealTexel is the ideal RGBA of one texel.
type idealTexel [4]idealChannel

func exactChannel(v float64) idealChannel { return idealChannel{want: v, tol: bcExactError} }

// idealBC returns the ideal decode of texel (x, y) of a block of format f,
// and false for a format without a closed form here (BC6H, BC7, ETC2, EAC).
func idealBC(f gputypes.TextureFormat, b []byte, x, y int) (idealTexel, bool) {
	t := 4*y + x
	switch f {
	case gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb:
		return idealBC1Color(b[0:8], t, true), true
	case gputypes.TextureFormatBC2RGBAUnorm, gputypes.TextureFormatBC2RGBAUnormSrgb:
		tx := idealBC1Color(b[8:16], t, false)
		a := float64(binary.LittleEndian.Uint64(b[0:8])>>(4*t)&0xF) / 15
		tx[3] = idealChannel{want: a, tol: bcUnorm8Error}
		if a == 0 || a == 1 {
			tx[3].tol = bcExactError
		}
		return tx, true
	case gputypes.TextureFormatBC3RGBAUnorm, gputypes.TextureFormatBC3RGBAUnormSrgb:
		tx := idealBC1Color(b[8:16], t, false)
		tx[3] = idealBC4(b[0:8], t, false)
		return tx, true
	case gputypes.TextureFormatBC4RUnorm, gputypes.TextureFormatBC4RSnorm:
		snorm := f == gputypes.TextureFormatBC4RSnorm
		return idealTexel{idealBC4(b, t, snorm), exactChannel(0), exactChannel(0), exactChannel(1)}, true
	case gputypes.TextureFormatBC5RGUnorm, gputypes.TextureFormatBC5RGSnorm:
		snorm := f == gputypes.TextureFormatBC5RGSnorm
		return idealTexel{idealBC4(b[0:8], t, snorm), idealBC4(b[8:16], t, snorm), exactChannel(0), exactChannel(1)}, true
	}
	return idealTexel{}, false
}

// idealBC1Color decodes texel t of a BC1 color block. bc1 selects the BC1
// rule: color0 <= color1 is the three-color mode with transparent black.
// BC2 and BC3 always use the four-color mode (KDFS "BC2": "treated as
// though color0 > color1, regardless of the actual values").
func idealBC1Color(b []byte, t int, bc1 bool) idealTexel {
	c0, c1 := binary.LittleEndian.Uint16(b[0:2]), binary.LittleEndian.Uint16(b[2:4])
	code := binary.LittleEndian.Uint32(b[4:8]) >> (2 * t) & 3
	rgb := func(c uint16) [3]float64 {
		return [3]float64{float64(c>>11) / 31, float64(c>>5&63) / 63, float64(c&31) / 31}
	}
	e0, e1 := rgb(c0), rgb(c1)
	var tx idealTexel
	tx[3] = exactChannel(1)
	four := !bc1 || c0 > c1
	if !four && code == 3 {
		for i := range 3 {
			tx[i] = exactChannel(0)
		}
		tx[3] = exactChannel(0)
		return tx
	}
	for i := range 3 {
		var v float64
		interp := code >= 2
		switch {
		case code == 0:
			v = e0[i]
		case code == 1:
			v = e1[i]
		case four && code == 2:
			v = (2*e0[i] + e1[i]) / 3
		case four:
			v = (e0[i] + 2*e1[i]) / 3
		default:
			v = (e0[i] + e1[i]) / 2
		}
		tx[i] = idealChannel{want: v, tol: bcUnorm8Error, interp: interp, lo: min(e0[i], e1[i]), hi: max(e0[i], e1[i])}
		if v == 0 || v == 1 {
			tx[i].tol = bcExactError
		}
	}
	return tx
}

// idealBC4 decodes texel t of a BC4 block (KDFS "BC4 unsigned", "BC4
// signed"). For a signed block with red0 = -127 and red1 = -128 the mode is
// left to the implementation (KDFS "BC4 signed", CAVEAT: the comparison "is
// considered undefined (read: may vary by implementation)"), so the value of
// the other mode is allowed too.
func idealBC4(b []byte, t int, snorm bool) idealChannel {
	code := int(binary.LittleEndian.Uint64(append(b[2:8:8], 0, 0)) >> (3 * t) & 7)
	if !snorm {
		r0, r1 := float64(b[0])/255, float64(b[1])/255
		return bc4Palette(r0, r1, b[0] > b[1], code, 0, 1, bcUnorm8Error)
	}
	s0, s1 := int8(b[0]), int8(b[1])
	red := func(s int8) float64 { return max(float64(s)/127, -1) }
	r0, r1 := red(s0), red(s1)
	ch := bc4Palette(r0, r1, s0 > s1, code, -1, 1, bcSnorm8Error)
	if s0 == -127 && s1 == -128 {
		other := bc4Palette(r0, r1, false, code, -1, 1, bcSnorm8Error)
		ch.alt, ch.hasAlt = other.want, true
	}
	return ch
}

// bc4Palette returns palette entry code of a BC4 block with endpoints r0 and
// r1, in the eight-value mode when eight is set and in the six-value mode
// (with rmin and rmax) otherwise.
func bc4Palette(r0, r1 float64, eight bool, code int, rmin, rmax, tol float64) idealChannel {
	switch {
	case code == 0:
		return exactChannel(r0)
	case code == 1:
		return exactChannel(r1)
	case eight:
		v := (float64(8-code)*r0 + float64(code-1)*r1) / 7
		return idealChannel{want: v, tol: tol, interp: true, lo: min(r0, r1), hi: max(r0, r1)}
	case code == 6:
		return exactChannel(rmin)
	case code == 7:
		return exactChannel(rmax)
	default:
		v := (float64(6-code)*r0 + float64(code-1)*r1) / 5
		return idealChannel{want: v, tol: tol, interp: true, lo: min(r0, r1), hi: max(r0, r1)}
	}
}
