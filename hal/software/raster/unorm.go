//go:build !(js && wasm)

package raster

import "math"

// Float to normalized integer conversion for every write of a float value to
// an unorm or snorm texel, as the D3D11 functional spec defines it (§3.2.3.6
// FLOAT -> UNORM, §3.2.3.4 FLOAT -> SNORM; §3.2.2 lists these as exceptions
// to the general round-to-nearest-even rule):
//   - unorm: NaN is 0; clamp to [0, 1]; c = c * (2^n-1); c = c + 0.5; drop
//     the fraction (round half up);
//   - snorm: NaN is 0; clamp to [-1, 1]; c = c * (2^(n-1)-1); c + 0.5 for
//     c >= 0, c - 0.5 for c < 0; drop the fraction (round half away from 0).
//
// Vulkan ("Conversion from Floating-Point to Normalized Fixed-Point") asks
// for the nearest integer and leaves the tie open, so the D3D rule satisfies
// both. The product is formed in float64, where a float32 times 2^n-1
// (n <= 16) is exact, so the rounding sees the true value instead of a
// float32 product (the spec's 0.6 ULP tolerance exists for that error).
//
// Truncating instead, as the software backend did before v0.34.7, stores one
// less than the GPUs for every value whose scaled fraction is 0.5 or more:
// about half of all inputs.

// FloatToUnorm converts f to an unsigned normalized integer with the given
// maximum value (2^n-1).
func FloatToUnorm(f float32, maxValue uint32) uint32 {
	if !(f > 0) { // NaN, zero and negative values
		return 0
	}
	if f >= 1 {
		return maxValue
	}
	return roundHalfUp(float64(f) * float64(maxValue))
}

// FloatToUnorm8 converts f to an 8-bit unsigned normalized value.
func FloatToUnorm8(f float32) uint8 {
	return uint8(FloatToUnorm(f, math.MaxUint8))
}

// FloatToUnorm16 converts f to a 16-bit unsigned normalized value.
func FloatToUnorm16(f float32) uint16 {
	return uint16(FloatToUnorm(f, math.MaxUint16))
}

// FloatToSnorm converts f to a signed normalized integer with the given
// maximum value (2^(n-1)-1). The result is in [-maxValue, maxValue]: the most
// negative two's complement value is never produced.
func FloatToSnorm(f float32, maxValue int32) int32 {
	switch {
	case math.IsNaN(float64(f)):
		return 0
	case f >= 1:
		return maxValue
	case f <= -1:
		return -maxValue
	}
	return roundHalfAway(float64(f) * float64(maxValue))
}

// roundHalfUp is the D3D unorm rounding of a scaled value c >= 0: c + 0.5
// with the fraction dropped.
func roundHalfUp(c float64) uint32 {
	return uint32(c + 0.5)
}

// roundHalfAway is the D3D snorm rounding of a scaled value: c + 0.5 for
// c >= 0, c - 0.5 for c < 0, with the fraction dropped (toward zero).
func roundHalfAway(c float64) int32 {
	if c >= 0 {
		return int32(c + 0.5)
	}
	return int32(c - 0.5)
}
