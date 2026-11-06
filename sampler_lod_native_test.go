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
