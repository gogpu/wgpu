// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// WebGPU zero-initializes workgroup memory unless a compute stage opts out.
// Vulkan zeroes it natively where the device enables
// shaderZeroInitializeWorkgroupMemory and with naga's polyfill (zero stores
// in the shader) otherwise, as Rust wgpu-hal picks (P1-VK-013); the other
// backends use the polyfill. The test first fills the workgroup memory of
// many workgroups with a pattern, then reads it back in a second dispatch
// without writing it: anything but zero is stale data.

const (
	wgZeroWords      = 256 // workgroup array length
	wgZeroGroupSize  = 64
	wgZeroGroups     = 16
	wgZeroDirtyFill  = 0xdead0000
	wgZeroOutWords   = wgZeroWords * wgZeroGroups
	wgZeroOutBytes   = wgZeroOutWords * 4
	wgZeroDirtyEntry = "dirty"
	wgZeroReadEntry  = "read"
)

const workgroupZeroWGSL = `
var<workgroup> wg: array<u32, 256>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(64)
fn dirty(@builtin(local_invocation_index) i: u32, @builtin(workgroup_id) w: vec3<u32>) {
    for (var j = i; j < 256u; j += 64u) {
        wg[j] = 0xdead0000u + j;
    }
    workgroupBarrier();
    // Keep the stores live.
    out[w.x * 256u + i] = wg[(i + 1u) % 256u];
}

@compute @workgroup_size(64)
fn read(@builtin(local_invocation_index) i: u32, @builtin(workgroup_id) w: vec3<u32>) {
    for (var j = i; j < 256u; j += 64u) {
        out[w.x * 256u + j] = wg[j];
    }
}
`

func TestWorkgroupMemoryZeroInitialized(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		got := runWorkgroupZero(t, device, nil)
		for i, w := range got {
			if w != 0 {
				t.Fatalf("workgroup %d word %d = %#x: workgroup memory not zero-initialized", i/wgZeroWords, i%wgZeroWords, w)
			}
		}

		// With zeroing turned off the second dispatch may see the first's
		// data. WebGPU leaves the contents undefined, so this is only
		// reported: it shows whether this device can tell the two apart.
		off := false
		stale := 0
		for _, w := range runWorkgroupZero(t, device, &off) {
			if w&0xffff0000 == wgZeroDirtyFill {
				stale++
			}
		}
		t.Logf("zero-init off: %d of %d words stale", stale, wgZeroOutWords)
	})
}

// runWorkgroupZero runs the dirty pass then the read pass, both with
// ZeroInitializeWorkgroupMemory zeroInit, and returns what the read pass saw.
func runWorkgroupZero(t *testing.T, device *wgpu.Device, zeroInit *bool) []uint32 {
	t.Helper()
	bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
		Binding:    0,
		Visibility: gputypes.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
	})
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: workgroupZeroWGSL})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline := func(entry string) *wgpu.ComputePipeline {
		p, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Layout: layout, Module: shader, EntryPoint: entry, ZeroInitializeWorkgroupMemory: zeroInit,
		})
		if err != nil {
			t.Fatalf("CreateComputePipeline(%s): %v", entry, err)
		}
		t.Cleanup(p.Release)
		return p
	}
	dirty, read := pipeline(wgZeroDirtyEntry), pipeline(wgZeroReadEntry)

	usage := wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst
	scratch := newPublicBuffer(t, device, wgZeroOutBytes, usage)
	out := newPublicBuffer(t, device, wgZeroOutBytes, usage)
	if err := device.Queue().WriteBuffer(out, 0, make([]byte, wgZeroOutBytes)); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	entry := func(b *wgpu.Buffer) wgpu.BindGroupEntry {
		return wgpu.BindGroupEntry{Binding: 0, Buffer: b, Size: wgZeroOutBytes}
	}
	scratchGroup, outGroup := newBindGroup(t, device, bgl, entry(scratch)), newBindGroup(t, device, bgl, entry(out))

	enc := newEncoder(t, device)
	for _, step := range []struct {
		pipeline *wgpu.ComputePipeline
		group    *wgpu.BindGroup
	}{{dirty, scratchGroup}, {read, outGroup}} {
		pass, err := enc.BeginComputePass(nil)
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetPipeline(step.pipeline)
		pass.SetBindGroup(0, step.group, nil)
		pass.Dispatch(wgZeroGroups, 1, 1)
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	}
	staging := newPublicBuffer(t, device, wgZeroOutBytes, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	enc.CopyBufferToBuffer(out, 0, staging, 0, wgZeroOutBytes)
	submit(t, device, enc)
	return readPublicBuffer(t, staging, wgZeroOutWords)
}
