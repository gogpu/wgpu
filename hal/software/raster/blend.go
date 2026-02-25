package raster

// BlendFactor specifies the blend factor for source or destination color.
// These match WebGPU blend factors.
type BlendFactor uint8

const (
	// BlendFactorZero multiplies by 0.
	BlendFactorZero BlendFactor = iota

	// BlendFactorOne multiplies by 1.
	BlendFactorOne

	// BlendFactorSrc multiplies by source color.
	BlendFactorSrc

	// BlendFactorOneMinusSrc multiplies by (1 - source color).
	BlendFactorOneMinusSrc

	// BlendFactorSrcAlpha multiplies by source alpha.
	BlendFactorSrcAlpha

	// BlendFactorOneMinusSrcAlpha multiplies by (1 - source alpha).
	BlendFactorOneMinusSrcAlpha

	// BlendFactorDst multiplies by destination color.
	BlendFactorDst

	// BlendFactorOneMinusDst multiplies by (1 - destination color).
	BlendFactorOneMinusDst

	// BlendFactorDstAlpha multiplies by destination alpha.
	BlendFactorDstAlpha

	// BlendFactorOneMinusDstAlpha multiplies by (1 - destination alpha).
	BlendFactorOneMinusDstAlpha

	// BlendFactorSrcAlphaSaturated multiplies by min(srcAlpha, 1-dstAlpha).
	BlendFactorSrcAlphaSaturated

	// BlendFactorConstant multiplies by a constant color.
	BlendFactorConstant

	// BlendFactorOneMinusConstant multiplies by (1 - constant color).
	BlendFactorOneMinusConstant
)

// BlendOperation specifies how to combine source and destination after applying factors.
type BlendOperation uint8

const (
	// BlendOpAdd computes src + dst.
	BlendOpAdd BlendOperation = iota

	// BlendOpSubtract computes src - dst.
	BlendOpSubtract

	// BlendOpReverseSubtract computes dst - src.
	BlendOpReverseSubtract

	// BlendOpMin computes min(src, dst).
	BlendOpMin

	// BlendOpMax computes max(src, dst).
	BlendOpMax
)

// BlendState configures color blending for a render target.
type BlendState struct {
	// Enabled indicates whether blending is active.
	Enabled bool

	// SrcColor is the blend factor for the source color (RGB).
	SrcColor BlendFactor

	// DstColor is the blend factor for the destination color (RGB).
	DstColor BlendFactor

	// ColorOp is the operation to combine source and destination colors.
	ColorOp BlendOperation

	// SrcAlpha is the blend factor for the source alpha.
	SrcAlpha BlendFactor

	// DstAlpha is the blend factor for the destination alpha.
	DstAlpha BlendFactor

	// AlphaOp is the operation to combine source and destination alphas.
	AlphaOp BlendOperation

	// Constant is the constant color used for BlendFactorConstant.
	Constant [4]float32
}

