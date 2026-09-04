//go:build !(js && wasm)

package shader

import (
	"math"
	"math/bits"
)

// Pack / Unpack helpers for GLSL.std.450 opcodes 54–65.
// Component packing follows little-endian bit layout: first vector component
// occupies the least-significant bits of the result.

func packSnorm4x8(v Value) uint32 {
	c := Vec4ToFloat32(ensureVec4(v))
	var out uint32
	for i := 0; i < 4; i++ {
		s := c[i]
		if s < -1 {
			s = -1
		}
		if s > 1 {
			s = 1
		}
		n := int32(math.Round(float64(s * 127)))
		out |= uint32(uint8(int8(n))) << (8 * i)
	}
	return out
}

func packUnorm4x8(v Value) uint32 {
	c := Vec4ToFloat32(ensureVec4(v))
	var out uint32
	for i := 0; i < 4; i++ {
		s := c[i]
		if s < 0 {
			s = 0
		}
		if s > 1 {
			s = 1
		}
		n := uint32(math.Round(float64(s * 255)))
		out |= (n & 0xff) << (8 * i)
	}
	return out
}

func packSnorm2x16(v Value) uint32 {
	c := vec2Components(v)
	var out uint32
	for i := 0; i < 2; i++ {
		s := c[i]
		if s < -1 {
			s = -1
		}
		if s > 1 {
			s = 1
		}
		n := int32(math.Round(float64(s * 32767)))
		out |= uint32(uint16(int16(n))) << (16 * i)
	}
	return out
}

func packUnorm2x16(v Value) uint32 {
	c := vec2Components(v)
	var out uint32
	for i := 0; i < 2; i++ {
		s := c[i]
		if s < 0 {
			s = 0
		}
		if s > 1 {
			s = 1
		}
		n := uint32(math.Round(float64(s * 65535)))
		out |= (n & 0xffff) << (16 * i)
	}
	return out
}

func packHalf2x16(v Value) uint32 {
	c := vec2Components(v)
	lo := uint32(float32ToFloat16bits(c[0]))
	hi := uint32(float32ToFloat16bits(c[1]))
	return lo | (hi << 16)
}

func packDouble2x32(v Value) Value {
	lo, hi := uint32Pair(v)
	return ValFloat64(math.Float64frombits(uint64(lo) | uint64(hi)<<32))
}

// uint32Pair extracts two 32-bit integer components from a uvec2-like Value.
// Prefer TagArray of two uints; TagVec2 falls back to truncated float components
// (how the interpreter currently materializes integer vectors).
func uint32Pair(v Value) (lo, hi uint32) {
	switch v.Tag {
	case TagArray:
		arr := v.AsArray()
		if len(arr) >= 1 {
			lo = toUint32(arr[0])
		}
		if len(arr) >= 2 {
			hi = toUint32(arr[1])
		}
	case TagVec2, TagVec3, TagVec4:
		lo = uint32(v.F[0])
		hi = uint32(v.F[1])
	default:
		lo, hi = v.U[0], v.U[1]
	}
	return lo, hi
}

func unpackSnorm2x16(p uint32) Value {
	x := int16(p & 0xffff)
	y := int16(p >> 16)
	return ValVec2(snorm16(x), snorm16(y))
}

func unpackUnorm2x16(p uint32) Value {
	x := p & 0xffff
	y := p >> 16
	return ValVec2(float32(x)/65535, float32(y)/65535)
}

func unpackHalf2x16(p uint32) Value {
	return ValVec2(
		float16bitsToFloat32(uint16(p&0xffff)),
		float16bitsToFloat32(uint16(p>>16)),
	)
}

func unpackSnorm4x8(p uint32) Value {
	return ValVec4(
		snorm8(int8(p)),
		snorm8(int8(p>>8)),
		snorm8(int8(p>>16)),
		snorm8(int8(p>>24)),
	)
}

func unpackUnorm4x8(p uint32) Value {
	return ValVec4(
		float32(p&0xff)/255,
		float32((p>>8)&0xff)/255,
		float32((p>>16)&0xff)/255,
		float32((p>>24)&0xff)/255,
	)
}

