// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import "math"

// Lane-wise integer, bool and comparison ops (ADR-078 rules 2 and 4). The
// instruction gives the semantics (OpSDiv divides int32, OpUDiv uint32) and
// its precomputed result shape gives the kind and lane count of the result;
// an operand's tag never chooses them. Each op is one loop over the lanes of
// the result, the same code for a scalar (one lane) and a vector.

// laneWords returns the lanes of v as raw words of kind k. An integer or
// bool scalar or vector gives its words as they are. A float scalar or
// vector, and an array of scalars, are the forms integer vectors took before
// the typed value model: their lanes convert by value (laneWord).
func laneWords(v Value, k laneKind) [4]uint32 {
	switch vectorKind(v.Tag) {
	case kindUint, kindSint, kindBool:
		return v.U
	}
	return convertedWords(v, k)
}

// convertedWords is laneWords for a float scalar or vector or an array.
func convertedWords(v Value, k laneKind) [4]uint32 {
	var w [4]uint32
	switch v.Tag {
	case TagFloat32, TagVec2, TagVec3, TagVec4:
		for i := range vectorLanes(v.Tag) {
			w[i] = laneWord(k, ValFloat(v.F[i]))
		}
	case TagArray:
		for i, e := range v.AsArray() {
			if i == len(w) {
				break
			}
			w[i] = laneWord(k, e)
		}
	}
	return w
}

// floatLanes returns the lanes of a float scalar or vector; any other
// scalar or vector converts lane by lane by value.
func floatLanes(v Value) [4]float32 {
	if vectorKind(v.Tag) == kindFloat {
		return v.F
	}
	return Vec4ToFloat32(v)
}

// valueLanes returns the lane count of a scalar, vector or array of scalars:
// the shape a result takes when its instruction has no precomputed shape.
func valueLanes(v Value) int {
	if n := vectorLanes(v.Tag); n > 0 {
		return n
	}
	if v.Tag == TagArray {
		if n := len(v.AsArray()); n >= 1 && n <= 4 {
			return n
		}
	}
	return 1
}

// shapeOr returns s, or when s is zero (an instruction built without a
// module, as unit tests do) the shape of operand a with lanes of kind k: an
// integer operand keeps its signedness when k is an integer kind.
func shapeOr(s shape, a Value, k laneKind) shape {
	if s != 0 {
		return s
	}
	if ak := vectorKind(a.Tag); (k == kindUint || k == kindSint) && (ak == kindUint || ak == kindSint) {
		k = ak
	}
	return shapeOf(k, valueLanes(a))
}

// b2u returns 1 for true and 0 for false, the word of a bool lane.
func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// intBinary implements the lane-wise integer ops of two operands that
// cannot fail: OpIAdd, OpISub, OpIMul, the bitwise ops and the shifts. The
// shift amount is taken modulo 32, as WGSL defines it.
func intBinary(opcode uint16, s shape, a, b Value) Value {
	s = shapeOr(s, a, kindUint)
	k, n := s.kind(), s.lanes()
	x, y := laneWords(a, k), laneWords(b, k)
	var r [4]uint32
	switch opcode {
	case OpIAdd:
		for i := range n {
			r[i] = x[i] + y[i]
		}
	case OpISub:
		for i := range n {
			r[i] = x[i] - y[i]
		}
	case OpIMul:
		for i := range n {
			r[i] = x[i] * y[i]
		}
	case OpBitwiseAnd:
		for i := range n {
			r[i] = x[i] & y[i]
		}
	case OpBitwiseOr:
		for i := range n {
			r[i] = x[i] | y[i]
		}
	case OpBitwiseXor:
		for i := range n {
			r[i] = x[i] ^ y[i]
		}
	case OpShiftLeftLogical:
		for i := range n {
			r[i] = x[i] << (y[i] & 31)
		}
	case OpShiftRightLogical:
		for i := range n {
			r[i] = x[i] >> (y[i] & 31)
		}
	case OpShiftRightArithmetic:
		for i := range n {
			r[i] = uint32(int32(x[i]) >> (y[i] & 31))
		}
	}
	return laneValue(k, n, r)
}

// intUnary implements OpNot and OpSNegate lane-wise (negation wraps:
// -INT_MIN is INT_MIN).
func intUnary(opcode uint16, s shape, a Value) Value {
	s = shapeOr(s, a, kindUint)
	k, n := s.kind(), s.lanes()
	x := laneWords(a, k)
	var r [4]uint32
	if opcode == OpNot {
		for i := range n {
			r[i] = ^x[i]
		}
	} else {
		for i := range n {
			r[i] = -x[i]
		}
	}
	return laneValue(k, n, r)
}

