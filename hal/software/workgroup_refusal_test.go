//go:build !(js && wasm)

package software

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

// TestComputeWorkgroupBarrierRefused checks the P1-SW-025 stopgap: a compute
// shader whose workgroup invocations exchange data through workgroup memory
// across a barrier is refused with ErrWorkgroupBarrierUnsupported (the
// interpreter would compute wrong results), and shaders that cannot exchange
// data that way are still accepted.
func TestComputeWorkgroupBarrierRefused(t *testing.T) {
	tests := []struct {
		name    string
		wgsl    string
		refused bool
	}{
		{"workgroup memory and a barrier", `
var<workgroup> wg: array<u32, 64>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32) {
    wg[i] = i;
    workgroupBarrier();
    out[i] = wg[(i + 1u) % 64u];
}
`, true},
		{"a barrier without workgroup memory", `
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32) {
    out[i] = i;
    storageBarrier();
}
`, false},
		{"workgroup memory in a one-invocation workgroup", `
var<workgroup> w: u32;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1)
fn main() {
    w = 7u;
    workgroupBarrier();
    out[0] = w;
}
`, false},
	}
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: tc.name, Source: hal.ShaderSource{WGSL: tc.wgsl}})
			if err != nil {
				t.Fatalf("CreateShaderModule: %v", err)
			}
			_, err = dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
				Label:   tc.name,
				Compute: hal.ComputeState{Module: sm, EntryPoint: "main"},
			})
			switch {
			case tc.refused && !errors.Is(err, ErrWorkgroupBarrierUnsupported):
				t.Errorf("CreateComputePipeline error = %v, want ErrWorkgroupBarrierUnsupported", err)
			case !tc.refused && err != nil:
				t.Errorf("CreateComputePipeline: %v, want accepted", err)
			}
		})
	}
}
