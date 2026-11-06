//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestSamplerDescriptorToHALLodMaxClamp checks that the HAL descriptor, which
// every native backend takes literally, carries 32 for a zero LodMaxClamp and
// the given value otherwise.
func TestSamplerDescriptorToHALLodMaxClamp(t *testing.T) {
	for _, c := range []struct {
		name           string
		desc           SamplerDescriptor
		wantMin, wantM float32
	}{
		{"zero value", SamplerDescriptor{}, 0, 32},
		{"min only", SamplerDescriptor{LodMinClamp: 2}, 2, 32},
		{"explicit max", SamplerDescriptor{LodMinClamp: 0.5, LodMaxClamp: 4}, 0.5, 4},
	} {
		h := c.desc.toHAL()
		if h.LodMinClamp != c.wantMin || h.LodMaxClamp != c.wantM {
			t.Errorf("%s: HAL clamps [%v, %v], want [%v, %v]", c.name, h.LodMinClamp, h.LodMaxClamp, c.wantMin, c.wantM)
		}
	}
}

// TestSamplerDescriptorToHALZeroEnums: a zero-value descriptor reaches the HAL
// with nearest filters and clamp-to-edge address modes, explicit ones as given.
func TestSamplerDescriptorToHALZeroEnums(t *testing.T) {
	h := (&SamplerDescriptor{}).toHAL()
	if h.MinFilter != gputypes.FilterModeNearest || h.MagFilter != gputypes.FilterModeNearest || h.MipmapFilter != gputypes.FilterModeNearest {
		t.Errorf("zero filters reach the HAL as %v/%v/%v, want nearest", h.MinFilter, h.MagFilter, h.MipmapFilter)
	}
	if h.AddressModeU != gputypes.AddressModeClampToEdge || h.AddressModeV != gputypes.AddressModeClampToEdge || h.AddressModeW != gputypes.AddressModeClampToEdge {
		t.Errorf("zero address modes reach the HAL as %v/%v/%v, want clamp-to-edge", h.AddressModeU, h.AddressModeV, h.AddressModeW)
	}
	e := (&SamplerDescriptor{MipmapFilter: gputypes.FilterModeLinear, AddressModeU: gputypes.AddressModeRepeat}).toHAL()
	if e.MipmapFilter != gputypes.FilterModeLinear || e.AddressModeU != gputypes.AddressModeRepeat {
		t.Errorf("explicit values changed: mipmap %v, U %v", e.MipmapFilter, e.AddressModeU)
	}
}

// TestSamplerDescriptorToHALAnisotropy checks that the HAL gets the WebGPU
// default maxAnisotropy 1 for a zero Anisotropy, decided in the root, and the
// given value otherwise.
func TestSamplerDescriptorToHALAnisotropy(t *testing.T) {
	for _, c := range []struct{ in, want uint16 }{{0, 1}, {1, 1}, {4, 4}, {16, 16}} {
		if got := (&SamplerDescriptor{Anisotropy: c.in}).toHAL().Anisotropy; got != c.want {
			t.Errorf("Anisotropy %d: HAL %d, want %d", c.in, got, c.want)
		}
	}
}