// intDivRem implements OpUDiv, OpSDiv, OpUMod, OpSMod and OpSRem lane-wise.
// SPIR-V leaves a zero divisor and INT_MIN / -1 undefined; the results here
// are WGSL's (wgsl/index.bs "Arithmetic Expressions"), as SwiftShader does:
// x / 0 = x, x % 0 = 0, INT_MIN / -1 = INT_MIN and INT_MIN % -1 = 0 (Go's
// own results for the last two).
func intDivRem(opcode uint16, s shape, a, b Value) Value {
	s = shapeOr(s, a, kindUint)
	k, n := s.kind(), s.lanes()
	x, y := laneWords(a, k), laneWords(b, k)
	var r [4]uint32
	for i := range n {
		r[i] = divRemWord(opcode, x[i], y[i])
	}
	return laneValue(k, n, r)
}

// divRemWord is one lane of intDivRem.
func divRemWord(opcode uint16, x, y uint32) uint32 {
	switch opcode {
	case OpUDiv:
		if y == 0 {
			return x
		}
		return x / y
	case OpSDiv:
		if y == 0 {
			return x
		}
		return uint32(int32(x) / int32(y))
	case OpUMod:
		if y == 0 {
			return 0
		}
		return x % y
	case OpSRem:
		if y == 0 {
			return 0
		}
		return uint32(int32(x) % int32(y))
	case OpSMod:
		// SMod takes the sign of the divisor, SRem the sign of the dividend.
		if y == 0 {
			return 0
		}
		rem := int32(x) % int32(y)
		if rem != 0 && (rem < 0) != (int32(y) < 0) {
			rem += int32(y)
		}
		return uint32(rem)
	}
	return 0
}

// intCompare implements the integer comparisons lane-wise; the result is a
// bool scalar or vector.
func intCompare(opcode uint16, s shape, a, b Value) Value {
	n := shapeOr(s, a, kindBool).lanes()
	x, y := laneWords(a, kindUint), laneWords(b, kindUint)
	var r [4]uint32
	for i := range n {
		r[i] = b2u(compareWord(opcode, x[i], y[i]))
	}
	return laneValue(kindBool, n, r)
}

// compareWord is one lane of intCompare.
func compareWord(opcode uint16, x, y uint32) bool {
	switch opcode {
	case OpIEqual:
		return x == y
	case OpINotEqual:
		return x != y
	case OpULessThan:
		return x < y
	case OpUGreaterThan:
		return x > y
	case OpULessThanEqual:
		return x <= y
	case OpUGreaterThanEqual:
		return x >= y
	case OpSLessThan:
		return int32(x) < int32(y)
	case OpSGreaterThan:
		return int32(x) > int32(y)
	case OpSLessThanEqual:
		return int32(x) <= int32(y)
	case OpSGreaterThanEqual:
		return int32(x) >= int32(y)
	}
	return false
}

// floatCompare implements the float comparisons lane-wise; the result is a
// bool scalar or vector. An ordered comparison is false when either lane is
// NaN (Go's comparisons already are), an unordered one true.
func floatCompare(opcode uint16, s shape, a, b Value) Value {
	n := shapeOr(s, a, kindBool).lanes()
	x, y := floatLanes(a), floatLanes(b)
	var r [4]uint32
	for i := range n {
		r[i] = b2u(compareFloat(opcode, x[i], y[i]))
	}
	return laneValue(kindBool, n, r)
}

// compareFloat is one lane of floatCompare.
func compareFloat(opcode uint16, x, y float32) bool {
	switch opcode {
	case OpFOrdEqual:
		return x == y
	case OpFOrdLessThan:
		return x < y
	case OpFOrdGreaterThan:
		return x > y
	case OpFOrdLessThanEqual:
		return x <= y
	case OpFOrdGreaterThanEqual:
		return x >= y
	case OpFOrdNotEqual:
		return floatNotEqual(x, y, true)
	case OpFUnordNotEqual:
		return floatNotEqual(x, y, false)
	}
	return false
}

