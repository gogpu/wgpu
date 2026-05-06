//go:build !(js && wasm)

// GLSL.std.450 extended instruction set implementation for the SPIR-V interpreter.
//
// This file implements the math intrinsics from the GLSL.std.450 extended
// instruction set, which is the standard math library for SPIR-V shaders.
// Instruction numbers reference the GLSL.std.450 specification.

package shader

import "math"

// GLSL.std.450 instruction numbers.
// Reference: SPIR-V Extended Instructions for GLSL, section 2.
const (
	GLSLRound     = 1
	GLSLRoundEven = 2
	GLSLTrunc     = 3
	GLSLFAbs      = 4
	GLSLSAbs      = 5
	GLSLFSign     = 6
	GLSLSSign     = 7
	GLSLFloor     = 8
	GLSLCeil      = 9
	GLSLFract     = 10

	GLSLSin  = 13
	GLSLCos  = 14
	GLSLTan  = 15
	GLSLAsin = 16
	GLSLAcos = 17
	GLSLAtan = 18

	GLSLPow         = 26
	GLSLExp         = 27
	GLSLLog         = 28
	GLSLExp2        = 29
	GLSLLog2        = 30
	GLSLSqrt        = 31
	GLSLInverseSqrt = 32

	GLSLFMin   = 37
	GLSLUMin   = 38
	GLSLSMin   = 39
	GLSLFMax   = 40
	GLSLUMax   = 41
	GLSLSMax   = 42
	GLSLFClamp = 43

	GLSLFMix       = 46
	GLSLStep       = 48
	GLSLSmoothStep = 49

	GLSLAtan2 = 25

	GLSLLength    = 66
	GLSLDistance  = 67
	GLSLCross     = 68
	GLSLNormalize = 69
	GLSLReflect   = 71

	GLSLDeterminant   = 76
	GLSLMatrixInverse = 77
)

