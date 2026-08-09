// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && darwin && !rust && !(js && wasm)

package wgpu_test

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Register the Metal HAL backend. Without a real backend registered only
	// the noop/software backends are available, and neither executes compute.
	//
	// This import is why the file sits behind the integration tag: registering
	// a real backend changes adapter selection for every test in the package.
	_ "github.com/gogpu/wgpu/hal/metal"
)

// arrayLengthShader writes arrayLength(&out) into the first element of a
// runtime-sized storage array.
//
// WGSL defines arrayLength() as the number of elements bound to the binding, so
// for a 16-byte binding of u32 the only correct answer is 4. One invocation
// writes it, so the result cannot depend on dispatch size, scheduling, thread
// index, or buffer placement.
const arrayLengthShader = `
@group(0) @binding(0) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn main() {
    out[0] = arrayLength(&out);
}
`

// noArrayLengthShader has no runtime-sized array, but uses the same explicit
// pipeline layout as arrayLengthShader. It lets a test switch between compatible
// pipelines without rebinding the bind group.
const noArrayLengthShader = `
@compute @workgroup_size(1)
fn main() {}
`

// arrayLengthElems is the element count bound to the storage binding.
const arrayLengthElems = 4

type arrayLengthBindOrder uint8

const (
	pipelineBeforeBindGroup arrayLengthBindOrder = iota
	bindGroupBeforePipeline
	compatiblePipelineSwitch
)

// TestArrayLengthReportsBoundSize asserts the WGSL contract that arrayLength()
// equals the number of elements bound to the binding.
//
// This currently FAILS on Metal, returning 0x40000000 instead of 4.
//
// naga's MSL backend cannot express arrayLength() directly — MSL has no
// equivalent construct — so it lowers it to
// `1 + (_buffer_sizes.sizeN - offset - elemSize) / stride`, reading an extra
// `constant _mslBufferSizes&` entry-point argument. That argument only carries
// a [[buffer(N)]] attribute when msl.Options.PerEntryPointMap[ep].SizesBuffer
// is set, and CreateShaderModule passes msl.DefaultOptions(), which leaves the
// map nil. naga still reports the requirement via
// TranslationInfo.RequiresSizesBuffer, but hal/metal never reads that flag and
// never creates or binds a sizes buffer.
//
// The observed 0x40000000 follows from _buffer_sizes.size0 reading as zero:
// in uint, 1 + (0 - 0 - 4) / 4 underflows to 1 + 0x3FFFFFFF. Which buffer
// index Metal assigns to the un-attributed argument is not verified here.
//
// The defect is specific to Metal by construction: this extra argument exists
// only in naga's MSL backend. SPIR-V emits OpArrayLength, HLSL emits a
// NagaBufferLength helper over ByteAddressBuffer.GetDimensions, DXIL emits
// dx.op.getDimensions, and GLSL emits .length() — all derived from the bound
// resource, so there is nothing extra left to bind.
func TestArrayLengthReportsBoundSize(t *testing.T) {
	testArrayLengthReportsBoundSize(t, pipelineBeforeBindGroup)
}

// TestArrayLengthReportsBoundSizeWhenBindGroupPrecedesPipeline verifies that
// buffer-size state does not depend on whether the pipeline is set first.
func TestArrayLengthReportsBoundSizeWhenBindGroupPrecedesPipeline(t *testing.T) {
	testArrayLengthReportsBoundSize(t, bindGroupBeforePipeline)
}

// TestArrayLengthReportsBoundSizeAfterCompatiblePipelineSwitch verifies that a
// compatible bind group remains usable when switching from a pipeline that
// needs no sizes buffer to one that does.
func TestArrayLengthReportsBoundSizeAfterCompatiblePipelineSwitch(t *testing.T) {
	testArrayLengthReportsBoundSize(t, compatiblePipelineSwitch)
}

func testArrayLengthReportsBoundSize(t *testing.T, order arrayLengthBindOrder) {
	t.Helper()

	const u32Size = 4
	const nbytes = uint64(arrayLengthElems) * u32Size

	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	defer adapter.Release()

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{Label: "arraylength-repro"})
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	defer device.Release()

	info := adapter.Info()
	t.Logf("adapter: backend=%v name=%q", info.Backend, info.Name)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: arrayLengthShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []wgpu.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: wgpu.ShaderStageCompute,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer pipelineLayout.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	defer pipeline.Release()

	var noSizesPipeline *wgpu.ComputePipeline
	if order == compatiblePipelineSwitch {
		noSizesShader, shaderErr := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: noArrayLengthShader})
		if shaderErr != nil {
			t.Fatalf("CreateShaderModule(no arrayLength): %v", shaderErr)
		}
		defer noSizesShader.Release()

		noSizesPipeline, err = device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Layout:     pipelineLayout,
			Module:     noSizesShader,
			EntryPoint: "main",
		})
		if err != nil {
			t.Fatalf("CreateComputePipeline(no arrayLength): %v", err)
		}
		defer noSizesPipeline.Release()
	}

	storage, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "arraylength-storage",
		Size:  nbytes,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(storage): %v", err)
	}
	defer storage.Release()

	// Zero the buffer so a non-zero readback can only come from the dispatch.
	device.Queue().WriteBuffer(storage, 0, make([]byte, nbytes))

	bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  bgl,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: storage, Offset: 0, Size: nbytes}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bindGroup.Release()

	staging, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "arraylength-staging",
		Size:  nbytes,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(staging): %v", err)
	}
	defer staging.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	switch order {
	case pipelineBeforeBindGroup:
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, bindGroup, nil)
	case bindGroupBeforePipeline:
		pass.SetBindGroup(0, bindGroup, nil)
		pass.SetPipeline(pipeline)
	case compatiblePipelineSwitch:
		pass.SetPipeline(noSizesPipeline)
		pass.SetBindGroup(0, bindGroup, nil)
		pass.SetPipeline(pipeline)
	default:
		t.Fatalf("unknown bind order %d", order)
	}
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("ComputePass.End: %v", err)
	}

	encoder.CopyBufferToBuffer(storage, 0, staging, 0, nbytes)

	cmd, err := encoder.Finish()
	if err != nil {
		t.Fatalf("CommandEncoder.Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Queue.Submit: %v", err)
	}

	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, nbytes); err != nil {
		t.Fatalf("Buffer.Map: %v", err)
	}
	rng, err := staging.MappedRange(0, nbytes)
	if err != nil {
		t.Fatalf("Buffer.MappedRange: %v", err)
	}
	got := binary.LittleEndian.Uint32(rng.Bytes())
	staging.Unmap()

	if got != arrayLengthElems {
		t.Errorf("arrayLength(&out) = %d (0x%08X); want %d\n"+
			"backend %v derived a runtime array length from an unbound buffer-sizes argument",
			got, got, arrayLengthElems, info.Backend)
	}
}
