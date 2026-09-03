//go:build !(js && wasm)

// GLSL.std.450 extended instruction set implementation for the SPIR-V interpreter.
//
// This file implements the math intrinsics from the GLSL.std.450 extended
// instruction set, which is the standard math library for SPIR-V shaders.
// Instruction numbers reference the Khronos GLSL.std.450 specification
// (SPIRV-Headers GLSL.std.450.h).

package shader

import (
	"math"

	"github.com/gogpu/wgpu/hal"
)

// GLSL.std.450 instruction numbers.
// Reference: https://github.com/KhronosGroup/SPIRV-Headers/blob/main/include/spirv/unified1/GLSL.std.450.h
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

	GLSLRadians = 11
	GLSLDegrees = 12
	GLSLSin     = 13
	GLSLCos     = 14
	GLSLTan     = 15
	GLSLAsin    = 16
	GLSLAcos    = 17
	GLSLAtan    = 18
	GLSLSinh    = 19
	GLSLCosh    = 20
	GLSLTanh    = 21
	GLSLAsinh   = 22
	GLSLAcosh   = 23
	GLSLAtanh   = 24
	GLSLAtan2   = 25

	GLSLPow         = 26
	GLSLExp         = 27
	GLSLLog         = 28
	GLSLExp2        = 29
	GLSLLog2        = 30
	GLSLSqrt        = 31
	GLSLInverseSqrt = 32

	GLSLDeterminant   = 33
	GLSLMatrixInverse = 34

	GLSLModf       = 35
	GLSLModfStruct = 36
	GLSLFMin       = 37
	GLSLUMin       = 38
	GLSLSMin       = 39
	GLSLFMax       = 40
	GLSLUMax       = 41
	GLSLSMax       = 42
	GLSLFClamp     = 43
	GLSLUClamp     = 44
	GLSLSClamp     = 45
	GLSLFMix       = 46
	GLSLIMix       = 47 // Reserved; treated as FMix.
	GLSLStep       = 48
	GLSLSmoothStep = 49

	GLSLFma         = 50
	GLSLFrexp       = 51
	GLSLFrexpStruct = 52
	GLSLLdexp       = 53

	GLSLPackSnorm4x8    = 54
	GLSLPackUnorm4x8    = 55
	GLSLPackSnorm2x16   = 56
	GLSLPackUnorm2x16   = 57
	GLSLPackHalf2x16    = 58
	GLSLPackDouble2x32  = 59
	GLSLUnpackSnorm2x16 = 60
	GLSLUnpackUnorm2x16 = 61
	GLSLUnpackHalf2x16  = 62
	GLSLUnpackSnorm4x8  = 63
	GLSLUnpackUnorm4x8  = 64
	GLSLUnpackDouble2x32 = 65

	GLSLLength      = 66
	GLSLDistance    = 67
	GLSLCross       = 68
	GLSLNormalize   = 69
	GLSLFaceForward = 70
	GLSLReflect     = 71
	GLSLRefract     = 72

	GLSLFindILsb = 73
	GLSLFindSMsb = 74
	GLSLFindUMsb = 75

	GLSLInterpolateAtCentroid = 76
	GLSLInterpolateAtSample   = 77
	GLSLInterpolateAtOffset   = 78

	GLSLNMin   = 79
	GLSLNMax   = 80
	GLSLNClamp = 81
)