// logicalOp implements OpLogicalAnd, OpLogicalOr, OpLogicalEqual,
// OpLogicalNotEqual and (b unused) OpLogicalNot lane-wise.
func logicalOp(opcode uint16, s shape, a, b Value) Value {
	n := shapeOr(s, a, kindBool).lanes()
	x, y := laneWords(a, kindBool), laneWords(b, kindBool)
	var r [4]uint32
	for i := range n {
		p, q := x[i] != 0, y[i] != 0
		switch opcode {
		case OpLogicalAnd:
			r[i] = b2u(p && q)
		case OpLogicalOr:
			r[i] = b2u(p || q)
		case OpLogicalEqual:
			r[i] = b2u(p == q)
		case OpLogicalNotEqual:
			r[i] = b2u(p != q)
		case OpLogicalNot:
			r[i] = b2u(!p)
		}
	}
	return laneValue(kindBool, n, r)
}

// allAny implements OpAll (all set) and OpAny: the AND or the OR of the lanes
// of a bool vector. SPIR-V requires a vector operand; a bool scalar counts as
// one lane.
func allAny(all bool, v Value) Value {
	lanes := laneWords(v, kindBool)
	for i := range valueLanes(v) {
		if (lanes[i] != 0) != all {
			// A false lane decides OpAll, a true lane decides OpAny.
			return ValBool(!all)
		}
	}
	return ValBool(all)
}

// selectLanes implements OpSelect. A bool scalar condition picks a whole
// object (any type); a bool vector condition picks each lane of two vectors
// (SPIR-V: the condition then has the result's lane count).
func selectLanes(cond, t, f Value) Value {
	if !isWordVector(cond.Tag) {
		if toBool(cond) {
			return t
		}
		return f
	}
	out := f
	for i := range vectorLanes(cond.Tag) {
		if cond.U[i] != 0 {
			// A 32-bit vector keeps its lanes in F or U and zeros in the
			// other, so copying both copies the lane of any kind.
			out.F[i], out.U[i] = t.F[i], t.U[i]
		}
	}
	return out
}

// texelResult returns a decoded texel as the result shape s of the image
// instruction: a float vector, or for an integer texture an integer vector
// of the same channel values. texel is the decoded vec4 the image functions
// return.
//
// An integer texel is decoded through float32 (readTexel), so a channel is
// exact up to 2^24; a wider channel (R32Uint, R32Sint) loses its low bits.
func texelResult(s shape, texelVal Value) Value {
	k := s.kind()
	if k != kindUint && k != kindSint {
		return texelVal
	}
	texel := texelVal.F
	n := max(s.lanes(), 1)
	var w [4]uint32
	for i := range n {
		if k == kindSint {
			w[i] = uint32(floatToInt32(texel[i]))
		} else {
			w[i] = floatToUint32(texel[i])
		}
	}
	return laneValue(k, n, w)
}

// lanePool collects the lanes of the operands of OpVectorShuffle and
// OpCompositeConstruct as raw words of kind k: an f32 lane as its bits.
type lanePool struct {
	w [8]uint32
	n int
}

// add appends the lanes of v: a scalar gives one, a vector each of its
// lanes, an array of scalars (the old integer vector form) each element.
func (p *lanePool) add(v Value, k laneKind) {
	vk := vectorKind(v.Tag)
	switch {
	case vk == kindFloat && k == kindFloat:
		for i := range vectorLanes(v.Tag) {
			p.put(math.Float32bits(v.F[i]))
		}
	case vk != kindNone && vk != kindFloat && k != kindFloat:
		for i := range vectorLanes(v.Tag) {
			p.put(v.U[i])
		}
	case vk != kindNone:
		for i := range vectorLanes(v.Tag) {
			p.put(laneWord(k, indexLane(v, i)))
		}
	case v.Tag == TagArray:
		for _, e := range v.AsArray() {
			p.put(laneWord(k, e))
		}
	default:
		p.put(0)
	}
}

// put appends one word; lanes past the eighth are dropped.
func (p *lanePool) put(w uint32) {
	if p.n < len(p.w) {
		p.w[p.n] = w
		p.n++
	}
}

// indexLane returns lane i of a scalar (itself) or vector as a scalar.
func indexLane(v Value, i int) Value {
	if vectorLanes(v.Tag) == 1 {
		return v
	}
	return indexComposite(v, uint32(i))
}

// wordsValue returns the first lanes of words w as a value of shape s; an
// f32 lane takes its bits.
func wordsValue(s shape, w [4]uint32) Value {
	if s.kind() != kindFloat {
		return laneValue(s.kind(), s.lanes(), w)
	}
	v := Value{Tag: s.tag()}
	for i := range s.lanes() {
		v.F[i] = math.Float32frombits(w[i])
	}
	return v
}
