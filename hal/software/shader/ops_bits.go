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

// mapIntLanes applies f to every lane of the integer value v; the result has
// shape s (or, when s is zero, the shape of v).
func mapIntLanes(s shape, v Value, f func(uint32) uint32) Value {
	s = shapeOr(s, v, kindUint)
	k, n := s.kind(), s.lanes()
	w := laneWords(v, k)
	for i := range n {
		w[i] = f(w[i])
	}
	return laneValue(k, n, w)
}

// laneBits returns the 32-bit pattern of an integer held as a float lane,
// saturated to the range of the type: float32 rounds the largest u32 and
// i32 values past it (0xFFFFFFFF to 2^32), and a plain conversion would wrap
// them or leave them implementation-defined. No float-held integer is NaN;
// NaN gives 0.
func laneBits(f float32, signed bool) uint32 {
	d := float64(f)
	if d != d {
		return 0
	}
	if signed {
		return uint32(int32(max(math.MinInt32, min(math.MaxInt32, d))))
	}
	return uint32(max(0, min(math.MaxUint32, d)))
}

// bitcast implements OpBitcast between 32-bit integer and float scalars and
// vectors: the source's bits under the result shape s (or, when s is zero,
// the shape of dstTypeID), so later stores and arithmetic see an integer as
// an integer and a float as a float, exact for every bit pattern. Any other
// bitcast (pointers, bools, other widths, a source whose type is unknown, a
// lane count change) copies the value.
func (interp *interpreter) bitcast(v Value, srcTypeID uint32, s shape, dstTypeID uint32) Value {
	if s == 0 {
		s = interp.module.typeShape(dstTypeID)
	}
	src := interp.module.typeShape(srcTypeID)
	if src == 0 || s == 0 || src.kind() == kindBool || s.kind() == kindBool || src.lanes() != s.lanes() {
		return v
	}
	var w [4]uint32
	if src.kind() == kindFloat {
		f := floatLanes(v)
		for i := range src.lanes() {
			w[i] = math.Float32bits(f[i])
		}
	} else {
		w = laneWords(v, src.kind())
	}
	return wordsValue(s, w)
}

// convertLanes applies the scalar conversion conv to every lane of v, with
// result shape s: OpConvertFToS, OpConvertFToU, OpConvertSToF and
// OpConvertUToF are lane-wise. A scalar, or a result without a shape (f16,
// f64), converts v as a whole.
func convertLanes(v Value, s shape, conv func(Value) Value) Value {
	n := s.lanes()
	if n <= 1 {
		return conv(v)
	}
	k := s.kind()
	var w [4]uint32
	for i := range n {
		w[i] = laneWord(k, conv(indexLane(v, i)))
	}
	return wordsValue(s, w)
}

// vectorTag returns the tag of an n-component float vector.
func vectorTag(n int) ValueTag {
	switch n {
	case 2:
		return TagVec2
	case 3:
		return TagVec3
	case 4:
		return TagVec4
	default:
		return TagFloat32
	}
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