// Common blend presets.
var (
	// BlendDisabled disables blending (source replaces destination).
	BlendDisabled = BlendState{Enabled: false}

	// BlendSourceOver implements standard alpha blending (Porter-Duff Source Over).
	// Formula: out = src * srcAlpha + dst * (1 - srcAlpha)
	BlendSourceOver = BlendState{
		Enabled:  true,
		SrcColor: BlendFactorSrcAlpha,
		DstColor: BlendFactorOneMinusSrcAlpha,
		ColorOp:  BlendOpAdd,
		SrcAlpha: BlendFactorOne,
		DstAlpha: BlendFactorOneMinusSrcAlpha,
		AlphaOp:  BlendOpAdd,
	}

	// BlendPremultiplied implements premultiplied alpha blending.
	// Assumes source color is already multiplied by alpha.
	// Formula: out = src + dst * (1 - srcAlpha)
	BlendPremultiplied = BlendState{
		Enabled:  true,
		SrcColor: BlendFactorOne,
		DstColor: BlendFactorOneMinusSrcAlpha,
		ColorOp:  BlendOpAdd,
		SrcAlpha: BlendFactorOne,
		DstAlpha: BlendFactorOneMinusSrcAlpha,
		AlphaOp:  BlendOpAdd,
	}

	// BlendAdditive implements additive blending.
	// Formula: out = src + dst
	BlendAdditive = BlendState{
		Enabled:  true,
		SrcColor: BlendFactorOne,
		DstColor: BlendFactorOne,
		ColorOp:  BlendOpAdd,
		SrcAlpha: BlendFactorOne,
		DstAlpha: BlendFactorOne,
		AlphaOp:  BlendOpAdd,
	}

	// BlendMultiply implements multiplicative blending.
	// Formula: out = src * dst
	BlendMultiply = BlendState{
		Enabled:  true,
		SrcColor: BlendFactorDst,
		DstColor: BlendFactorZero,
		ColorOp:  BlendOpAdd,
		SrcAlpha: BlendFactorDstAlpha,
		DstAlpha: BlendFactorZero,
		AlphaOp:  BlendOpAdd,
	}

	// BlendScreen implements screen blending.
	// Formula: out = 1 - (1 - src) * (1 - dst) = src + dst - src*dst
	BlendScreen = BlendState{
		Enabled:  true,
		SrcColor: BlendFactorOne,
		DstColor: BlendFactorOneMinusSrc,
		ColorOp:  BlendOpAdd,
		SrcAlpha: BlendFactorOne,
		DstAlpha: BlendFactorOneMinusSrcAlpha,
		AlphaOp:  BlendOpAdd,
	}
)

// Blend blends source color with destination color using the blend state.
// All colors are in float32 RGBA format with values in [0, 1].
// Returns the blended color, clamped to [0, 1].
func Blend(src, dst [4]float32, state BlendState) [4]float32 {
	if !state.Enabled {
		return src
	}

	// Get blend factors
	srcColorFactor := applyBlendFactor(state.SrcColor, src, dst, state.Constant)
	dstColorFactor := applyBlendFactor(state.DstColor, src, dst, state.Constant)
	srcAlphaFactor := applyBlendFactorAlpha(state.SrcAlpha, src, dst, state.Constant)
	dstAlphaFactor := applyBlendFactorAlpha(state.DstAlpha, src, dst, state.Constant)

	// Apply factors
	srcR := src[0] * srcColorFactor[0]
	srcG := src[1] * srcColorFactor[1]
	srcB := src[2] * srcColorFactor[2]
	srcA := src[3] * srcAlphaFactor

	dstR := dst[0] * dstColorFactor[0]
	dstG := dst[1] * dstColorFactor[1]
	dstB := dst[2] * dstColorFactor[2]
	dstA := dst[3] * dstAlphaFactor

	// Apply operations
	outR := applyBlendOp(state.ColorOp, srcR, dstR)
	outG := applyBlendOp(state.ColorOp, srcG, dstG)
	outB := applyBlendOp(state.ColorOp, srcB, dstB)
	outA := applyBlendOp(state.AlphaOp, srcA, dstA)

	// Clamp results
	return [4]float32{
		clampFloat(outR, 0, 1),
		clampFloat(outG, 0, 1),
		clampFloat(outB, 0, 1),
		clampFloat(outA, 0, 1),
	}
}

