//go:build !(js && wasm)

package raster

import (
	"math"
	"testing"
)

func TestFloatToUnorm(t *testing.T) {
	for _, f := range unormInputs(255) {
		if got, want := FloatToUnorm8(f), refUnorm(f, 255); uint32(got) != want {
			t.Errorf("FloatToUnorm8(%v) = %d, want %d", f, got, want)
		}
	}
	for _, f := range unormInputs(65535) {
		if got, want := FloatToUnorm16(f), refUnorm(f, 65535); uint32(got) != want {
			t.Errorf("FloatToUnorm16(%v) = %d, want %d", f, got, want)
		}
	}
	// Every value decodes and encodes back to itself.
	for k := 0; k <= 65535; k++ {
		if got := FloatToUnorm16(float32(k) / 65535); int(got) != k {
			t.Fatalf("FloatToUnorm16(%d/65535) = %d", k, got)
		}
	}
	// Hand-checked anchors, including the only exact ties (x*max = k+0.5).
	anchors := []struct {
		f    float32
		u8   uint8
		u16  uint16
		name string
	}{
		{0.5, 128, 32768, "0.5: 127.5 and 32767.5 round half up"},
		{float32(math.NaN()), 0, 0, "NaN is 0"},
		{float32(math.Inf(1)), 255, 65535, "+Inf clamps to 1"},
		{float32(math.Inf(-1)), 0, 0, "-Inf clamps to 0"},
		{100.75 / 255, 101, 25893, "100.75/255"},
		{200.25 / 255, 200, 51464, "200.25/255"},
	}
	for _, a := range anchors {
		if got := FloatToUnorm8(a.f); got != a.u8 {
			t.Errorf("%s: FloatToUnorm8 = %d, want %d", a.name, got, a.u8)
		}
		if got := FloatToUnorm16(a.f); got != a.u16 {
			t.Errorf("%s: FloatToUnorm16 = %d, want %d", a.name, got, a.u16)
		}
	}
}

// TestD3DRoundingTies pins the D3D11 tie rule on the rounding step itself:
// unorm rounds half up (§3.2.3.6), snorm half away from zero (§3.2.3.4), not
// to the even neighbor. A float32 times 2^n-1 lands exactly on a half only at
// 0.5, where the lower neighbor is odd and both rules agree, so the ties that
// tell the rules apart (even k + 0.5) are fed to the rounding step directly.
func TestD3DRoundingTies(t *testing.T) {
	up := []struct {
		c    float64
		want uint32
	}{
		{0.5, 1}, {2.5, 3}, {4.5, 5}, {126.5, 127}, {254.5, 255}, {65534.5, 65535},
		{2.4999999, 2}, {0, 0}, {255, 255},
	}
	for _, tt := range up {
		if got := roundHalfUp(tt.c); got != tt.want {
			t.Errorf("roundHalfUp(%v) = %d, want %d", tt.c, got, tt.want)
		}
	}
	away := []struct {
		c    float64
		want int32
	}{
		{0.5, 1}, {-0.5, -1}, {2.5, 3}, {-2.5, -3}, {62.5, 63}, {-62.5, -63},
		{2.4999999, 2}, {-2.4999999, -2}, {0, 0}, {-127, -127},
	}
	for _, tt := range away {
		if got := roundHalfAway(tt.c); got != tt.want {
			t.Errorf("roundHalfAway(%v) = %d, want %d", tt.c, got, tt.want)
		}
	}
}

func TestFloatToSnorm(t *testing.T) {
	tests := []struct {
		f    float32
		want int32
	}{
		{float32(math.NaN()), 0},
		{0, 0},
		{1, 127},
		{2, 127},
		{-1, -127},
		{float32(math.Inf(-1)), -127},
		{0.5, 64},   // 63.5 is the only positive tie: half away from zero
		{-0.5, -64}, // and its negation
		{10.25 / 127, 10},
		{-10.75 / 127, -11},
	}
	for _, tt := range tests {
		if got := FloatToSnorm(tt.f, math.MaxInt8); got != tt.want {
			t.Errorf("FloatToSnorm(%v, 127) = %d, want %d", tt.f, got, tt.want)
		}
	}
	for k := int32(-32767); k <= 32767; k++ {
		if got := FloatToSnorm(float32(k)/32767, math.MaxInt16); got != k {
			t.Fatalf("FloatToSnorm(%d/32767) = %d", k, got)
		}
	}
}
