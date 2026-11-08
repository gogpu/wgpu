// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && linux && !(js && wasm) && wgpu_gles_force_es

package gles

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// esComputeVariants are three small compute shaders, each reading one kind of
// binding and writing a storage buffer: a constant, a storage buffer read, and
// a uniform block read. All three are read back through CopyBufferToBuffer and
// MapBuffer.
var esComputeVariants = []struct {
	name string
	wgsl string
	want uint32 // the value of the first output word
}{
	{
		name: "constant",
		wgsl: `
@group(0) @binding(0) var<storage, read_write> output: array<u32>;
@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    output[id.x] = 7u;
}`,
		want: 7,
	},
	{
		name: "storage_buffer",
		wgsl: `
@group(0) @binding(0) var<storage, read_write> output: array<u32>;
@group(0) @binding(1) var<storage, read> input: array<u32>;
@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    output[id.x] = input[id.x] + 1u;
}`,
		want: 101, // input[0] is 100
	},
	{
		name: "uniform",
		wgsl: `
@group(0) @binding(0) var<storage, read_write> output: array<u32>;
struct Params { count: u32 }
@group(0) @binding(1) var<uniform> params: Params;
@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    output[id.x] = params.count;
}`,
		want: 42,
	},
}

// TestESComputeReadsItsBindings dispatches compute shaders on an OpenGL ES
// context and reads their output buffers back through MapBuffer.
//
// The readback used to return zeros for every buffer: Device.MapBuffer mapped
// with glMapBuffer, which OpenGL ES does not have (ES maps with
// glMapBufferRange), so the mapping failed silently and the CPU copy stayed
// zero-filled. Rendering and texture readbacks were unaffected because those
// go through the CPU shadow, which is why only compute and buffer readbacks
// looked broken on ES (P1-GLES-025 item 3). Every compute workload whose
// shader read or wrote only buffers read as zeros.
func TestESComputeReadsItsBindings(t *testing.T) {
	_, dev, queue := openForcedESDevice(t)

	const words = 8
	for _, tt := range esComputeVariants {
		t.Run(tt.name, func(t *testing.T) {
			mk := func(label string, size uint64, usage gputypes.BufferUsage) hal.Buffer {
				b, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: label, Size: size, Usage: usage})
				if err != nil {
					t.Fatalf("CreateBuffer(%s): %v", label, err)
				}
				t.Cleanup(func() { dev.DestroyBuffer(b) })
				return b
			}
			in := mk("input", words*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopyDst)
			out := mk("output", words*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
			staging := mk("staging", words*4, gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)
			params := mk("params", 4, gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)

			inData := make([]byte, words*4)
			for i := 0; i < words; i++ {
				binary.LittleEndian.PutUint32(inData[i*4:], uint32(100+i))
			}
			if err := queue.WriteBuffer(in, 0, inData); err != nil {
				t.Fatalf("WriteBuffer(input): %v", err)
			}
			if err := queue.WriteBuffer(params, 0, binary.LittleEndian.AppendUint32(nil, 42)); err != nil {
				t.Fatalf("WriteBuffer(params): %v", err)
			}

			module, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: tt.name, Source: hal.ShaderSource{WGSL: tt.wgsl}})
			if err != nil {
				t.Fatalf("CreateShaderModule: %v", err)
			}
			t.Cleanup(func() { dev.DestroyShaderModule(module) })

			entries := []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
				{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			}
			bound := []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{Buffer: out.NativeHandle(), Size: words * 4}},
				{Binding: 1, Resource: gputypes.BufferBinding{Buffer: in.NativeHandle(), Size: words * 4}},
			}
			if tt.name == "uniform" {
				entries[1] = gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}}
				bound[1] = gputypes.BindGroupEntry{Binding: 1, Resource: gputypes.BufferBinding{Buffer: params.NativeHandle(), Size: 4}}
			}
			bgl, err := dev.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{Label: tt.name, Entries: entries})
			if err != nil {
				t.Fatalf("CreateBindGroupLayout: %v", err)
			}
			bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{Label: tt.name, Layout: bgl, Entries: bound})
			if err != nil {
				t.Fatalf("CreateBindGroup: %v", err)
			}
			pl, err := dev.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{Label: tt.name, BindGroupLayouts: []hal.BindGroupLayout{bgl}})
			if err != nil {
				t.Fatalf("CreatePipelineLayout: %v", err)
			}
			pipe, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
				Label: tt.name, Layout: pl, Compute: hal.ComputeState{Module: module, EntryPoint: "main"},
			})
			if err != nil {
				t.Fatalf("CreateComputePipeline: %v", err)
			}

			enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: tt.name})
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			if err := enc.BeginEncoding(tt.name); err != nil {
				t.Fatalf("BeginEncoding: %v", err)
			}
			pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: tt.name})
			pass.SetPipeline(pipe)
			pass.SetBindGroup(0, bg, nil)
			pass.Dispatch(2, 1, 1)
			pass.End()
			enc.CopyBufferToBuffer(out, staging, []hal.BufferCopy{{Size: words * 4}})
			cmd, err := enc.EndEncoding()
			if err != nil {
				t.Fatalf("EndEncoding: %v", err)
			}
			if _, err := queue.Submit([]hal.CommandBuffer{cmd}); err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if err := dev.WaitIdle(); err != nil {
				t.Fatalf("WaitIdle: %v", err)
			}

			m, err := dev.MapBuffer(staging, 0, words*4)
			if err != nil {
				t.Fatalf("MapBuffer: %v", err)
			}
			got := make([]byte, words*4)
			copy(got, unsafe.Slice((*byte)(m.Ptr), len(got)))
			if err := dev.UnmapBuffer(staging); err != nil {
				t.Fatalf("UnmapBuffer: %v", err)
			}
			const index = 0
			if v := binary.LittleEndian.Uint32(got[index*4:]); v != tt.want {
				t.Errorf("output[%d] = %d, want %d (words %v)", index, v, tt.want, got)
			}
		})
	}
}
