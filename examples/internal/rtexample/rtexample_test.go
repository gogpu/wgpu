// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package rtexample

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

func approxEqual(t *testing.T, name string, got, want Mat4) {
	t.Helper()
	for i := range got {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Fatalf("%s: element %d (row %d, col %d) = %v, want %v\ngot  %v\nwant %v",
				name, i, i%4, i/4, got[i], want[i], got, want)
		}
	}
}

func TestLookAtRHFromPositiveZ(t *testing.T) {
	// Camera at +Z looking at the origin is a pure translation by -eye.
	got := LookAtRH(Vec3{0, 0, 2.5}, Vec3{}, Vec3{0, 1, 0})
	approxEqual(t, "look_at", got, Translation(Vec3{0, 0, -2.5}))
}

func TestPerspectiveRH(t *testing.T) {
	m := PerspectiveRH(Radians(59), 2, 0.001, 1000)
	h := float32(1 / math.Tan(float64(Radians(59))/2))
	if math.Abs(float64(m.At(1, 1)-h)) > 1e-5 || math.Abs(float64(m.At(0, 0)-h/2)) > 1e-5 {
		t.Fatalf("unexpected focal terms: %v", m)
	}
	if m.At(3, 2) != -1 || m.At(3, 3) != 0 {
		t.Fatalf("expected RH projection w row (0,0,-1,0): %v", m)
	}
	// Near plane maps to depth 0, far plane to depth 1 (DirectX convention).
	for _, tc := range []struct{ z, depth float32 }{{-0.001, 0}, {-1000, 1}} {
		clipZ := m.At(2, 2)*tc.z + m.At(2, 3)
		clipW := m.At(3, 2) * tc.z
		if d := clipZ / clipW; math.Abs(float64(d-tc.depth)) > 1e-4 {
			t.Errorf("z=%v: depth %v, want %v", tc.z, d, tc.depth)
		}
	}
}

func TestInverse(t *testing.T) {
	m := LookAtRH(Vec3{1, 2, 3}, Vec3{-1, 0, 0.5}, Vec3{0, 1, 0}).
		Mul(PerspectiveRH(Radians(59), 4.0/3.0, 0.001, 1000))
	approxEqual(t, "m*inv", m.Mul(m.Inverse()), Identity())
}

func TestRotationsAndRows(t *testing.T) {
	// Ry(90°) maps +X to -Z.
	r := RotationY(math.Pi / 2)
	if math.Abs(float64(r.At(2, 0)+1)) > 1e-6 {
		t.Fatalf("RotationY(90°) x axis = (%v,%v,%v)", r.At(0, 0), r.At(1, 0), r.At(2, 0))
	}
	// Rz(180°) with translation, as used by ray-scene at t=0.
	m := RotationTranslationEulerXYZ(0, 0, math.Pi, Vec3{-3.5, 3.5, -14})
	rows := m.Rows3x4()
	want := [12]float32{-1, 0, 0, -3.5, 0, -1, 0, 3.5, 0, 0, 1, -14}
	for i := range rows {
		if math.Abs(float64(rows[i]-want[i])) > 1e-6 {
			t.Fatalf("Rows3x4 = %v, want %v", rows, want)
		}
	}
	if got := Translation(Vec3{1, 2, 3}).Rows3x4(); got != [12]float32{1, 0, 0, 1, 0, 1, 0, 2, 0, 0, 1, 3} {
		t.Fatalf("translation rows = %v", got)
	}
}

func TestAppendBytesColumnMajor(t *testing.T) {
	b := Translation(Vec3{5, 0, 0}).AppendBytes(nil)
	if len(b) != 64 {
		t.Fatalf("len = %d", len(b))
	}
	// Column 3, row 0 is element 12 → byte offset 48.
	if got := math.Float32frombits(uint32(b[48]) | uint32(b[49])<<8 | uint32(b[50])<<16 | uint32(b[51])<<24); got != 5 {
		t.Fatalf("translation x at offset 48 = %v", got)
	}
}

func solid(c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestMeanDifference(t *testing.T) {
	black := solid(color.NRGBA{A: 255})
	white := solid(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if d, _ := MeanDifference(black, black); d != 0 {
		t.Errorf("identical: %v", d)
	}
	if d, _ := MeanDifference(black, white); math.Abs(d-1) > 1e-9 {
		t.Errorf("black vs white: %v", d)
	}
	if _, err := MeanDifference(black, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err == nil {
		t.Error("expected size mismatch error")
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, white); err != nil {
		t.Fatal(err)
	}
	if err := ComparePNG(white, buf.Bytes(), DefaultMeanThreshold); err != nil {
		t.Errorf("ComparePNG identical: %v", err)
	}
	if err := ComparePNG(black, buf.Bytes(), DefaultMeanThreshold); err == nil {
		t.Error("ComparePNG black vs white: expected failure")
	}
}
