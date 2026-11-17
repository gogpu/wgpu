// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// One shader module used by two pipelines whose layouts place its binding at
// different backend registers. DX12 numbers registers across the whole
// pipeline layout, so @group(1) @binding(0) is u1 under a layout whose group 0
// has one storage buffer and u2 under one whose group 0 has two. Each
// pipeline must get the shader compiled for its own layout, as Rust
// wgpu-hal compiles every stage with its pipeline's layout
// (wgpu-hal/src/dx12/device.rs, load_shader) (BUG-DX12-SHADER-LAYOUT-001).

const shaderLayoutWGSL = `@group(1) @binding(0) var<storage, read_write> out: array<u32, 4>;

@compute @workgroup_size(1)
fn main() {
    out[0] = 7u;
}
`

func TestShaderModuleSharedByPipelineLayouts(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		storage := func(binding uint32) gputypes.BindGroupLayoutEntry {
			return gputypes.BindGroupLayoutEntry{
				Binding:    binding,
				Visibility: gputypes.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
			}
		}
		oneBGL := newBindGroupLayout(t, device, storage(0))
		twoBGL := newBindGroupLayout(t, device, storage(0), storage(1))
		outBGL := newBindGroupLayout(t, device, storage(0))

		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: shaderLayoutWGSL})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(shader.Release)

		// The pipeline with the one-buffer group 0 is created first: the
		// module's first compile used to be reused by every later pipeline.
		pipelineOne := newLayoutPipeline(t, device, shader, oneBGL, outBGL)
		pipelineTwo := newLayoutPipeline(t, device, shader, twoBGL, outBGL)

		usage := wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst
		newWords := func() *wgpu.Buffer {
			b := newPublicBuffer(t, device, 16, usage)
			writeWords(t, device, b, 0, 0, 0, 0)
			return b
		}
		entry := func(binding uint32, b *wgpu.Buffer) wgpu.BindGroupEntry {
			return wgpu.BindGroupEntry{Binding: binding, Buffer: b, Size: 16}
		}
		decoyA, decoyB, outOne, outTwo := newWords(), newWords(), newWords(), newWords()
		groupOne := newBindGroup(t, device, oneBGL, entry(0, decoyA))
		groupTwo := newBindGroup(t, device, twoBGL, entry(0, decoyA), entry(1, decoyB))
		groupOutOne := newBindGroup(t, device, outBGL, entry(0, outOne))
		groupOutTwo := newBindGroup(t, device, outBGL, entry(0, outTwo))

		enc := newEncoder(t, device)
		pass, err := enc.BeginComputePass(nil)
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetPipeline(pipelineOne)
		pass.SetBindGroup(0, groupOne, nil)
		pass.SetBindGroup(1, groupOutOne, nil)
		pass.Dispatch(1, 1, 1)
		pass.SetPipeline(pipelineTwo)
		pass.SetBindGroup(0, groupTwo, nil)
		pass.SetBindGroup(1, groupOutTwo, nil)
		pass.Dispatch(1, 1, 1)
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		staging := newPublicBuffer(t, device, 64, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		for i, b := range []*wgpu.Buffer{outOne, outTwo, decoyA, decoyB} {
			enc.CopyBufferToBuffer(b, 0, staging, uint64(i*16), 16)
		}
		submit(t, device, enc)

		got := readPublicBuffer(t, staging, 16)
		names := []string{"out of the one-buffer layout", "out of the two-buffer layout", "group 0 binding 0", "group 0 binding 1"}
		for i, name := range names {
			want := uint32(0)
			if i < 2 {
				want = 7
			}
			if got[i*4] != want {
				t.Errorf("%s holds %d, want %d (words %v)", name, got[i*4], want, got)
			}
		}
	})
}

func newLayoutPipeline(t *testing.T, device *wgpu.Device, shader *wgpu.ShaderModule, bgls ...*wgpu.BindGroupLayout) *wgpu.ComputePipeline {
	t.Helper()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: bgls})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{Layout: layout, Module: shader, EntryPoint: "main"})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}
