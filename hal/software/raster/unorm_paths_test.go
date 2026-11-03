//go:build !(js && wasm)

package raster

import (
	"math"
	"testing"
)

// refUnorm is the D3D11 §3.2.3.6 conversion of a float to a b-bit unorm value
// (maxValue = 2^b-1), written independently of the implementation: NaN is 0,
// the value is clamped to [0, 1], scaled exactly, and rounded half up.
func refUnorm(f float32, maxValue uint32) uint32 {
	switch {
	case math.IsNaN(float64(f)) || f <= 0:
		return 0
	case f >= 1:
		return maxValue
	}
	p := float64(f) * float64(maxValue) // exact: 24-bit mantissa times <= 16 bits
	k := math.Floor(p)
	if p-k >= 0.5 {
		k++
	}
	return uint32(k)
}

// unormInputs returns float values that exercise the conversion: every
// k/max (must convert back to k), the midpoints (k+0.5)/max and the quarter
// points either side as float32 (nearest is decided by which side of k+0.5
// the float lands), the only exact tie 0.5, and values at and beyond both
// ends, infinities and NaN.
func unormInputs(maxValue uint32) []float32 {
	m := float32(maxValue)
	in := []float32{
		0.5, 0, float32(math.Copysign(0, -1)), -1e-30, 1e-30, math.SmallestNonzeroFloat32,
		0.5 / m, math.Nextafter32(0.5/m, 0), math.Nextafter32(0.5/m, 1),
		1, math.Nextafter32(1, 0), math.Nextafter32(1, 2), 1.5, -0.5,
		(m - 0.5) / m, math.Nextafter32((m-0.5)/m, 0), math.Nextafter32((m-0.5)/m, 1),
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	}
	step := uint32(1)
	if maxValue > 255 {
		step = 251 // a prime stride through the 16-bit range keeps the test fast
	}
	for k := uint32(0); k <= maxValue; k += step {
		kf := float32(k)
		in = append(in, kf/m, (kf+0.5)/m, (kf+0.25)/m, (kf+0.75)/m)
	}
	return in
}

// TestUnormWritePaths checks every place the rasterizer converts a float
// color to an 8-bit unorm channel: the clear, an unblended draw, and the blend
// result. Before v0.34.7 these truncated (0.5 became 127, (k+0.75)/255 became
// k), one off from the GPUs.
func TestUnormWritePaths(t *testing.T) {
	copyBlend := BlendState{
		Enabled: true, SrcColor: BlendFactorOne, DstColor: BlendFactorZero, ColorOp: BlendOpAdd,
		SrcAlpha: BlendFactorOne, DstAlpha: BlendFactorZero, AlphaOp: BlendOpAdd,
	}
	tri := []Triangle{fillTri(0, 0, 4, 0, 0, 4)}
	bad := 0
	report := func(path string, f float32, got byte) {
		if want := refUnorm(f, 255); uint32(got) != want {
			if bad < 12 {
				t.Errorf("%s(%v) = %d, want %d", path, f, got, want)
			}
			bad++
		}
	}
	p := NewPipeline(4, 4)
	defer p.Close()
	for _, f := range unormInputs(255) {
		p.Clear(f, f, f, f)
		r, _, _, _ := p.GetPixel(3, 3)
		report("Clear", f, r)

		p.SetBlendState(BlendDisabled)
		p.DrawTriangles(tri, [4]float32{f, f, f, f})
		r, _, _, _ = p.GetPixel(0, 0)
		report("DrawTriangles", f, r)

		r, _, _, _ = BlendFloatToByte([4]float32{f, f, f, f}, 9, 9, 9, 9, copyBlend)
		report("BlendFloatToByte", f, r)

		r, _, _, _ = BlendBytes(byteOf(f), 0, 0, 0, 9, 9, 9, 9, copyBlend)
		if f >= 0 && f <= 1 && !math.IsNaN(float64(f)) {
			// A byte source survives a copy blend unchanged.
			if r != byteOf(f) {
				t.Errorf("BlendBytes copy of %d = %d", byteOf(f), r)
			}
		}
	}
	if bad > 12 {
		t.Errorf("... %d conversions wrong in total", bad)
	}
}

// byteOf is refUnorm for 8 bits as a byte.
func byteOf(f float32) byte { return byte(refUnorm(f, 255)) }
