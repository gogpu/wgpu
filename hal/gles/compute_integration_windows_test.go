// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// pairSumShaderWGSL is the examples/compute-sum shader: a read-only storage
// input, a read-write storage output and a uniform element count.
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

// glTestDevice is a GL device on the WGL hidden-window context, opened by
// TestWGLInstanceLifecycle, which runs the tests in this file as subtests.
type glTestDevice struct {
	adapter *Adapter
	device  hal.Device
	queue   hal.Queue
}

func (d *glTestDevice) buffer(t *testing.T, label string, size uint64, usage gputypes.BufferUsage) hal.Buffer {
	t.Helper()
	buf, err := d.device.CreateBuffer(&hal.BufferDescriptor{Label: label, Size: size, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer(%s): %v", label, err)
	}
	t.Cleanup(func() { d.device.DestroyBuffer(buf) })
	return buf
}

func (d *glTestDevice) submitAndWait(t *testing.T, enc hal.CommandEncoder) {
	t.Helper()
	cmd, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := d.queue.Submit([]hal.CommandBuffer{cmd}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := d.device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}

func (d *glTestDevice) readBack(t *testing.T, buf hal.Buffer, size uint64) []byte {
	t.Helper()
	m, err := d.device.MapBuffer(buf, 0, size)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(m.Ptr), size))
	if err := d.device.UnmapBuffer(buf); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}
	return out
}

// testGLCopyBufferToBuffer verifies that a recorded CopyBufferToBuffer
// actually moves bytes on the GPU. CopyBufferCommand used to bind
// COPY_READ/COPY_WRITE and never call glCopyBufferSubData, so every
// buffer-to-buffer copy (and all compute readback) produced zeros.
func testGLCopyBufferToBuffer(t *testing.T, d *glTestDevice) {
	const size = 256
	src := d.buffer(t, "src", size, gputypes.BufferUsageCopySrc|gputypes.BufferUsageCopyDst)
	dst := d.buffer(t, "dst", size, gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*7 + 1)
	}
	if err := d.queue.WriteBuffer(src, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}

	enc, err := d.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "copy"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := enc.BeginEncoding("copy"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	// Two regions with distinct offsets to check offset plumbing.
	enc.CopyBufferToBuffer(src, dst, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 128, Size: 128},
		{SrcOffset: 128, DstOffset: 0, Size: 128},
	})
	d.submitAndWait(t, enc)

	got := d.readBack(t, dst, size)
	for i := 0; i < 128; i++ {
		if got[128+i] != data[i] || got[i] != data[128+i] {
			t.Fatalf("byte %d: got lo=%d hi=%d, want lo=%d hi=%d",
				i, got[i], got[128+i], data[128+i], data[i])
		}
	}
}

// testGLComputePairSum runs the examples/compute-sum workload through the
// HAL: dispatch → CopyBufferToBuffer into a MapRead buffer → MapBuffer.
func testGLComputePairSum(t *testing.T, d *glTestDevice) {
	if d.adapter.caps.DownlevelFlags&gputypes.DownlevelFlagsComputeShaders == 0 {
		t.Skip("GL context has no compute shader support")
	}

	const n = 256
	const outN = n / 2
	in := d.buffer(t, "input", n*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopyDst)
	out := d.buffer(t, "output", outN*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	staging := d.buffer(t, "staging", outN*4, gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)
	params := d.buffer(t, "params", 4, gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)

	inData := make([]byte, n*4)
	for i := uint32(0); i < n; i++ {
		binary.LittleEndian.PutUint32(inData[i*4:], i+1)
	}
	if err := d.queue.WriteBuffer(in, 0, inData); err != nil {
		t.Fatalf("WriteBuffer(input): %v", err)
	}
	if err := d.queue.WriteBuffer(params, 0, binary.LittleEndian.AppendUint32(nil, outN)); err != nil {
		t.Fatalf("WriteBuffer(params): %v", err)
	}

	pipeline, bg := createPairSumPipeline(t, d.device, in, out, params)

	enc, err := d.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "sum"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := enc.BeginEncoding("sum"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "sum"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch((outN+63)/64, 1, 1)
	pass.End()
	enc.CopyBufferToBuffer(out, staging, []hal.BufferCopy{{Size: outN * 4}})
	d.submitAndWait(t, enc)

	got := d.readBack(t, staging, outN*4)
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
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: in.NativeHandle(), Size: 256 * 4}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: out.NativeHandle(), Size: 128 * 4}},
			{Binding: 2, Resource: gputypes.BufferBinding{Buffer: params.NativeHandle(), Size: 4}},
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
