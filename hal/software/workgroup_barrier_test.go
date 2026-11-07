//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestComputeWorkgroupBarrierAccepted checks P1-SW-025 through the HAL: a
// compute shader whose workgroup invocations exchange data through workgroup
// memory across a barrier is accepted by CreateComputePipeline (v0.34.7
// refused it with ErrWorkgroupBarrierUnsupported) and computes what the WGSL
// spec says: every invocation sees the stores the others made before the
// barrier, and each workgroup has its own copy of the variables.
func TestComputeWorkgroupBarrierAccepted(t *testing.T) {
	tests := []struct {
		name   string
		wgsl   string
		groups uint32
		want   func(k uint32) uint32
		words  uint32
	}{
		{"workgroup memory and a barrier", `
var<workgroup> wg: array<u32, 64>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32,
        @builtin(workgroup_id) w: vec3<u32>,
        @builtin(global_invocation_id) g: vec3<u32>) {
    wg[i] = i + 100u * w.x;
    workgroupBarrier();
    out[g.x] = wg[(i + 1u) % 64u];
}
`, 2, func(k uint32) uint32 { return (k%64+1)%64 + 100*(k/64) }, 128},
		{"a barrier without workgroup memory", `
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32) {
    out[i] = i;
    storageBarrier();
}
`, 1, func(k uint32) uint32 { return k }, 64},
		{"workgroup memory in a one-invocation workgroup", `
var<workgroup> w: u32;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1)
fn main() {
    w = 7u;
    workgroupBarrier();
    out[0] = w;
}
`, 1, func(uint32) uint32 { return 7 }, 1},
	}
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := dispatchWGSL(t, dev, tc.wgsl, tc.groups, tc.words)
			for k := range tc.words {
				if got, want := binary.LittleEndian.Uint32(out[4*k:]), tc.want(k); got != want {
					t.Errorf("out[%d] = %d, want %d", k, got, want)
				}
			}
		})
	}
}

// dispatchWGSL creates a compute pipeline for wgsl's "main", binds a zeroed
// storage buffer of the given number of words at @group(0) @binding(0),
// dispatches groups workgroups along x and returns the buffer's bytes.
func dispatchWGSL(t *testing.T, dev hal.Device, wgsl string, groups, words uint32) []byte {
	t.Helper()
	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "wgsl", Source: hal.ShaderSource{WGSL: wgsl}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer dev.DestroyShaderModule(sm)
	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:   "wgsl",
		Compute: hal.ComputeState{Module: sm, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v, want accepted", err)
	}
	defer dev.DestroyComputePipeline(pipeline)

	size := uint64(4 * words)
	buf, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: "out", Size: size, Usage: gputypes.BufferUsageStorage})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer dev.DestroyBuffer(buf)
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "out",
		Entries: []gputypes.BindGroupEntry{{
			Binding:  0,
			Resource: gputypes.BufferBinding{Buffer: buf.NativeHandle(), Size: size},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer dev.DestroyBindGroup(bg)

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "wgsl"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "wgsl"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(groups, 1, 1)
	pass.End()
	submit(t, enc)
	return buf.(*Buffer).GetData()
}
