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
// vectors: the source's bits under the result type, tagged by it, so later
// stores and arithmetic see an integer as an integer and a float as a
// float. An integer vector result is an array of integer scalars, exact for
// every bit pattern. Any other bitcast (pointers, other widths, a source
// whose type is unknown) copies the value, as all of them did before.
func (interp *interpreter) bitcast(v Value, srcTypeID, dstTypeID uint32) Value {
	src, n := interp.scalarLanes(srcTypeID)
	dst, dstN := interp.scalarLanes(dstTypeID)
	if src == nil || dst == nil || n != dstN {
		return v
	}
	if n == 1 {
		return bitsAs(valueBits(v, src), dst)
	}
	lanes := make([]Value, n)
	for i := range lanes {
		lanes[i] = bitsAs(valueBits(indexComposite(v, uint32(i)), src), dst)
	}
	if dst.Kind != TypeFloat {
		return ValArray(lanes)
	}
	out := Value{Tag: vectorTag(n)}
	for i, l := range lanes {
		out.F[i] = l.F[0]
	}
	return out
}

// convertLanes applies the scalar conversion conv to every component of v,
// whose result type is typeID: OpConvertFToS, OpConvertFToU, OpConvertSToF
// and OpConvertUToF are component-wise. A float vector result is a float
// vector; an integer one is an array of integer scalars, exact for every
// value. A scalar, or a result type that is not a 32-bit vector, converts v
// as a whole.
func (interp *interpreter) convertLanes(v Value, typeID uint32, conv func(Value) Value) Value {
	dst, n := interp.scalarLanes(typeID)
	if dst == nil || n == 1 {
		return conv(v)
	}
	lanes := make([]Value, n)
	for i := range lanes {
		lanes[i] = conv(indexComposite(v, uint32(i)))
	}
	if dst.Kind != TypeFloat {
		return ValArray(lanes)
	}
	out := Value{Tag: vectorTag(n)}
	for i, l := range lanes {
		out.F[i] = toFloat32(l)
	}
	return out
}

// scalarLanes returns the 32-bit integer or float scalar type of a scalar
// or vector type and its lane count, or nil for any other type.
func (interp *interpreter) scalarLanes(typeID uint32) (*TypeInfo, int) {
	ti := interp.typeInfo(typeID)
	n := 1
	if ti != nil && ti.Kind == TypeVector {
		n = int(ti.Components)
		ti = interp.typeInfo(ti.ElemType)
	}
	if ti == nil || ti.Width != 32 || (ti.Kind != TypeInt && ti.Kind != TypeFloat) || n < 1 || n > 4 {
		return nil, 0
	}
	return ti, n
}

// valueBits returns the 32-bit pattern of a scalar value of type ti; a
// float32 standing for an integer (a component of an integer vector) gives
// the integer's bits.
func valueBits(v Value, ti *TypeInfo) uint32 {
	switch v.Tag {
	case TagUint32, TagInt32:
		return v.U[0]
	case TagFloat32:
		if ti.Kind == TypeInt {
			return laneBits(v.F[0], ti.Signed)
		}
		return math.Float32bits(v.F[0])
	default:
		return 0
	}
}

// bitsAs returns a 32-bit pattern as a scalar of type ti.
func bitsAs(pattern uint32, ti *TypeInfo) Value {
	switch {
	case ti.Kind == TypeFloat:
		return ValFloat(math.Float32frombits(pattern))
	case ti.Signed:
		return ValInt(int32(pattern))
	default:
		return ValUint(pattern)
	}
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