// executeGLSLExtInst dispatches a GLSL.std.450 extended instruction.
// instNum is the instruction number from the GLSL set.
// operands are the remaining SPIR-V operands after the set ID and instruction number.
//
//nolint:maintidx // Large switch is inherent to GLSL.std.450 opcode dispatch.
func (interp *interpreter) executeGLSLExtInst(instNum uint32, operands []uint32) Value {
	switch instNum {
	// --- Scalar/vector unary float ops ---
	case GLSLRound:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.RoundToEven(float64(x)))
		})
	case GLSLRoundEven:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.RoundToEven(float64(x)))
		})
	case GLSLTrunc:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Trunc(float64(x)))
		})
	case GLSLFAbs:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Abs(float64(x)))
		})
	case GLSLFSign:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			if x > 0 {
				return 1
			}
			if x < 0 {
				return -1
			}
			return 0
		})
	case GLSLFloor:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Floor(float64(x)))
		})
	case GLSLCeil:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Ceil(float64(x)))
		})
	case GLSLFract:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return x - float32(math.Floor(float64(x)))
		})

	// --- Trigonometric ops ---
	case GLSLSin:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Sin(float64(x)))
		})
	case GLSLCos:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Cos(float64(x)))
		})
	case GLSLTan:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Tan(float64(x)))
		})
	case GLSLAsin:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Asin(float64(x)))
		})
	case GLSLAcos:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Acos(float64(x)))
		})
	case GLSLAtan:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Atan(float64(x)))
		})
	case GLSLAtan2:
		return interp.glslBinaryFloat(operands, func(y, x float32) float32 {
			return float32(math.Atan2(float64(y), float64(x)))
		})

	// --- Exponential ops ---
	case GLSLPow:
		return interp.glslBinaryFloat(operands, func(x, y float32) float32 {
			return float32(math.Pow(float64(x), float64(y)))
		})
	case GLSLExp:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Exp(float64(x)))
		})
	case GLSLLog:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Log(float64(x)))
		})
	case GLSLExp2:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Exp2(float64(x)))
		})
	case GLSLLog2:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Log2(float64(x)))
		})
	case GLSLSqrt:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Sqrt(float64(x)))
		})
	case GLSLInverseSqrt:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			if x <= 0 {
				return float32(math.Inf(1))
			}
			return 1.0 / float32(math.Sqrt(float64(x)))
		})

	// --- Min/Max/Clamp ---
	case GLSLFMin:
		return interp.glslBinaryFloat(operands, func(a, b float32) float32 {
			return float32(math.Min(float64(a), float64(b)))
		})
	case GLSLFMax:
		return interp.glslBinaryFloat(operands, func(a, b float32) float32 {
			return float32(math.Max(float64(a), float64(b)))
		})
	case GLSLFClamp:
		return interp.glslTernaryFloat(operands, func(x, minVal, maxVal float32) float32 {
			return float32(math.Min(math.Max(float64(x), float64(minVal)), float64(maxVal)))
		})
	case GLSLUMin:
		return interp.glslBinaryUint(operands, func(a, b uint32) uint32 {
			if a < b {
				return a
			}
			return b
		})
	case GLSLUMax:
		return interp.glslBinaryUint(operands, func(a, b uint32) uint32 {
			if a > b {
				return a
			}
			return b
		})
	case GLSLSMin:
		return interp.glslBinaryInt(operands, func(a, b int32) int32 {
			if a < b {
				return a
			}
			return b
		})
	case GLSLSMax:
		return interp.glslBinaryInt(operands, func(a, b int32) int32 {
			if a > b {
				return a
			}
			return b
		})
	case GLSLSAbs:
		if len(operands) >= 1 {
			v := int32(toUint32(interp.values[operands[0]]))
			if v < 0 {
				return -v
			}
			return v
		}
	case GLSLSSign:
		if len(operands) >= 1 {
			v := int32(toUint32(interp.values[operands[0]]))
			if v > 0 {
				return Int32(1)
			}
			if v < 0 {
				return Int32(-1)
			}
			return Int32(0)
		}

	// --- Interpolation ---
	case GLSLFMix:
		return interp.glslTernaryFloat(operands, func(x, y, a float32) float32 {
			return x*(1-a) + y*a
		})
	case GLSLStep:
		return interp.glslBinaryFloat(operands, func(edge, x float32) float32 {
			if x < edge {
				return 0
			}
			return 1
		})
	case GLSLSmoothStep:
		return interp.glslTernaryFloat(operands, func(edge0, edge1, x float32) float32 {
			if edge0 == edge1 {
				if x < edge0 {
					return 0
				}
				return 1
			}
			t := (x - edge0) / (edge1 - edge0)
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			return t * t * (3 - 2*t)
		})

	// --- Geometric ops ---
	case GLSLLength:
		if len(operands) >= 1 {
			return Float32(vectorLength(interp.values[operands[0]]))
		}
	case GLSLDistance:
		if len(operands) >= 2 {
			diff := floatBinOp(interp.values[operands[0]], interp.values[operands[1]],
				func(a, b float32) float32 { return a - b })
			return Float32(vectorLength(diff))
		}
	case GLSLNormalize:
		if len(operands) >= 1 {
			return normalizeVector(interp.values[operands[0]])
		}
	case GLSLCross:
		if len(operands) >= 2 {
			return crossProduct(interp.values[operands[0]], interp.values[operands[1]])
		}
	case GLSLReflect:
		if len(operands) >= 2 {
			return reflectVector(interp.values[operands[0]], interp.values[operands[1]])
		}
	}

	return Uint32(0)
}

// =============================================================================
// Helper functions for GLSL ops
// =============================================================================

// glslUnaryFloat applies a unary float function to a scalar or vector value.
func (interp *interpreter) glslUnaryFloat(operands []uint32, fn func(float32) float32) Value {
	if len(operands) < 1 {
		return Float32(0)
	}
	return floatUnaryOp(interp.values[operands[0]], fn)
}

