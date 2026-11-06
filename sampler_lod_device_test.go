//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

// TestCreateSamplerLodMaxClampZeroValidation checks that validation sees the
// resolved LodMaxClamp: with the zero value (32), any LodMinClamp up to 32 is
// valid, while an explicit LodMaxClamp below LodMinClamp still fails.
func TestCreateSamplerLodMaxClampZeroValidation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	for _, c := range []struct {
		name    string
		desc    wgpu.SamplerDescriptor
		wantErr bool
	}{
		{"zero max, min 5", wgpu.SamplerDescriptor{LodMinClamp: 5}, false},
		{"zero max, min 32", wgpu.SamplerDescriptor{LodMinClamp: 32}, false},
		{"zero max, min 33", wgpu.SamplerDescriptor{LodMinClamp: 33}, true},
		{"max 1 below min 2", wgpu.SamplerDescriptor{LodMinClamp: 2, LodMaxClamp: 1}, true},
	} {
		s, err := device.CreateSampler(&c.desc)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: CreateSampler error = %v, want error %v", c.name, err, c.wantErr)
		}
		if s != nil {
			s.Release()
		}
	}
}
