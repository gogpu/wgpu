// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"math"
	"math/bits"
)

// floatNotEqual implements OpFOrdNotEqual (ordered: false when either
// operand is NaN) and OpFUnordNotEqual (true when either is NaN).
func floatNotEqual(a, b float32, ordered bool) bool {
	if math.IsNaN(float64(a)) || math.IsNaN(float64(b)) {
		return !ordered
	}
	return a != b
}

// mapIntLanes applies f to every integer lane of v: a scalar keeps its tag,
// an integer vector held as a float vector or an array of scalars keeps its
// form. signed says how a float lane converts to and from its bits.
func mapIntLanes(v Value, signed bool, f func(uint32) uint32) Value {
	switch v.Tag {
	case TagVec2, TagVec3, TagVec4:
		out := v
		for i := range vectorSize(v.Tag) {
			r := f(laneBits(v.F[i], signed))
			if signed {
				out.F[i] = float32(int32(r))
			} else {
				out.F[i] = float32(r)
			}
		}
		return out
	case TagArray:
		src := v.AsArray()
		dst := make([]Value, len(src))
		for i, e := range src {
			dst[i] = mapIntLanes(e, signed, f)
		}
		return ValArray(dst)
	case TagInt32:
		return ValInt(int32(f(v.U[0])))
	default:
		return ValUint(f(toUint32(v)))
	}
}

// laneBits returns the 32-bit pattern of an integer held as a float lane.
func laneBits(f float32, signed bool) uint32 {
	if signed {
		return uint32(int32(f))
	}
	return uint32(int64(f))
}

// bitFieldMask returns count one bits from bit offset; WebGPU leaves offset +
// count > 32 to the implementation, and the bits past 31 are dropped here.
func bitFieldMask(offset, count uint32) uint32 {
	if offset >= 32 || count == 0 {
		return 0
	}
	if count >= 32 {
		return ^uint32(0) << offset
	}
	return (uint32(1)<<count - 1) << offset
}

// bitFieldInsert implements OpBitFieldInsert: base with count bits from bit
// offset replaced by the low bits of insert.
func bitFieldInsert(base, insert, offset, count uint32) uint32 {
	mask := bitFieldMask(offset, count)
	if mask == 0 {
		return base
	}
	return base&^mask | (insert<<offset)&mask
}

// bitFieldExtract implements OpBitFieldUExtract and, with signed,
// OpBitFieldSExtract (the extracted field's top bit sign-extends).
func bitFieldExtract(base, offset, count uint32, signed bool) uint32 {
	if count == 0 || offset >= 32 {
		return 0
	}
	count = min(count, 32-offset)
	field := (base >> offset) & (bitFieldMask(0, count))
	if signed && count < 32 && field&(1<<(count-1)) != 0 {
		field |= ^uint32(0) << count
	}
	return field
}

// bitCount implements OpBitCount.
func bitCount(x uint32) uint32 { return uint32(bits.OnesCount32(x)) }

// bitReverse implements OpBitReverse.
func bitReverse(x uint32) uint32 { return bits.Reverse32(x) }

// zeroDerivative returns the zero of the shape of v: what OpDPdx, OpDPdy and
// OpFwidth (and their fine and coarse forms) return. A fragment invocation
// runs alone here, not in a 2x2 quad, so there is no neighbor to take a
// difference from; the derivative of every value is taken as 0, which is
// exact for values uniform over the quad.
func zeroDerivative(v Value) Value {
	switch v.Tag {
	case TagVec2, TagVec3, TagVec4:
		return Value{Tag: v.Tag}
	default:
		return ValFloat(0)
	}
}
