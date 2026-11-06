package shader

import (
	"math"
	"testing"
)

// TestParseImageOperands checks that the operand ids are read in the order
// of the mask bits: Bias, Lod, Grad (two ids), then the offset.
func TestParseImageOperands(t *testing.T) {
	interp := &interpreter{values: []Value{
		1: ValFloat(0.5),
		2: ValVec2(0.25, 0),
		3: ValVec2(0, 0.125),
		4: ValArray([]Value{ValInt(-1), ValInt(2)}),
		5: ValFloat(3),
	}}
	ops := interp.parseImageOperands([]uint32{
		ImageOperandBiasMask | ImageOperandGradMask | ImageOperandConstOffsetMask, 1, 2, 3, 4,
	})
	if ops.bias != 0.5 || !ops.hasGrad || ops.dx != (Vec4{0.25}) || ops.dy != (Vec4{0, 0.125}) || ops.offset != [3]int{-1, 2, 0} {
		t.Errorf("Bias|Grad|ConstOffset = %+v", ops)
	}
	if ops := interp.parseImageOperands([]uint32{ImageOperandLodMask, 5}); ops.lod != 3 || ops.hasGrad {
		t.Errorf("Lod = %+v, want lod 3", ops)
	}
	if ops := interp.parseImageOperands([]uint32{ImageOperandGradMask, 2}); ops.hasGrad {
		t.Errorf("Grad with one id = %+v, want no gradient", ops)
	}
}

// TestGradLod checks the level of detail of explicit derivatives: log2 of
// the longer derivative in texels of the base level.
func TestGradLod(t *testing.T) {
	tex2D := &Texture2D{Width: 64, Height: 16}
	tex3D := &Texture2D{Width: 8, Height: 8, Depth: 32, Dimension: TexDim3D}
	tests := []struct {
		name   string
		tex    *Texture2D
		dx, dy Vec4
		want   float32
	}{
		{"2d along u", tex2D, Vec4{4.0 / 64}, Vec4{}, 2},
		{"2d along v wins", tex2D, Vec4{1.0 / 64}, Vec4{0, 8.0 / 16}, 3},
		{"2d diagonal", tex2D, Vec4{3.0 / 64, 4.0 / 16}, Vec4{}, float32(math.Log2(5))},
		{"3d along w", tex3D, Vec4{0, 0, 2.0 / 32}, Vec4{}, 1},
	}
	for _, tt := range tests {
		if got := gradLod(tt.tex, Vec4{}, tt.dx, tt.dy); math.Abs(float64(got-tt.want)) > 1e-5 {
			t.Errorf("%s: gradLod = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestCubeDerivative checks the face-coordinate derivative of a cube
// direction against a finite difference on every face.
func TestCubeDerivative(t *testing.T) {
	dirs := []Vec4{{1, 0.2, -0.3}, {-1, 0.2, 0.3}, {0.1, 1, 0.3}, {0.1, -1, 0.3}, {0.1, 0.2, 1}, {0.1, 0.2, -1}}
	g := Vec4{0.01, -0.02, 0.015}
	const eps = 1e-3
	for i, c := range dirs {
		face, s0, t0 := cubeFace(c)
		if face != i {
			t.Fatalf("direction %v selects face %d, want %d", c, face, i)
		}
		var moved Vec4
		for k := range moved {
			moved[k] = c[k] + eps*g[k]
		}
		_, s1, t1 := cubeFace(moved)
		ds, dt := cubeDerivative(c, g)
		if math.Abs(float64(ds-(s1-s0)/eps)) > 1e-3 || math.Abs(float64(dt-(t1-t0)/eps)) > 1e-3 {
			t.Errorf("face %d: derivative (%v, %v), finite difference (%v, %v)", i, ds, dt, (s1-s0)/eps, (t1-t0)/eps)
		}
	}
}

// TestSampleViewOffsetAndFilters checks a texel offset, the min/mag filter
// choice by the clamped level of detail, and linear filtering between two
// depth slices of a 3D view.
func TestSampleViewOffsetAndFilters(t *testing.T) {
	red, green := []byte{255, 0, 0, 255}, []byte{0, 255, 0, 255}
	row := &Texture2D{Width: 2, Height: 1, Data: append(append([]byte{}, red...), green...)}
	nearest := &Sampler{WrapU: WrapClampToEdge, WrapV: WrapClampToEdge}
	if got := sampleView(row, nearest, Vec4{0.25, 0.5}, 0, [3]int{1, 0, 0}); got != (Vec4{0, 1, 0, 1}) {
		t.Errorf("offset +1 from texel 0 = %v, want green", got)
	}

	// Magnified (λ ≤ 0) the nearest MagFilter applies; with lodMinClamp 1
	// the linear MinFilter blends both texels at the center.
	mixed := &Sampler{MinFilter: FilterLinear, WrapU: WrapClampToEdge, WrapV: WrapClampToEdge}
	if got := sampleView(row, mixed, Vec4{0.5, 0.5}, -1, [3]int{}); got != (Vec4{0, 1, 0, 1}) {
		t.Errorf("magnified = %v, want the nearest texel (green)", got)
	}
	mixed.LodMinClamp = 1
	if got := sampleView(row, mixed, Vec4{0.5, 0.5}, -1, [3]int{}); math.Abs(float64(got[0]-0.5)) > 1e-6 {
		t.Errorf("minified = %v, want a 50%% blend", got)
	}

	vol := &Texture2D{Width: 1, Height: 1, Depth: 2, Dimension: TexDim3D, Data: append(append([]byte{}, red...), green...)}
	linear := &Sampler{MinFilter: FilterLinear, MagFilter: FilterLinear, WrapW: WrapClampToEdge}
	if got := sampleView(vol, linear, Vec4{0.5, 0.5, 0.5}, 0, [3]int{}); math.Abs(float64(got[0]-0.5)) > 1e-6 || math.Abs(float64(got[1]-0.5)) > 1e-6 {
		t.Errorf("3d trilinear at w=0.5 = %v, want a 50%% blend of the slices", got)
	}
}