// applyBlendFactor returns the RGB factor multiplier based on the blend factor type.
func applyBlendFactor(factor BlendFactor, src, dst [4]float32, constant [4]float32) [3]float32 {
	switch factor {
	case BlendFactorZero:
		return [3]float32{0, 0, 0}
	case BlendFactorOne:
		return [3]float32{1, 1, 1}
	case BlendFactorSrc:
		return [3]float32{src[0], src[1], src[2]}
	case BlendFactorOneMinusSrc:
		return [3]float32{1 - src[0], 1 - src[1], 1 - src[2]}
	case BlendFactorSrcAlpha:
		return [3]float32{src[3], src[3], src[3]}
	case BlendFactorOneMinusSrcAlpha:
		a := 1 - src[3]
		return [3]float32{a, a, a}
	case BlendFactorDst:
		return [3]float32{dst[0], dst[1], dst[2]}
	case BlendFactorOneMinusDst:
		return [3]float32{1 - dst[0], 1 - dst[1], 1 - dst[2]}
	case BlendFactorDstAlpha:
		return [3]float32{dst[3], dst[3], dst[3]}
	case BlendFactorOneMinusDstAlpha:
		a := 1 - dst[3]
		return [3]float32{a, a, a}
	case BlendFactorSrcAlphaSaturated:
		f := minFloat(src[3], 1-dst[3])
		return [3]float32{f, f, f}
	case BlendFactorConstant:
		return [3]float32{constant[0], constant[1], constant[2]}
	case BlendFactorOneMinusConstant:
		return [3]float32{1 - constant[0], 1 - constant[1], 1 - constant[2]}
	default:
		return [3]float32{1, 1, 1}
	}
}

// applyBlendFactorAlpha returns the alpha factor multiplier based on the blend factor type.
func applyBlendFactorAlpha(factor BlendFactor, src, dst [4]float32, constant [4]float32) float32 {
	switch factor {
	case BlendFactorZero:
		return 0
	case BlendFactorOne:
		return 1
	case BlendFactorSrc, BlendFactorSrcAlpha:
		return src[3]
	case BlendFactorOneMinusSrc, BlendFactorOneMinusSrcAlpha:
		return 1 - src[3]
	case BlendFactorDst, BlendFactorDstAlpha:
		return dst[3]
	case BlendFactorOneMinusDst, BlendFactorOneMinusDstAlpha:
		return 1 - dst[3]
	case BlendFactorSrcAlphaSaturated:
		return 1 // For alpha, saturated is always 1
	case BlendFactorConstant:
		return constant[3]
	case BlendFactorOneMinusConstant:
		return 1 - constant[3]
	default:
		return 1
	}
}

// applyBlendOp applies the blend operation to source and destination values.
func applyBlendOp(op BlendOperation, src, dst float32) float32 {
	switch op {
	case BlendOpAdd:
		return src + dst
	case BlendOpSubtract:
		return src - dst
	case BlendOpReverseSubtract:
		return dst - src
	case BlendOpMin:
		return minFloat(src, dst)
	case BlendOpMax:
		return maxFloat(src, dst)
	default:
		return src + dst
	}
}

// clampFloat clamps a value to the range [min, max].
func clampFloat(v, minVal, maxVal float32) float32 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

// minFloat returns the minimum of two float32 values.
func minFloat(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// maxFloat returns the maximum of two float32 values.
func maxFloat(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// BlendBytes blends source color with destination color, working with byte values.
// This is a convenience function for working with RGBA8 framebuffers.
func BlendBytes(srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA byte, state BlendState) (r, g, b, a byte) {
	// Convert to float
	src := [4]float32{
		float32(srcR) / 255,
		float32(srcG) / 255,
		float32(srcB) / 255,
		float32(srcA) / 255,
	}
	dst := [4]float32{
		float32(dstR) / 255,
		float32(dstG) / 255,
		float32(dstB) / 255,
		float32(dstA) / 255,
	}

	// Blend
	result := Blend(src, dst, state)

	// Convert back to bytes
	return clampByte(result[0] * 255),
		clampByte(result[1] * 255),
		clampByte(result[2] * 255),
		clampByte(result[3] * 255)
}

// BlendFloatToByte blends a float source color with a byte destination color.
// Source is in float [0,1], destination is in bytes [0,255].
// Returns the result as bytes.
func BlendFloatToByte(src [4]float32, dstR, dstG, dstB, dstA byte, state BlendState) (r, g, b, a byte) {
	dst := [4]float32{
		float32(dstR) / 255,
		float32(dstG) / 255,
		float32(dstB) / 255,
		float32(dstA) / 255,
	}

	result := Blend(src, dst, state)

	return clampByte(result[0] * 255),
		clampByte(result[1] * 255),
		clampByte(result[2] * 255),
		clampByte(result[3] * 255)
}