// executeGLSLExtInst dispatches a GLSL.std.450 extended instruction.
// instNum is the instruction number from the GLSL set.
// operands are the remaining SPIR-V operands after the set ID and instruction number.
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
	case GLSLRadians:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return x * float32(math.Pi/180)
		})
	case GLSLDegrees:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return x * float32(180/math.Pi)
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

	// --- Hyperbolic ops ---
	case GLSLSinh:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Sinh(float64(x)))
		})
	case GLSLCosh:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Cosh(float64(x)))
		})
	case GLSLTanh:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Tanh(float64(x)))
		})
	case GLSLAsinh:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Asinh(float64(x)))
		})
	case GLSLAcosh:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Acosh(float64(x)))
		})
	case GLSLAtanh:
		return interp.glslUnaryFloat(operands, func(x float32) float32 {
			return float32(math.Atanh(float64(x)))
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

	// --- Matrix ops ---
	case GLSLDeterminant:
		if len(operands) >= 1 {
			return ValFloat(matrixDeterminant(interp.values[operands[0]]))
		}
	case GLSLMatrixInverse:
		if len(operands) >= 1 {
			return matrixInverse(interp.values[operands[0]])
		}

	// --- Split / frexp / ldexp ---
	case GLSLModf:
		return interp.glslModf(operands)
	case GLSLModfStruct:
		return interp.glslModfStruct(operands)
	case GLSLFrexp:
		return interp.glslFrexp(operands)
	case GLSLFrexpStruct:
		return interp.glslFrexpStruct(operands)
	case GLSLLdexp:
		return interp.glslLdexp(operands)

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
	case GLSLUClamp:
		return interp.glslTernaryUint(operands, func(x, minVal, maxVal uint32) uint32 {
			if x < minVal {
				x = minVal
			}
			if x > maxVal {
				x = maxVal
			}
			return x
		})
	case GLSLSClamp:
		return interp.glslTernaryInt(operands, func(x, minVal, maxVal int32) int32 {
			if x < minVal {
				x = minVal
			}
			if x > maxVal {
				x = maxVal
			}
			return x
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
				return ValInt(-v)
			}
			return ValInt(v)
		}
	case GLSLSSign:
		if len(operands) >= 1 {
			v := int32(toUint32(interp.values[operands[0]]))
			if v > 0 {
				return ValInt(1)
			}
			if v < 0 {
				return ValInt(-1)
			}
			return ValInt(0)
		}

	// --- Interpolation / mix ---
	case GLSLFMix, GLSLIMix:
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
	case GLSLFma:
		return interp.glslTernaryFloat(operands, func(a, b, c float32) float32 {
			return float32(math.FMA(float64(a), float64(b), float64(c)))
		})

	// --- Pack / Unpack ---
	case GLSLPackSnorm4x8:
		if len(operands) >= 1 {
			return ValUint(packSnorm4x8(interp.values[operands[0]]))
		}
	case GLSLPackUnorm4x8:
		if len(operands) >= 1 {
			return ValUint(packUnorm4x8(interp.values[operands[0]]))
		}
	case GLSLPackSnorm2x16:
		if len(operands) >= 1 {
			return ValUint(packSnorm2x16(interp.values[operands[0]]))
		}
	case GLSLPackUnorm2x16:
		if len(operands) >= 1 {
			return ValUint(packUnorm2x16(interp.values[operands[0]]))
		}
	case GLSLPackHalf2x16:
		if len(operands) >= 1 {
			return ValUint(packHalf2x16(interp.values[operands[0]]))
		}
	case GLSLPackDouble2x32:
		if len(operands) >= 1 {
			return packDouble2x32(interp.values[operands[0]])
		}
	case GLSLUnpackSnorm2x16:
		if len(operands) >= 1 {
			return unpackSnorm2x16(toUint32(interp.values[operands[0]]))
		}
	case GLSLUnpackUnorm2x16:
		if len(operands) >= 1 {
			return unpackUnorm2x16(toUint32(interp.values[operands[0]]))
		}
	case GLSLUnpackHalf2x16:
		if len(operands) >= 1 {
			return unpackHalf2x16(toUint32(interp.values[operands[0]]))
		}
	case GLSLUnpackSnorm4x8:
		if len(operands) >= 1 {
			return unpackSnorm4x8(toUint32(interp.values[operands[0]]))
		}
	case GLSLUnpackUnorm4x8:
		if len(operands) >= 1 {
			return unpackUnorm4x8(toUint32(interp.values[operands[0]]))
		}
	case GLSLUnpackDouble2x32:
		if len(operands) >= 1 {
			return unpackDouble2x32(interp.values[operands[0]])
		}

	// --- Geometric ops ---
	case GLSLLength:
		if len(operands) >= 1 {
			return ValFloat(vectorLength(interp.values[operands[0]]))
		}
	case GLSLDistance:
		if len(operands) >= 2 {
			diff := floatBinOp(interp.values[operands[0]], interp.values[operands[1]],
				func(a, b float32) float32 { return a - b })
			return ValFloat(vectorLength(diff))
		}
	case GLSLNormalize:
		if len(operands) >= 1 {
			return normalizeVector(interp.values[operands[0]])
		}
	case GLSLCross:
		if len(operands) >= 2 {
			return crossProduct(interp.values[operands[0]], interp.values[operands[1]])
		}
	case GLSLFaceForward:
		if len(operands) >= 3 {
			return faceForward(interp.values[operands[0]], interp.values[operands[1]], interp.values[operands[2]])
		}
	case GLSLReflect:
		if len(operands) >= 2 {
			return reflectVector(interp.values[operands[0]], interp.values[operands[1]])
		}
	case GLSLRefract:
		if len(operands) >= 3 {
			return refractVector(interp.values[operands[0]], interp.values[operands[1]], toFloat32(interp.values[operands[2]]))
		}

	// --- Bit find ---
	case GLSLFindILsb:
		if len(operands) >= 1 {
			return ValInt(findILsb(toUint32(interp.values[operands[0]])))
		}
	case GLSLFindSMsb:
		if len(operands) >= 1 {
			return ValInt(findSMsb(int32(toUint32(interp.values[operands[0]]))))
		}
	case GLSLFindUMsb:
		if len(operands) >= 1 {
			return ValInt(findUMsb(toUint32(interp.values[operands[0]])))
		}

	// --- Fragment interpolation (software: identity after rasterization) ---
	case GLSLInterpolateAtCentroid, GLSLInterpolateAtSample, GLSLInterpolateAtOffset:
		hal.Logger().Warn("GLSL.std.450 interpolate opcode is a no-op in software backend", "opcode", instNum)
		if len(operands) >= 1 {
			return interp.values[operands[0]]
		}

	// --- NaN-aware min/max/clamp ---
	case GLSLNMin:
		return interp.glslBinaryFloat(operands, nMin)
	case GLSLNMax:
		return interp.glslBinaryFloat(operands, nMax)
	case GLSLNClamp:
		return interp.glslTernaryFloat(operands, func(x, minVal, maxVal float32) float32 {
			return nMin(nMax(x, minVal), maxVal)
		})

	default:
		hal.Logger().Warn("unimplemented GLSL.std.450 opcode", "opcode", instNum)
		return ValUint(0)
	}

	return ValUint(0)
}

