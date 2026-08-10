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

const twoBufferSizesShader = `
@group(0) @binding(0) var<storage, read_write> first: array<u32>;
@group(0) @binding(1) var<storage, read_write> second: array<u32>;

@compute @workgroup_size(1)
fn main() {
    first[0] = arrayLength(&first);
    first[1] += 1u;
    second[0] = arrayLength(&second);
    second[1] += 1u;
}
`

const oneBufferSizeShader = `
@group(0) @binding(0) var<storage, read_write> first: array<u32>;

@compute @workgroup_size(1)
fn main() {
    first[2] = arrayLength(&first);
}
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

func TestBufferSizesRemainCorrectAcrossPipelineReuse(t *testing.T) {
	fixture := newBufferSizesFixture(t)
	firstA := newBufferSizesTestBuffer(t, fixture.device, "first-a", 4)
	secondA := newBufferSizesTestBuffer(t, fixture.device, "second-a", 7)
	firstB := newBufferSizesTestBuffer(t, fixture.device, "first-b", 5)
	secondB := newBufferSizesTestBuffer(t, fixture.device, "second-b", 9)
	groupA := fixture.newBindGroup(t, firstA, secondA)
	groupB := fixture.newBindGroup(t, firstB, secondB)

	encoder, err := fixture.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}

	pass.SetBindGroup(0, groupA, nil)
	pass.SetPipeline(fixture.oneSizePipeline)
	pass.Dispatch(1, 1, 1)

	pass.SetPipeline(fixture.twoSizesPipeline)
	pass.Dispatch(1, 1, 1)

	pass.SetBindGroup(0, groupB, nil)
	pass.Dispatch(1, 1, 1)

	pass.SetPipeline(fixture.oneSizePipeline)
	pass.Dispatch(1, 1, 1)

	pass.SetPipeline(fixture.twoSizesPipeline)
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("ComputePass.End: %v", err)
	}

	got := fixture.submitAndRead(t, encoder, firstA, secondA, firstB, secondB)
	assertBufferSizesValues(t, "first-a", got[0], []uint32{4, 1, 4})
	assertBufferSizesValues(t, "second-a", got[1], []uint32{7, 1})
	assertBufferSizesValues(t, "first-b", got[2], []uint32{5, 2, 5})
	assertBufferSizesValues(t, "second-b", got[3], []uint32{9, 2})
}

func TestBufferSizesRemainCorrectForIndirectDispatch(t *testing.T) {
	fixture := newBufferSizesFixture(t)
	first := newBufferSizesTestBuffer(t, fixture.device, "indirect-first", 6)
	second := newBufferSizesTestBuffer(t, fixture.device, "indirect-second", 11)
	group := fixture.newBindGroup(t, first, second)

	indirect, err := fixture.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "buffer-sizes-indirect-arguments",
		Size:  12,
		Usage: gputypes.BufferUsageIndirect | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(indirect): %v", err)
	}
	t.Cleanup(indirect.Release)
	arguments := make([]byte, 12)
	binary.LittleEndian.PutUint32(arguments[0:4], 1)
	binary.LittleEndian.PutUint32(arguments[4:8], 1)
	binary.LittleEndian.PutUint32(arguments[8:12], 1)
	if err := fixture.device.Queue().WriteBuffer(indirect, 0, arguments); err != nil {
		t.Fatalf("WriteBuffer(indirect): %v", err)
	}

	encoder, err := fixture.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(fixture.twoSizesPipeline)
	pass.SetBindGroup(0, group, nil)
	pass.DispatchIndirect(indirect, 0)
	if err := pass.End(); err != nil {
		t.Fatalf("ComputePass.End: %v", err)
	}

	got := fixture.submitAndRead(t, encoder, first, second)
	assertBufferSizesValues(t, "indirect-first", got[0], []uint32{6, 1})
	assertBufferSizesValues(t, "indirect-second", got[1], []uint32{11, 1})
}

type bufferSizesFixture struct {
	device           *wgpu.Device
	layout           *wgpu.BindGroupLayout
	twoSizesPipeline *wgpu.ComputePipeline
	oneSizePipeline  *wgpu.ComputePipeline
}

func newBufferSizesFixture(t *testing.T) *bufferSizesFixture {
	t.Helper()
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	t.Cleanup(adapter.Release)

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{Label: "buffer-sizes-test"})
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
			},
			{
				Binding:    1,
				Visibility: wgpu.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(layout.Release)

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{layout},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(pipelineLayout.Release)

	newPipeline := func(label, source string) *wgpu.ComputePipeline {
		t.Helper()
		shader, shaderErr := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: label,
			WGSL:  source,
		})
		if shaderErr != nil {
			t.Fatalf("CreateShaderModule(%s): %v", label, shaderErr)
		}
		t.Cleanup(shader.Release)
		pipeline, pipelineErr := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Label:      label,
			Layout:     pipelineLayout,
			Module:     shader,
			EntryPoint: "main",
		})
		if pipelineErr != nil {
			t.Fatalf("CreateComputePipeline(%s): %v", label, pipelineErr)
		}
		t.Cleanup(pipeline.Release)
		return pipeline
	}

	return &bufferSizesFixture{
		device:           device,
		layout:           layout,
		twoSizesPipeline: newPipeline("two-buffer-sizes", twoBufferSizesShader),
		oneSizePipeline:  newPipeline("one-buffer-size", oneBufferSizeShader),
	}
}

type bufferSizesTestBuffer struct {
	storage  *wgpu.Buffer
	staging  *wgpu.Buffer
	elements uint32
}

func newBufferSizesTestBuffer(t *testing.T, device *wgpu.Device, label string, elements uint32) *bufferSizesTestBuffer {
	t.Helper()
	nbytes := uint64(elements) * 4
	storage, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: label,
		Size:  nbytes,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(%s): %v", label, err)
	}
	t.Cleanup(storage.Release)
	if err := device.Queue().WriteBuffer(storage, 0, make([]byte, nbytes)); err != nil {
		t.Fatalf("WriteBuffer(%s): %v", label, err)
	}

	staging, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: label + "-staging",
		Size:  nbytes,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(%s staging): %v", label, err)
	}
	t.Cleanup(staging.Release)
	return &bufferSizesTestBuffer{storage: storage, staging: staging, elements: elements}
}

func (f *bufferSizesFixture) newBindGroup(t *testing.T, first, second *bufferSizesTestBuffer) *wgpu.BindGroup {
	t.Helper()
	group, err := f.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: f.layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: first.storage, Size: uint64(first.elements) * 4},
			{Binding: 1, Buffer: second.storage, Size: uint64(second.elements) * 4},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(group.Release)
	return group
}

func (f *bufferSizesFixture) submitAndRead(t *testing.T, encoder *wgpu.CommandEncoder, buffers ...*bufferSizesTestBuffer) [][]uint32 {
	t.Helper()
	for _, buffer := range buffers {
		nbytes := uint64(buffer.elements) * 4
		encoder.CopyBufferToBuffer(buffer.storage, 0, buffer.staging, 0, nbytes)
	}
	command, err := encoder.Finish()
	if err != nil {
		t.Fatalf("CommandEncoder.Finish: %v", err)
	}
	if _, err := f.device.Queue().Submit(command); err != nil {
		t.Fatalf("Queue.Submit: %v", err)
	}

	results := make([][]uint32, len(buffers))
	for i, buffer := range buffers {
		nbytes := uint64(buffer.elements) * 4
		if err := buffer.staging.Map(context.Background(), wgpu.MapModeRead, 0, nbytes); err != nil {
			t.Fatalf("Buffer.Map(%d): %v", i, err)
		}
		rng, err := buffer.staging.MappedRange(0, nbytes)
		if err != nil {
			t.Fatalf("Buffer.MappedRange(%d): %v", i, err)
		}
		values := make([]uint32, buffer.elements)
		for j := range values {
			values[j] = binary.LittleEndian.Uint32(rng.Bytes()[j*4:])
		}
		buffer.staging.Unmap()
		results[i] = values
	}
	return results
}

func assertBufferSizesValues(t *testing.T, name string, got, want []uint32) {
	t.Helper()
	for i, value := range want {
		if got[i] != value {
			t.Errorf("%s[%d] = %d; want %d (full result: %v)", name, i, got[i], value, got)
		}
	}
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
