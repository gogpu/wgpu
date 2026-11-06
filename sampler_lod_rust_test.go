//go:build rust

package wgpu

import "testing"

// TestSamplerDescriptorToRustLodMaxClamp checks the -tags rust conversion:
// wgpu-native takes LodMaxClamp literally, so the zero value must arrive as
// 32, and a nil descriptor (rejected by go-webgpu) means the defaults.
func TestSamplerDescriptorToRustLodMaxClamp(t *testing.T) {
	for _, c := range []struct {
		name           string
		desc           *SamplerDescriptor
		wantMin, wantM float32
	}{
		{"nil", nil, 0, 32},
		{"zero value", &SamplerDescriptor{}, 0, 32},
		{"min only", &SamplerDescriptor{LodMinClamp: 2}, 2, 32},
		{"explicit max", &SamplerDescriptor{LodMinClamp: 0.5, LodMaxClamp: 4}, 0.5, 4},
	} {
		r := samplerDescriptorToRust(c.desc)
		if r == nil {
			t.Fatalf("%s: nil wgpu-native descriptor", c.name)
		}
		if r.LodMinClamp != c.wantMin || r.LodMaxClamp != c.wantM {
			t.Errorf("%s: wgpu-native clamps [%v, %v], want [%v, %v]", c.name, r.LodMinClamp, r.LodMaxClamp, c.wantMin, c.wantM)
		}
	}
}
