// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import "math"

// Numeric ops naga emits for WGSL builtins the interpreter used to skip
// (ADR-078 step 6): the vector-times-matrix product, float classification,
// quantizeToF16 and the packed 4x8-bit dot products.

// floatVector returns the f32 scalar or vector of the first n lanes of f.
// Lanes past n are cleared. It returns ValNone for n outside 1..4.
func floatVector(n int, f [4]float32) Value {
	tag := laneTag(kindFloat, n)
	if tag == TagNone {
		return Value{}
	}
	v := Value{Tag: tag}
	copy(v.F[:n], f[:n])
	return v
}

// vectorTimesMatrix is vec * mat (OpVectorTimesMatrix): mat has C columns
// of R components and vec R components, and lane i of the result is
// dot(vec, column i), so the result has C lanes (vecR * matCxR = vecC).
func vectorTimesMatrix(vec, mat Value) Value {
	if mat.Tag != TagArray {
		return Value{}
	}
	cols := mat.AsArray()
	if len(cols) < 2 || len(cols) > 4 {
		return Value{}
	}
	x := floatLanes(vec)
	rows := vectorLanes(cols[0].Tag)
	var r [4]float32
	for i, c := range cols {
		col := floatLanes(c)
		for j := range rows {
			r[i] += x[j] * col[j]
		}
	}
	return floatVector(len(cols), r)
}

// floatClass implements OpIsNan and OpIsInf lane-wise: a bool lane per lane
// of the float operand.
func floatClass(opcode uint16, s shape, a Value) Value {
	n := shapeOr(s, a, kindBool).lanes()
	x := floatLanes(a)
	var r [4]uint32
	for i := range n {
		f := float64(x[i])
		if opcode == OpIsNan {
			r[i] = b2u(math.IsNaN(f))
		} else {
			r[i] = b2u(math.IsInf(f, 0))
		}
	}
	return laneValue(kindBool, n, r)
}

// f16 limits as float32 bit patterns (magnitudes): the smallest normal
// binary16 value 2^-14, and 2^16, the first magnitude past the binary16
// range that RNE rounding reaches (from 65520 up; 65504 is the largest
// finite value).
const (
	f16MinNormalBits = 0x38800000
	f16OverflowBits  = 0x47800000
	f32ExpMaskBits   = 0x7F800000
	f32SignBit       = 0x80000000
)

// quantizeToF16 rounds f to the nearest binary16 value, ties to even, and
// returns it as a float32 (OpQuantizeToF16, WGSL quantizeToF16). The SPIR-V
// rules decide the edges: an infinity or NaN stays as it is; a finite value
// too large for binary16 becomes the infinity of its sign; a value too small
// for a normal binary16 may become either zero, and becomes the zero of its
// sign here. WGSL leaves the result of an out-of-range runtime value
// indeterminate, so the SPIR-V result is a valid one.
func quantizeToF16(f float32) float32 {
	bits := math.Float32bits(f)
	sign, mag := bits&f32SignBit, bits&^f32SignBit
	if mag >= f32ExpMaskBits { // infinity or NaN
		return f
	}
	if mag < f16MinNormalBits {
		return math.Float32frombits(sign)
	}
	// Round the 23-bit mantissa to binary16's 10 bits, ties to even: add
	// just under half of the dropped 13 bits, plus the kept lowest bit.
	mag += 0x0FFF + (mag>>13)&1
	mag &^= 0x1FFF
	if mag >= f16OverflowBits {
		return math.Float32frombits(sign | f32ExpMaskBits)
	}
	return math.Float32frombits(sign | mag)
}

// integerDot evaluates OpSDotKHR or OpUDotKHR. With the
// PackedVectorFormat4x8Bit operand both operands are u32 words of four
// packed bytes (dot4Packed); without it they are integer vectors, and the
// result is the wrapping sum of the lane products, the same 32 bits for
// either signedness.
func (interp *interpreter) integerDot(inst Instruction) Value {
	a, b := interp.values[inst.Operands[0]], interp.values[inst.Operands[1]]
	signed := inst.Opcode == OpSDotKHR
	if len(inst.Operands) >= 3 && inst.Operands[2] == packedVectorFormat4x8Bit {
		return dot4Packed(toUint32(a), toUint32(b), signed)
	}
	k := kindUint
	if signed {
		k = kindSint
	}
	x, y := laneWords(a, k), laneWords(b, k)
	var sum uint32
	for i := range valueLanes(a) {
		sum += x[i] * y[i]
	}
	return scalarOfKind(k, sum)
}

// packedVectorFormat4x8Bit is the SPIR-V PackedVectorFormat operand of a
// dot product whose operands are 32-bit words of four 8-bit lanes.
const packedVectorFormat4x8Bit = 0

// dot4Packed is the packed 4x8-bit dot product of a and b (OpSDotKHR and
// OpUDotKHR with the PackedVectorFormat4x8Bit operand, WGSL dot4I8Packed and
// dot4U8Packed): the sum of the products of their four bytes, each byte
// sign-extended when signed is true, wrapping in 32 bits. The result is i32
// for the signed form and u32 for the unsigned one.
func dot4Packed(a, b uint32, signed bool) Value {
	var sum uint32
	for i := range 4 {
		shift := 8 * i
		x, y := (a>>shift)&0xFF, (b>>shift)&0xFF
		if signed {
			sum += uint32(int32(int8(x)) * int32(int8(y)))
		} else {
			sum += x * y
		}
	}
	if signed {
		return ValInt(int32(sum))
	}
	return ValUint(sum)
}