func unpackDouble2x32(v Value) Value {
	var bits64 uint64
	switch v.Tag {
	case TagFloat64:
		bits64 = math.Float64bits(v.AsFloat64())
	default:
		bits64 = math.Float64bits(float64(toFloat32(v)))
	}
	lo := uint32(bits64)
	hi := uint32(bits64 >> 32)
	return ValArray([]Value{ValUint(lo), ValUint(hi)})
}

func snorm8(v int8) float32 {
	if v == -128 {
		return -1
	}
	return float32(v) / 127
}

func snorm16(v int16) float32 {
	if v == -32768 {
		return -1
	}
	return float32(v) / 32767
}

func ensureVec4(v Value) Value {
	switch v.Tag {
	case TagVec4:
		return v
	case TagVec3:
		return ValVec4(v.F[0], v.F[1], v.F[2], 0)
	case TagVec2:
		return ValVec4(v.F[0], v.F[1], 0, 0)
	default:
		return ValVec4(toFloat32(v), 0, 0, 0)
	}
}

func vec2Components(v Value) [2]float32 {
	switch v.Tag {
	case TagVec2, TagVec3, TagVec4:
		return [2]float32{v.F[0], v.F[1]}
	default:
		return [2]float32{toFloat32(v), 0}
	}
}

// float32ToFloat16bits converts IEEE-754 binary32 to binary16 bits.
func float32ToFloat16bits(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16((b >> 16) & 0x8000)
	exp := int((b >> 23) & 0xff)
	mant := b & 0x7fffff

	switch {
	case exp == 255: // Inf / NaN
		if mant != 0 {
			return sign | 0x7e00 // quiet NaN
		}
		return sign | 0x7c00 // Inf
	case exp > 142: // overflow → Inf
		return sign | 0x7c00
	case exp < 113: // underflow → denorm or zero
		if exp < 103 {
			return sign
		}
		mant |= 0x800000                            // 24 bits with implicit 1
		shift := uint32(126 - exp)                  // shift ∈ [14,23]
		mant = (mant + (1 << (shift - 1))) >> shift // already in 10-bit range
		return sign | uint16(mant)
	default:
		newExp := uint16(exp - 127 + 15)
		return sign | (newExp << 10) | uint16(mant>>13)
	}
}

// float16bitsToFloat32 converts IEEE-754 binary16 bits to binary32.
func float16bitsToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := int((h >> 10) & 0x1f)
	mant := uint32(h & 0x3ff)

	switch exp {
	case 0:
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		// Normalize denormal (exp is int so decrement stays well-defined).
		for mant < 0x400 {
			mant <<= 1
			exp--
		}
		mant &= 0x3ff
		exp++
		fexp := uint32(exp - 15 + 127)
		return math.Float32frombits(sign | (fexp << 23) | (mant << 13))
	case 0x1f:
		if mant != 0 {
			return math.Float32frombits(sign | 0x7fc00000)
		}
		return math.Float32frombits(sign | 0x7f800000)
	default:
		fexp := uint32(exp - 15 + 127)
		return math.Float32frombits(sign | (fexp << 23) | (mant << 13))
	}
}

// TODO: GLSL.std.450 FindILsb/FindSMsb/FindUMsb accept scalar or vector integers
// and return component-wise results. Current helpers only handle scalars.

// findILsb returns the bit number of the least-significant 1 bit, or -1 if value is 0.
func findILsb(v uint32) int32 {
	if v == 0 {
		return -1
	}
	return int32(bits.TrailingZeros32(v))
}

// findSMsb returns the bit number of the most-significant bit of a signed value.
// For positive numbers: MSB of value. For negative: MSB of ^value. Returns -1 for 0/-1.
func findSMsb(v int32) int32 {
	if v == 0 || v == -1 {
		return -1
	}
	var u uint32
	if v >= 0 {
		u = uint32(v)
	} else {
		u = uint32(^v)
	}
	return int32(31 - bits.LeadingZeros32(u))
}

// findUMsb returns the bit number of the most-significant 1 bit, or -1 if value is 0.
func findUMsb(v uint32) int32 {
	if v == 0 {
		return -1
	}
	return int32(31 - bits.LeadingZeros32(v))
}
