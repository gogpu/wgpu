//go:build !rust && !(js && wasm)

package wgpu

import "testing"

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