// =============================================================================
// Helper functions for GLSL ops
// =============================================================================

// glslUnaryFloat applies a unary float function to a scalar or vector value.
func (interp *interpreter) glslUnaryFloat(operands []uint32, fn func(float32) float32) Value {
	if len(operands) < 1 {
		return ValFloat(0)
	}
	return floatUnaryOp(interp.values[operands[0]], fn)
}

// glslBinaryFloat applies a binary float function to two scalar or vector values.
func (interp *interpreter) glslBinaryFloat(operands []uint32, fn func(float32, float32) float32) Value {
	if len(operands) < 2 {
		return ValFloat(0)
	}
	return floatBinOp(interp.values[operands[0]], interp.values[operands[1]], fn)
}

// glslTernaryFloat applies a ternary float function component-wise.
func (interp *interpreter) glslTernaryFloat(operands []uint32, fn func(float32, float32, float32) float32) Value {
	if len(operands) < 3 {
		return ValFloat(0)
	}
	a := interp.values[operands[0]]
	b := interp.values[operands[1]]
	c := interp.values[operands[2]]

	switch a.Tag {
	case TagFloat32:
		return ValFloat(fn(a.F[0], toFloat32(b), toFloat32(c)))
	case TagVec2:
		return ValVec2(fn(a.F[0], b.F[0], c.F[0]), fn(a.F[1], b.F[1], c.F[1]))
	case TagVec3:
		return ValVec3(fn(a.F[0], b.F[0], c.F[0]), fn(a.F[1], b.F[1], c.F[1]), fn(a.F[2], b.F[2], c.F[2]))
	case TagVec4:
		return ValVec4(
			fn(a.F[0], b.F[0], c.F[0]), fn(a.F[1], b.F[1], c.F[1]),
			fn(a.F[2], b.F[2], c.F[2]), fn(a.F[3], b.F[3], c.F[3]),
		)
	}
	return ValFloat(fn(toFloat32(a), toFloat32(b), toFloat32(c)))
}

// glslBinaryUint applies a binary uint function.
func (interp *interpreter) glslBinaryUint(operands []uint32, fn func(uint32, uint32) uint32) Value {
	if len(operands) < 2 {
		return ValUint(0)
	}
	a := toUint32(interp.values[operands[0]])
	b := toUint32(interp.values[operands[1]])
	return ValUint(fn(a, b))
}

