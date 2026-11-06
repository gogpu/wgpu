// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"encoding/binary"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/software/raster"
)

// sRGB transfer function (IEC 61966-2-1, the curve WebGPU's *-srgb formats
// use): an 8-bit sRGB channel decodes to linear through srgbToLinear8, and a
// linear value encodes to the nearest 8-bit code through linearToSrgb8.
var (
	srgbToLinear8 = buildSRGBDecode()
	// srgbThresholds[n] is the linear value whose encoding lies halfway
	// between codes n and n+1: a linear value l encodes to the number of
	// thresholds not above it, which is round(encode(l) * 255) without a
	// pow per channel.
	srgbThresholds = buildSRGBThresholds()
)

func srgbDecode(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func buildSRGBDecode() [256]float32 {
	var t [256]float32
	for n := range t {
		t[n] = float32(srgbDecode(float64(n) / 255))
	}
	return t
}

func buildSRGBThresholds() [255]float32 {
	var t [255]float32
	for n := range t {
		t[n] = float32(srgbDecode((float64(n) + 0.5) / 255))
	}
	return t
}

// linearToSrgb8 encodes a linear channel value to an 8-bit sRGB code,
// clamping to [0, 1] first (NaN encodes as 0).
func linearToSrgb8(l float32) byte {
	if !(l > 0) {
		return 0
	}
	if l >= 1 {
		return 255
	}
	// The first threshold above l, by binary search over the 255.
	lo, hi := 0, len(srgbThresholds)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if srgbThresholds[mid] > l {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return byte(lo)
}

// encodePacked encodes v into a packed 32-bit texel of kind, red in the low
// bits, the inverse of decodePacked. rgb9e5ufloat is never written: it is
// neither renderable nor a storage format.
func encodePacked(v Vec4, kind texelKind) uint32 {
	switch kind {
	case texelRGB10A2Unorm:
		return raster.FloatToUnorm(v[0], 1023) | raster.FloatToUnorm(v[1], 1023)<<10 |
			raster.FloatToUnorm(v[2], 1023)<<20 | raster.FloatToUnorm(v[3], 3)<<30
	case texelRGB10A2Uint:
		return uint32(int64(v[0]))&0x3FF | uint32(int64(v[1]))&0x3FF<<10 |
			uint32(int64(v[2]))&0x3FF<<20 | uint32(int64(v[3]))&0x3<<30
	case texelRG11B10Ufloat:
		return encodeUnsignedSmallFloat(v[0], 6) | encodeUnsignedSmallFloat(v[1], 6)<<11 |
			encodeUnsignedSmallFloat(v[2], 5)<<22
	}
	return 0
}

// encodeUnsignedSmallFloat encodes f as an unsigned float with a 5-bit
// exponent (bias 15) above a mantBits-bit mantissa, rounding to nearest
// even, the inverse of unsignedSmallFloat. As D3D and Vulkan convert:
// negative values and -0 become 0, NaN stays NaN, +Inf stays +Inf, and
// finite values above the largest finite value become it.
func encodeUnsignedSmallFloat(f float32, mantBits int) uint32 {
	const expInfNaN = 31
	switch {
	case math.IsNaN(float64(f)):
		return expInfNaN<<mantBits | 1<<(mantBits-1)
	case math.IsInf(float64(f), 1):
		return expInfNaN << mantBits
	case !(f > 0):
		return 0
	}
	maxFinite := uint32(30<<mantBits | (1<<mantBits - 1))
	frac, exp := math.Frexp(float64(f)) // f = frac * 2^exp, frac in [0.5, 1)
	e := exp - 1                        // f = (2 frac) * 2^e
	if e < -14 {
		// Denormal: f / 2^(-14 - mantBits), rounded. A result of
		// 1<<mantBits is the smallest normal, exponent field 1.
		return uint32(math.RoundToEven(math.Ldexp(float64(f), 14+mantBits)))
	}
	m := uint32(math.RoundToEven((2*frac - 1) * float64(uint32(1)<<mantBits)))
	if m == 1<<mantBits {
		m, e = 0, e+1
	}
	if e+15 >= expInfNaN {
		return maxFinite
	}
	return uint32(e+15)<<mantBits | m
}

// ColorCodec reads and writes the texels of one colour attachment format
// in linear values: the colour writes and clears of the software
// rasterizer go through it. An sRGB format decodes and encodes the sRGB
// transfer function on red, green and blue; alpha is linear.
type ColorCodec struct {
	layout sampledFormat
}

// NewColorCodec returns the codec of colour format f, or false for a format
// that is not an uncompressed colour format the backend stores
// (depth-stencil, compressed, rgb9e5ufloat, Undefined).
func NewColorCodec(f gputypes.TextureFormat) (ColorCodec, bool) {
	if f.IsDepthStencil() || f == gputypes.TextureFormatRGB9E5Ufloat || uint64(f) >= uint64(len(sampledLayouts)) {
		return ColorCodec{}, false
	}
	l := sampledLayouts[f]
	if l.channels == 0 {
		return ColorCodec{}, false
	}
	return ColorCodec{layout: l}, true
}

// TexelSize returns the bytes of one texel.
func (c ColorCodec) TexelSize() int { return c.layout.texelStride() }

// Integer reports whether the format stores integers: such targets never
// blend (WebGPU rejects a blend state on them).
func (c ColorCodec) Integer() bool {
	switch c.layout.kind {
	case texelUint, texelSint, texelRGB10A2Uint:
		return true
	}
	return false
}

// Signed reports whether the format stores signed integers.
func (c ColorCodec) Signed() bool { return c.layout.kind == texelSint }

// EncodeIntMasked writes the integer channels v, each the 32 bits of the
// shader's u32 or i32 output, into the texel at b[0] of an integer format,
// the channels mask selects only. A narrower channel keeps the low bits, as
// the float path (encodeAt) truncates; a 32-bit channel stores v exactly,
// which a float32 cannot carry above 2^24.
func (c ColorCodec) EncodeIntMasked(b []byte, v [4]uint32, mask gputypes.ColorWriteMask) {
	l := c.layout
	if l.kind == texelRGB10A2Uint {
		w := v[0]&0x3FF | v[1]&0x3FF<<10 | v[2]&0x3FF<<20 | v[3]&3<<30
		keep := ^packedChannelBits(l.kind, mask)
		binary.LittleEndian.PutUint32(b, binary.LittleEndian.Uint32(b)&keep|w&^keep)
		return
	}
	for ch := range l.channels {
		if mask&(1<<ch) == 0 {
			continue
		}
		dst := b[ch*l.bytes:]
		switch l.bytes {
		case 1:
			dst[0] = byte(v[ch])
		case 2:
			binary.LittleEndian.PutUint16(dst, uint16(v[ch]))
		default:
			binary.LittleEndian.PutUint32(dst, v[ch])
		}
	}
}

// Range returns the range a normalized format clamps blend inputs and
// results to: [0, 1] for unorm (sRGB included), [-1, 1] for snorm. ok is
// false for float and integer formats, which WebGPU does not clamp.
func (c ColorCodec) Range() (lo, hi float32, ok bool) {
	switch c.layout.kind {
	case texelUnorm, texelRGB10A2Unorm:
		return 0, 1, true
	case texelSnorm:
		return -1, 1, true
	}
	return 0, 0, false
}

// Decode reads the texel at b[0] as linear RGBA, the channels the format
// lacks filled with (0, 0, 0, 1).
func (c ColorCodec) Decode(b []byte) [4]float32 { return decodeAt(b, c.layout) }

// Encode writes linear RGBA v into the texel at b[0].
func (c ColorCodec) Encode(b []byte, v [4]float32) { encodeAt(b, c.layout, v) }

// EncodeMasked writes the channels of v that mask (gputypes.ColorWriteMask
// bits: red 1, green 2, blue 4, alpha 8) selects into the texel at b[0] and
// keeps the stored bits of the others exactly.
func (c ColorCodec) EncodeMasked(b []byte, v [4]float32, mask gputypes.ColorWriteMask) {
	if mask&gputypes.ColorWriteMaskAll == gputypes.ColorWriteMaskAll {
		c.Encode(b, v)
		return
	}
	if mask&gputypes.ColorWriteMaskAll == 0 {
		return
	}
	l := c.layout
	if l.kind >= texelRGB10A2Unorm {
		old := binary.LittleEndian.Uint32(b)
		keep := ^packedChannelBits(l.kind, mask)
		binary.LittleEndian.PutUint32(b, old&keep|encodePacked(v, l.kind)&^keep)
		return
	}
	var tmp [16]byte
	encodeAt(tmp[:], l, v)
	for ch := range l.channels {
		// Storage channel ch holds blue when the format is BGRA and ch is 0,
		// red when ch is 2.
		logical := ch
		if l.bgra && ch != 1 && ch != 3 {
			logical = 2 - ch
		}
		if mask&(1<<logical) != 0 {
			copy(b[ch*l.bytes:(ch+1)*l.bytes], tmp[ch*l.bytes:(ch+1)*l.bytes])
		}
	}
}

// packedChannelBits returns the bits of a packed texel of kind that the
// channels in mask occupy.
func packedChannelBits(kind texelKind, mask gputypes.ColorWriteMask) uint32 {
	var widths [4]uint
	switch kind {
	case texelRGB10A2Unorm, texelRGB10A2Uint:
		widths = [4]uint{10, 10, 10, 2}
	default: // texelRG11B10Ufloat has no alpha
		widths = [4]uint{11, 11, 10, 0}
	}
	var bits uint32
	shift := uint(0)
	for ch, w := range widths {
		if mask&(1<<ch) != 0 {
			bits |= (1<<w - 1) << shift
		}
		shift += w
	}
	return bits
}

// EncodeClear writes the clear value v, given in float64 as
// GPURenderPassColorAttachment.clearValue is, into the texel at b[0]. The
// 32-bit integer formats convert from float64 directly, so every uint32 and
// int32 clears exactly; the other formats convert as Encode.
func (c ColorCodec) EncodeClear(b []byte, v [4]float64) {
	l := c.layout
	if (l.kind == texelUint || l.kind == texelSint) && l.bytes == 4 {
		for ch := range l.channels {
			var bits uint32
			if l.kind == texelUint {
				bits = uint32(int64(v[ch]))
			} else {
				bits = uint32(int32(int64(v[ch])))
			}
			binary.LittleEndian.PutUint32(b[ch*4:], bits)
		}
		return
	}
	c.Encode(b, [4]float32{float32(v[0]), float32(v[1]), float32(v[2]), float32(v[3])})
}
