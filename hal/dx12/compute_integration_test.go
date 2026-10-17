// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// pairSumShaderWGSL is the examples/compute-sum shader. naga lowers the
// bindings to ByteAddressBuffer t0, RWByteAddressBuffer u0 and cbuffer b0, so
// the bind group must provide a raw SRV, a raw UAV and a CBV in that order.
const pairSumShaderWGSL = `
@group(0) @binding(0) var<storage, read> input: array<u32>;
@group(0) @binding(1) var<storage, read_write> output: array<u32>;

struct Params {
    count: u32,
}
@group(0) @binding(2) var<uniform> params: Params;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    if (i >= params.count) {
        return;
    }
    output[i] = input[2u * i] + input[2u * i + 1u];
}
`

// TestComputePairSumIntegration runs examples/compute-sum through the DX12
// HAL on every adapter, with both shader paths (HLSL→FXC and naga DXIL).
// Storage buffers used to be bound as CBVs, so the dispatch wrote nothing and
// the readback was all zeros.
func TestComputePairSumIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}

	for _, path := range []struct{ name, dxil string }{{"FXC", ""}, {"DXIL", "1"}} {
		t.Run(path.name, func(t *testing.T) {
			t.Setenv("GOGPU_DX12_DXIL", path.dxil)

			raw, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsDX12})
			if err != nil {
				t.Skipf("DX12 unavailable: %v", err)
			}
			defer raw.Destroy()

			adapters := raw.EnumerateAdapters(nil)
			if len(adapters) == 0 {
				t.Skip("no DX12 adapters")
			}
			for _, a := range adapters {
				t.Run(a.Info.Name, func(t *testing.T) {
					open, err := a.Adapter.Open(0, a.Capabilities.Limits)
					if err != nil {
						t.Skipf("Open: %v", err)
					}
					// Registered first so it runs after the resource cleanups.
					t.Cleanup(open.Device.Destroy)
					runPairSum(t, open.Device, open.Queue)
				})
			}
		})
	}
}

func runPairSum(t *testing.T, dev hal.Device, queue hal.Queue) {
	t.Helper()
	const n = 256
	const outN = n / 2

	newBuf := func(label string, size uint64, usage gputypes.BufferUsage) hal.Buffer {
		b, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: label, Size: size, Usage: usage})
		if err != nil {
			t.Fatalf("CreateBuffer(%s): %v", label, err)
		}
		t.Cleanup(func() { dev.DestroyBuffer(b) })
		return b
	}
	in := newBuf("input", n*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopyDst)
	out := newBuf("output", outN*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	staging := newBuf("staging", outN*4, gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)
	params := newBuf("params", 4, gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)

	inData := make([]byte, n*4)
	for i := uint32(0); i < n; i++ {
		binary.LittleEndian.PutUint32(inData[i*4:], i+1)
	}
	if err := queue.WriteBuffer(in, 0, inData); err != nil {
		t.Fatalf("WriteBuffer(input): %v", err)
	}
	if err := queue.WriteBuffer(params, 0, binary.LittleEndian.AppendUint32(nil, outN)); err != nil {
		t.Fatalf("WriteBuffer(params): %v", err)
	}

	pipeline, bg := createPairSumPipeline(t, dev, in, out, params)

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "pair-sum"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := enc.BeginEncoding("pair-sum"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "pair-sum"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch((outN+63)/64, 1, 1)
	pass.End()
	enc.CopyBufferToBuffer(out, staging, []hal.BufferCopy{{Size: outN * 4}})
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

	m, err := dev.MapBuffer(staging, 0, outN*4)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	got := make([]byte, outN*4)
	copy(got, unsafe.Slice((*byte)(m.Ptr), outN*4))
	if err := dev.UnmapBuffer(staging); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}

	for i := uint32(0); i < outN; i++ {
		want := (2*i + 1) + (2*i + 2)
		if v := binary.LittleEndian.Uint32(got[i*4:]); v != want {
			t.Fatalf("output[%d] = %d, want %d", i, v, want)
		}
	}
}

func createPairSumPipeline(t *testing.T, dev hal.Device, in, out, params hal.Buffer) (hal.ComputePipeline, hal.BindGroup) {
	t.Helper()
	module, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "pair-sum", Source: hal.ShaderSource{WGSL: pairSumShaderWGSL},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(func() { dev.DestroyShaderModule(module) })

	bgl, err := dev.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "pair-sum",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 2, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(func() { dev.DestroyBindGroupLayout(bgl) })

	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "pair-sum",
		Layout: bgl,
		// Entry order differs from the layout order on purpose.
		Entries: []gputypes.BindGroupEntry{
			{Binding: 2, Resource: gputypes.BufferBinding{Buffer: params.NativeHandle(), Size: 4}},
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: in.NativeHandle(), Size: 256 * 4}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: out.NativeHandle(), Size: 128 * 4}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(func() { dev.DestroyBindGroup(bg) })

	pl, err := dev.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label: "pair-sum", BindGroupLayouts: []hal.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(func() { dev.DestroyPipelineLayout(pl) })

	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "pair-sum", Layout: pl,
		Compute: hal.ComputeState{Module: module, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(func() { dev.DestroyComputePipeline(pipeline) })
	return pipeline, bg
}
