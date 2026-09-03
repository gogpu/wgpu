//go:build !(js && wasm)

package shader

import "math"

// Modf / Frexp / Ldexp helpers for GLSL.std.450 opcodes 35–36 and 51–53.

// glslModf implements Modf: returns fractional part and stores the integer part
// through the pointer operand (GLSL.std.450 Modf = 35).
func (interp *interpreter) glslModf(operands []uint32) Value {
	if len(operands) < 2 {
		return ValFloat(0)
	}
	x := interp.values[operands[0]]
	fract, whole := splitModf(x)
	interp.storeThrough(interp.values[operands[1]], whole)
	return fract
}

// glslModfStruct implements ModfStruct: returns {fract, whole} as TagArray (36).
func (interp *interpreter) glslModfStruct(operands []uint32) Value {
	if len(operands) < 1 {
		return ValArray(nil)
	}
	fract, whole := splitModf(interp.values[operands[0]])
	return ValArray([]Value{fract, whole})
}

// glslFrexp implements Frexp: returns significand in [0.5, 1) and stores exponent
// through the pointer operand (GLSL.std.450 Frexp = 51).
func (interp *interpreter) glslFrexp(operands []uint32) Value {
	if len(operands) < 2 {
		return ValFloat(0)
	}
	sig, exp := splitFrexp(interp.values[operands[0]])
	interp.storeThrough(interp.values[operands[1]], exp)
	return sig
}

// glslFrexpStruct implements FrexpStruct: returns {significand, exponent} (52).
func (interp *interpreter) glslFrexpStruct(operands []uint32) Value {
	if len(operands) < 1 {
		return ValArray(nil)
	}
	sig, exp := splitFrexp(interp.values[operands[0]])
	return ValArray([]Value{sig, exp})
}

// glslLdexp implements Ldexp: builds floating-point from significand and exponent (53).
func (interp *interpreter) glslLdexp(operands []uint32) Value {
	if len(operands) < 2 {
		return ValFloat(0)
	}
	x := interp.values[operands[0]]
	exp := interp.values[operands[1]]
	apply := func(s float32, e int) float32 {
		return float32(math.Ldexp(float64(s), e))
	}
	switch x.Tag {
	case TagVec2:
		return ValVec2(apply(x.F[0], expComponent(exp, 0)), apply(x.F[1], expComponent(exp, 1)))
	case TagVec3:
		return ValVec3(
			apply(x.F[0], expComponent(exp, 0)),
			apply(x.F[1], expComponent(exp, 1)),
			apply(x.F[2], expComponent(exp, 2)),
		)
	case TagVec4:
		return ValVec4(
			apply(x.F[0], expComponent(exp, 0)),
			apply(x.F[1], expComponent(exp, 1)),
			apply(x.F[2], expComponent(exp, 2)),
			apply(x.F[3], expComponent(exp, 3)),
		)
	default:
		return ValFloat(apply(toFloat32(x), expComponent(exp, 0)))
	}
}

// storeThrough writes val through a Pointer / SubPointer / BufferPointer.
func (interp *interpreter) storeThrough(ptr, val Value) {
	switch ptr.Tag {
	case TagPointer:
		if p := ptr.AsPointer(); p != nil {
			p.Val = val
		}
	case TagSubPointer:
		if sp := ptr.AsSubPointer(); sp != nil {
			subPointerStore(sp, val)
		}
	case TagBufferPointer:
		if bp := ptr.AsBufferPointer(); bp != nil {
			writeValueToBuffer(bp.Buffer, bp.Offset, val)
		}
	}
}

func splitModf(x Value) (fract, whole Value) {
	return floatUnaryOp(x, func(v float32) float32 {
			_, f := math.Modf(float64(v))
			return float32(f)
		}), floatUnaryOp(x, func(v float32) float32 {
			w, _ := math.Modf(float64(v))
			return float32(w)
		})
}

func splitFrexp(x Value) (significand, exponent Value) {
	switch x.Tag {
	case TagVec2:
		s0, e0 := math.Frexp(float64(x.F[0]))
		s1, e1 := math.Frexp(float64(x.F[1]))
		return ValVec2(float32(s0), float32(s1)), ValArray([]Value{ValInt(int32(e0)), ValInt(int32(e1))})
	case TagVec3:
		s0, e0 := math.Frexp(float64(x.F[0]))
		s1, e1 := math.Frexp(float64(x.F[1]))
		s2, e2 := math.Frexp(float64(x.F[2]))
		return ValVec3(float32(s0), float32(s1), float32(s2)),
			ValArray([]Value{ValInt(int32(e0)), ValInt(int32(e1)), ValInt(int32(e2))})
	case TagVec4:
		s0, e0 := math.Frexp(float64(x.F[0]))
		s1, e1 := math.Frexp(float64(x.F[1]))
		s2, e2 := math.Frexp(float64(x.F[2]))
		s3, e3 := math.Frexp(float64(x.F[3]))
		return ValVec4(float32(s0), float32(s1), float32(s2), float32(s3)),
			ValArray([]Value{ValInt(int32(e0)), ValInt(int32(e1)), ValInt(int32(e2)), ValInt(int32(e3))})
	default:
		s, e := math.Frexp(float64(toFloat32(x)))
		return ValFloat(float32(s)), ValInt(int32(e))
	}
}

// expComponent returns the integer exponent at component i, broadcasting scalars.
func expComponent(exp Value, i int) int {
	switch exp.Tag {
	case TagArray:
		arr := exp.AsArray()
		if len(arr) == 0 {
			return 0
		}
		if i < len(arr) {
			return int(int32(toUint32(arr[i])))
		}
		return int(int32(toUint32(arr[0])))
	case TagVec2, TagVec3, TagVec4:
		if i >= 0 && i < 4 {
			return int(exp.F[i])
		}
		return int(exp.F[0])
	case TagInt32:
		return int(exp.AsInt32())
	case TagUint32:
		return int(exp.AsUint32())
	default:
		return int(toFloat32(exp))
	}
}