// glslBinaryFloat applies a binary float function to two scalar or vector values.
func (interp *interpreter) glslBinaryFloat(operands []uint32, fn func(float32, float32) float32) Value {
	if len(operands) < 2 {
		return Float32(0)
	}
	return floatBinOp(interp.values[operands[0]], interp.values[operands[1]], fn)
}

// glslTernaryFloat applies a ternary float function component-wise.
func (interp *interpreter) glslTernaryFloat(operands []uint32, fn func(float32, float32, float32) float32) Value {
	if len(operands) < 3 {
		return Float32(0)
	}
	a := interp.values[operands[0]]
	b := interp.values[operands[1]]
	c := interp.values[operands[2]]

	switch av := a.(type) {
	case Float32:
		return Float32(fn(av, toFloat32(b), toFloat32(c)))
	case Vec2:
		bv, _ := b.(Vec2)
		cv, _ := c.(Vec2)
		return Vec2{fn(av[0], bv[0], cv[0]), fn(av[1], bv[1], cv[1])}
	case Vec3:
		bv, _ := b.(Vec3)
		cv, _ := c.(Vec3)
		return Vec3{fn(av[0], bv[0], cv[0]), fn(av[1], bv[1], cv[1]), fn(av[2], bv[2], cv[2])}
	case Vec4:
		bv, _ := b.(Vec4)
		cv, _ := c.(Vec4)
		return Vec4{
			fn(av[0], bv[0], cv[0]), fn(av[1], bv[1], cv[1]),
			fn(av[2], bv[2], cv[2]), fn(av[3], bv[3], cv[3]),
		}
	}
	return Float32(fn(toFloat32(a), toFloat32(b), toFloat32(c)))
}

// glslBinaryUint applies a binary uint function.
func (interp *interpreter) glslBinaryUint(operands []uint32, fn func(uint32, uint32) uint32) Value {
	if len(operands) < 2 {
		return Uint32(0)
	}
	a := toUint32(interp.values[operands[0]])
	b := toUint32(interp.values[operands[1]])
	return fn(a, b)
}

// glslBinaryInt applies a binary signed int function.
func (interp *interpreter) glslBinaryInt(operands []uint32, fn func(int32, int32) int32) Value {
	if len(operands) < 2 {
		return Int32(0)
	}
	a := int32(toUint32(interp.values[operands[0]]))
	b := int32(toUint32(interp.values[operands[1]]))
	return fn(a, b)
}

// =============================================================================
// Geometric functions
// =============================================================================

// vectorLength computes the Euclidean length of a vector.
func vectorLength(val Value) float32 {
	switch v := val.(type) {
	case Float32:
		return float32(math.Abs(float64(v)))
	case Vec2:
		return float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1])))
	case Vec3:
		return float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])))
	case Vec4:
		return float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1] + v[2]*v[2] + v[3]*v[3])))
	}
	return 0
}

// normalizeVector returns a unit-length vector in the same direction.
// Returns zero vector if the input has zero length.
func normalizeVector(val Value) Value {
	length := vectorLength(val)
	if length == 0 {
		return val
	}
	invLen := 1.0 / length
	return vectorTimesScalar(val, invLen)
}

// crossProduct computes the cross product of two Vec3 values.
func crossProduct(a, b Value) Value {
	av, aOK := a.(Vec3)
	bv, bOK := b.(Vec3)
	if !aOK || !bOK {
		return Vec3{}
	}
	return Vec3{
		av[1]*bv[2] - av[2]*bv[1],
		av[2]*bv[0] - av[0]*bv[2],
		av[0]*bv[1] - av[1]*bv[0],
	}
}

// reflectVector computes the reflection of incident vector I around normal N.
// reflect(I, N) = I - 2*dot(N, I)*N
func reflectVector(incident, normal Value) Value {
	d := toFloat32(dotProduct(normal, incident))
	// 2 * dot(N, I) * N
	scaled := vectorTimesScalar(normal, 2*d)
	return floatBinOp(incident, scaled, func(a, b float32) float32 { return a - b })
}