// glslBinaryInt applies a binary signed int function.
func (interp *interpreter) glslBinaryInt(operands []uint32, fn func(int32, int32) int32) Value {
	if len(operands) < 2 {
		return ValInt(0)
	}
	a := int32(toUint32(interp.values[operands[0]]))
	b := int32(toUint32(interp.values[operands[1]]))
	return ValInt(fn(a, b))
}

// glslTernaryUint applies a ternary uint function.
func (interp *interpreter) glslTernaryUint(operands []uint32, fn func(uint32, uint32, uint32) uint32) Value {
	if len(operands) < 3 {
		return ValUint(0)
	}
	a := toUint32(interp.values[operands[0]])
	b := toUint32(interp.values[operands[1]])
	c := toUint32(interp.values[operands[2]])
	return ValUint(fn(a, b, c))
}

// glslTernaryInt applies a ternary signed int function.
func (interp *interpreter) glslTernaryInt(operands []uint32, fn func(int32, int32, int32) int32) Value {
	if len(operands) < 3 {
		return ValInt(0)
	}
	a := int32(toUint32(interp.values[operands[0]]))
	b := int32(toUint32(interp.values[operands[1]]))
	c := int32(toUint32(interp.values[operands[2]]))
	return ValInt(fn(a, b, c))
}

// =============================================================================
// Geometric functions
// =============================================================================

// vectorLength computes the Euclidean length of a vector.
func vectorLength(val Value) float32 {
	switch val.Tag {
	case TagFloat32:
		return float32(math.Abs(float64(val.F[0])))
	case TagVec2:
		return float32(math.Sqrt(float64(val.F[0]*val.F[0] + val.F[1]*val.F[1])))
	case TagVec3:
		return float32(math.Sqrt(float64(val.F[0]*val.F[0] + val.F[1]*val.F[1] + val.F[2]*val.F[2])))
	case TagVec4:
		return float32(math.Sqrt(float64(val.F[0]*val.F[0] + val.F[1]*val.F[1] + val.F[2]*val.F[2] + val.F[3]*val.F[3])))
	}
	return 0
}

// normalizeVector returns a unit-length vector in the same direction.
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
	if a.Tag != TagVec3 || b.Tag != TagVec3 {
		return ValVec3(0, 0, 0)
	}
	return ValVec3(
		a.F[1]*b.F[2]-a.F[2]*b.F[1],
		a.F[2]*b.F[0]-a.F[0]*b.F[2],
		a.F[0]*b.F[1]-a.F[1]*b.F[0],
	)
}

// reflectVector computes the reflection of incident vector I around normal N.
// reflect(I, N) = I - 2*dot(N, I)*N
func reflectVector(incident, normal Value) Value {
	d := toFloat32(dotProduct(normal, incident))
	scaled := vectorTimesScalar(normal, 2*d)
	return floatBinOp(incident, scaled, func(a, b float32) float32 { return a - b })
}

// faceForward returns N if dot(Nref, I) < 0, otherwise -N.
func faceForward(n, i, nref Value) Value {
	if toFloat32(dotProduct(nref, i)) < 0 {
		return n
	}
	return vectorTimesScalar(n, -1)
}

// refractVector computes refraction of incident I for normal N and ratio eta.
// Returns the zero vector when total internal reflection occurs (k < 0).
func refractVector(i, n Value, eta float32) Value {
	dotNI := toFloat32(dotProduct(n, i))
	k := 1 - eta*eta*(1-dotNI*dotNI)
	if k < 0 {
		return vectorTimesScalar(i, 0) // zero vector, same shape as I
	}
	scaledI := vectorTimesScalar(i, eta)
	scaledN := vectorTimesScalar(n, eta*dotNI+float32(math.Sqrt(float64(k))))
	return floatBinOp(scaledI, scaledN, func(a, b float32) float32 { return a - b })
}

// nMin is NaN-aware min: if one operand is NaN, returns the other.
func nMin(a, b float32) float32 {
	if math.IsNaN(float64(b)) {
		return a
	}
	if math.IsNaN(float64(a)) {
		return b
	}
	return float32(math.Min(float64(a), float64(b)))
}

// nMax is NaN-aware max: if one operand is NaN, returns the other.
func nMax(a, b float32) float32 {
	if math.IsNaN(float64(b)) {
		return a
	}
	if math.IsNaN(float64(a)) {
		return b
	}
	return float32(math.Max(float64(a), float64(b)))
}
