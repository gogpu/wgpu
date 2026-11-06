package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestNormalizeSamplerEnums: only the Undefined member of FilterMode and
// AddressMode maps to the WebGPU default (nearest, clamp-to-edge); every
// explicit value is kept.
func TestNormalizeSamplerEnums(t *testing.T) {
	for _, c := range []struct{ in, want gputypes.FilterMode }{
		{gputypes.FilterModeUndefined, gputypes.FilterModeNearest},
		{gputypes.FilterModeNearest, gputypes.FilterModeNearest},
		{gputypes.FilterModeLinear, gputypes.FilterModeLinear},
	} {
		if got := normalizeFilterMode(c.in); got != c.want {
			t.Errorf("normalizeFilterMode(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	for _, c := range []struct{ in, want gputypes.AddressMode }{
		{gputypes.AddressModeUndefined, gputypes.AddressModeClampToEdge},
		{gputypes.AddressModeClampToEdge, gputypes.AddressModeClampToEdge},
		{gputypes.AddressModeRepeat, gputypes.AddressModeRepeat},
		{gputypes.AddressModeMirrorRepeat, gputypes.AddressModeMirrorRepeat},
	} {
		if got := normalizeAddressMode(c.in); got != c.want {
			t.Errorf("normalizeAddressMode(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

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

// TestNormalizeAnisotropy: the zero value is the WebGPU default maxAnisotropy
// 1; every other value is kept for validation to judge.
func TestNormalizeAnisotropy(t *testing.T) {
	for _, c := range []struct{ in, want uint16 }{{0, 1}, {1, 1}, {2, 2}, {16, 16}, {17, 17}} {
		if got := normalizeAnisotropy(c.in); got != c.want {
			t.Errorf("normalizeAnisotropy(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
