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
	wgZeroWords      = 64 // workgroup array length: one word per invocation
	wgZeroDirtyFill  = 0xdead0000
	wgZeroDirtyEntry = "dirty"
	wgZeroReadEntry  = "read"
	// Many workgroups make a GPU likely to reuse a previous group's shared
	// memory. The software backend allocates fresh memory per workgroup, so
	// two are enough there; its interpreter is slow on workgroup arrays
	// (P1-SW-019), which matters under CI's race instrumentation.
	wgZeroGroupsGPU      = 16
	wgZeroGroupsSoftware = 2
)

const workgroupZeroWGSL = `
var<workgroup> wg: array<u32, 64>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(64)
fn dirty(@builtin(local_invocation_index) i: u32, @builtin(workgroup_id) w: vec3<u32>) {
    wg[i] = 0xdead0000u + i;
    workgroupBarrier();
    // Keep the stores live.
    out[w.x * 64u + i] = wg[(i + 1u) % 64u];
}

@compute @workgroup_size(64)
fn read(@builtin(local_invocation_index) i: u32, @builtin(workgroup_id) w: vec3<u32>) {
    out[w.x * 64u + i] = wg[i];
}
`

func TestWorkgroupMemoryZeroInitialized(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		groups := uint32(wgZeroGroupsGPU)
		if b.software {
			groups = wgZeroGroupsSoftware
		}
		got := runWorkgroupZero(t, device, groups, nil)
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
		words := runWorkgroupZero(t, device, groups, &off)
		for _, w := range words {
			if w&0xffff0000 == wgZeroDirtyFill {
				stale++
			}
		}
		t.Logf("zero-init off: %d of %d words stale", stale, len(words))
	})
}

// runWorkgroupZero runs the dirty pass then the read pass over groups
// workgroups, both with ZeroInitializeWorkgroupMemory zeroInit, and returns
// what the read pass saw.
func runWorkgroupZero(t *testing.T, device *wgpu.Device, groups uint32, zeroInit *bool) []uint32 {
	t.Helper()
	outWords := wgZeroWords * int(groups)
	outBytes := uint64(outWords * 4)
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
	scratch := newPublicBuffer(t, device, outBytes, usage)
	out := newPublicBuffer(t, device, outBytes, usage)
	if err := device.Queue().WriteBuffer(out, 0, make([]byte, outBytes)); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	entry := func(b *wgpu.Buffer) wgpu.BindGroupEntry {
		return wgpu.BindGroupEntry{Binding: 0, Buffer: b, Size: outBytes}
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
		pass.Dispatch(groups, 1, 1)
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	}
	staging := newPublicBuffer(t, device, outBytes, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	enc.CopyBufferToBuffer(out, 0, staging, 0, outBytes)
	submit(t, device, enc)
	return readPublicBuffer(t, staging, outWords)
}
