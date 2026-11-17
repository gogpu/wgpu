// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The built-ins WebGPU defines relative to the draw or dispatch parameters:
// vertex_index includes firstVertex (or baseVertex), instance_index includes
// firstInstance, num_workgroups is the dispatch size. D3D12's SV_VertexID,
// SV_InstanceID and SV_GroupID do not carry them, so the DX12 backend passes
// them as root constants (P1-DX12-054).

var specialConstantsBackends = []featureBackend{
	{overrideBackend: overrideBackend{name: backendVulkan, backends: gputypes.BackendsVulkan, backend: gputypes.BackendVulkan}},
	{overrideBackend: overrideBackend{name: backendDX12FXC, backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12}},
	{overrideBackend: overrideBackend{name: backendDX12DXIL, backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12, dxil: true}, dxilValidate: true},
}

func forEachSpecialConstantsDevice(t *testing.T, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	for _, b := range specialConstantsBackends {
		for _, pref := range featurePowerPreferences {
			t.Run(string(b.name)+"/"+pref.name, func(t *testing.T) {
				if b.dxil {
					t.Setenv("GOGPU_DX12_DXIL", "1")
				} else {
					t.Setenv("GOGPU_DX12_DXIL", "0")
				}
				if b.dxilValidate {
					t.Setenv("GOGPU_DX12_DXIL_VALIDATE", "1")
				}
				// A draw with firstInstance != 0 needs IndirectFirstInstance in core
				// (renderpass_native.go), so the instance part runs where the
				// adapter offers it.
				fn(t, openFeatureDevice(t, b.overrideBackend, pref.pref, 0, gputypes.Features(gputypes.FeatureIndirectFirstInstance), gputypes.DownlevelFlagsComputeShaders))
			})
		}
	}
}

// drawParamsShader places vertices 0-2 on the left and 3-5 on the right of
// the middle row, and writes instance_index into the red channel.
const drawParamsShader = `struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) @interpolate(flat) inst: u32,
}

@vertex
fn vs(@builtin(vertex_index) v: u32, @builtin(instance_index) i: u32) -> VOut {
    var p = array<vec2f, 6>(
        vec2f(-1.0, -1.0), vec2f(0.0, -1.0), vec2f(-1.0, 3.0),
        vec2f(0.0, -1.0), vec2f(1.0, -1.0), vec2f(0.0, 3.0),
    );
    return VOut(vec4f(p[v % 6u], 0.0, 1.0), i);
}

@fragment
fn fs(in: VOut) -> @location(0) vec4f {
    return vec4f(f32(in.inst) / 255.0, 0.0, 0.0, 1.0);
}
`

// checkRightTriangle checks that only the right triangle was drawn, with
// instance_index wantInstance.
func checkRightTriangle(t *testing.T, px []uint32, wantInstance uint32) {
	t.Helper()
	row := featureTargetHeight / 2 * featureTargetWidth
	if px[row+8] != 0xffffffff {
		t.Errorf("left triangle drawn (pixel %#08x): vertex_index misses the first/base vertex", px[row+8])
	}
	if got := px[row+40]; got&0xff != wantInstance || got == 0xffffffff {
		t.Errorf("right triangle pixel %#08x: want drawn with instance_index %d", got, wantInstance)
	}
}

// TestDrawFirstVertexFirstInstance checks vertex_index and instance_index of
// a draw with firstVertex 3 and firstInstance 5.
func TestDrawFirstVertexFirstInstance(t *testing.T) {
	forEachSpecialConstantsDevice(t, func(t *testing.T, device *wgpu.Device) {
		first := firstInstance(device, 5)
		px := renderFeatureTarget(t, device, drawParamsShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1, FirstVertex: 3, FirstInstance: first})
		})
		checkRightTriangle(t, px, first)
	})
}

// TestDrawIndexedBaseVertexFirstInstance checks vertex_index and
// instance_index of an indexed draw with baseVertex 3 and firstInstance 7.
func TestDrawIndexedBaseVertexFirstInstance(t *testing.T) {
	forEachSpecialConstantsDevice(t, func(t *testing.T, device *wgpu.Device) {
		first := firstInstance(device, 7)
		indices := newPublicBuffer(t, device, 8, wgpu.BufferUsageIndex|wgpu.BufferUsageCopyDst)
		if err := device.Queue().WriteBuffer(indices, 0, []byte{0, 0, 1, 0, 2, 0, 0, 0}); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		px := renderFeatureTarget(t, device, drawParamsShader, func(pass *wgpu.RenderPassEncoder) {
			pass.SetIndexBuffer(indices, gputypes.IndexFormatUint16, 0)
			pass.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 3, InstanceCount: 1, BaseVertex: 3, FirstInstance: first})
		})
		checkRightTriangle(t, px, first)
	})
}

const numWorkgroupsShader = `@group(0) @binding(0) var<storage, read_write> out: array<u32, 4>;

@compute @workgroup_size(1)
fn main(@builtin(num_workgroups) n: vec3<u32>, @builtin(workgroup_id) w: vec3<u32>) {
    if all(w == vec3<u32>(1u, 2u, 3u)) {
        out[0] = n.x;
        out[1] = n.y;
        out[2] = n.z;
        out[3] = 1u;
    }
}
`

// TestDispatchNumWorkgroups checks num_workgroups of a 2x3x4 dispatch, read
// in the last workgroup.
func TestDispatchNumWorkgroups(t *testing.T) {
	forEachSpecialConstantsDevice(t, func(t *testing.T, device *wgpu.Device) {
		bgl, pipeline, err := newPublicComputePipeline(t, device, numWorkgroupsShader, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		out := newPublicBuffer(t, device, 16, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		if err := device.Queue().WriteBuffer(out, 0, make([]byte, 16)); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl, Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: out, Size: 16}}})
		if err != nil {
			t.Fatalf("CreateBindGroup: %v", err)
		}
		t.Cleanup(bg.Release)
		staging := newPublicBuffer(t, device, 16, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass, err := enc.BeginComputePass(nil)
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, bg, nil)
		pass.Dispatch(2, 3, 4)
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		enc.CopyBufferToBuffer(out, 0, staging, 0, 16)
		cmd, err := enc.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if _, err := device.Queue().Submit(cmd); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		got := readPublicBuffer(t, staging, 4)
		want := []uint32{2, 3, 4, 1}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("num_workgroups read %v (x, y, z, written), want %v", got, want)
			}
		}
	})
}

// firstInstance returns want where the device has IndirectFirstInstance and
// 0 otherwise (core refuses a non-zero firstInstance without it).
func firstInstance(device *wgpu.Device, want uint32) uint32 {
	if device.Features().Contains(gputypes.FeatureIndirectFirstInstance) {
		return want
	}
	return 0
}
