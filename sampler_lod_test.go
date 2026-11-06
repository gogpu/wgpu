package wgpu

import "testing"

// TestNormalizeLodMaxClamp checks the one rule every CreateSampler path
// (native, -tags rust, js/wasm) applies to LodMaxClamp: the zero value is the
// WebGPU default 32, any other value is kept.
func TestNormalizeLodMaxClamp(t *testing.T) {
	for _, c := range []struct {
		in, want float32
	}{
		{0, 32},
		{0.25, 0.25},
		{1, 1},
		{32, 32},
		{1000, 1000},
	} {
		if got := normalizeLodMaxClamp(c.in); got != c.want {
			t.Errorf("normalizeLodMaxClamp(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
