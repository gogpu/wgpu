// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Command compute-copy demonstrates GPU buffer copying via a compute shader.
// It uploads an array of float32 values, dispatches a shader that copies
// each element from source to destination (with a scale factor), and reads
// back the results for CPU verification.
//
// The example is headless (no window required) and works on any supported GPU.
package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Register all available GPU backends (Vulkan, DX12, GLES, Metal, etc.)
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// copyShaderWGSL copies elements from source to destination with a scale factor.
// output[i] = input[i] * scale
const copyShaderWGSL = `
@group(0) @binding(0) var<storage, read> input: array<f32>;
@group(0) @binding(1) var<storage, read_write> output: array<f32>;

struct Params {
    count: u32,
    scale: f32,
}
@group(0) @binding(2) var<uniform> params: Params;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    if (i >= params.count) {
        return;
    }
    output[i] = input[i] * params.scale;
}
`

const (
	numElements = 1024
	scaleFactor = 2.5
	bufSize     = uint64(numElements * 4)
	uniformSize = uint64(8) // count (u32) + scale (f32)
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("FATAL: %v", err)
	}
}

func run() error {
	fmt.Println("=== Compute Shader: Scaled Copy ===")
	fmt.Println()

	device, cleanup, err := initDevice()
	if err != nil {
		return err
	}
	defer cleanup()

	inputData := prepareInput()
	fmt.Printf("4. Input: %d float32 elements, scale = %.1f\n", numElements, scaleFactor)

	buffers, err := createBuffers(device, inputData)
	if err != nil {
		return err
	}
	defer buffers.release()

	pipeline, bindGroup, err := createPipeline(device, buffers)
	if err != nil {
		return err
	}
	defer pipeline.release()

	resultBytes, err := dispatchAndReadBack(device, pipeline.pipeline, bindGroup, buffers)
	if err != nil {
		return err
	}

	return verifyResults(resultBytes)
}

func initDevice() (*wgpu.Device, func(), error) {
	fmt.Print("1. Creating instance... ")
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("CreateInstance: %w", err)
	}
	fmt.Println("OK")

	fmt.Print("2. Requesting adapter... ")
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		return nil, nil, fmt.Errorf("RequestAdapter: %w", err)
	}
	fmt.Printf("OK (%s)\n", adapter.Info().Name)

	fmt.Print("3. Creating device... ")
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, nil, fmt.Errorf("RequestDevice: %w", err)
	}
	fmt.Println("OK")

	cleanup := func() {
		device.Release()
		adapter.Release()
		instance.Release()
	}
	return device, cleanup, nil
}

func prepareInput() []byte {
	inputData := make([]byte, bufSize)
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], math.Float32bits(float32(i+1)))
	}
	return inputData
}

type bufferSet struct {
	input, output, staging, uniform *wgpu.Buffer
}

func (b *bufferSet) release() {
	b.uniform.Release()
	b.staging.Release()
	b.output.Release()
	b.input.Release()
}

func createBuffers(device *wgpu.Device, inputData []byte) (*bufferSet, error) {
	fmt.Print("5. Creating buffers... ")
	inputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "src", Size: bufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create input buffer: %w", err)
	}
	outputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "dst", Size: bufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, fmt.Errorf("create output buffer: %w", err)
	}
	stagingBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging", Size: bufSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return nil, fmt.Errorf("create staging buffer: %w", err)
	}

	uniformData := make([]byte, uniformSize)
	binary.LittleEndian.PutUint32(uniformData[0:4], numElements)
	binary.LittleEndian.PutUint32(uniformData[4:8], math.Float32bits(scaleFactor))

	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "params", Size: uniformSize,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create uniform buffer: %w", err)
	}

	if err := device.Queue().WriteBuffer(inputBuf, 0, inputData); err != nil {
		return nil, fmt.Errorf("write input buffer: %w", err)
	}
	if err := device.Queue().WriteBuffer(uniformBuf, 0, uniformData); err != nil {
		return nil, fmt.Errorf("write uniform buffer: %w", err)
	}
	fmt.Println("OK")

	return &bufferSet{input: inputBuf, output: outputBuf, staging: stagingBuf, uniform: uniformBuf}, nil
}

type pipelineSet struct {
	shader, bgLayout, pipelineLayout interface{ Release() }
	bindGroup                        *wgpu.BindGroup
	pipeline                         *wgpu.ComputePipeline
}

func (p *pipelineSet) release() {
	p.pipeline.Release()
	p.pipelineLayout.Release()
	p.bindGroup.Release()
	p.bgLayout.Release()
	p.shader.Release()
}

func createPipeline(device *wgpu.Device, bufs *bufferSet) (*pipelineSet, *wgpu.BindGroup, error) {
	fmt.Print("6. Creating compute pipeline... ")
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "copy-shader", WGSL: copyShaderWGSL,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create shader: %w", err)
	}
	bgLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "copy-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create bind group layout: %w", err)
	}
	bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "copy-bg", Layout: bgLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: bufs.input, Size: bufSize},
			{Binding: 1, Buffer: bufs.output, Size: bufSize},
			{Binding: 2, Buffer: bufs.uniform, Size: uniformSize},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create bind group: %w", err)
	}
	plLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label: "copy-pl", BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create pipeline layout: %w", err)
	}
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "copy-pipeline", Layout: plLayout, Module: shader, EntryPoint: "main",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create compute pipeline: %w", err)
	}
	fmt.Println("OK")

	ps := &pipelineSet{
		shader: shader, bgLayout: bgLayout, pipelineLayout: plLayout,
		bindGroup: bindGroup, pipeline: pipeline,
	}
	return ps, bindGroup, nil
}

func dispatchAndReadBack(device *wgpu.Device, pipeline *wgpu.ComputePipeline, bindGroup *wgpu.BindGroup, bufs *bufferSet) ([]byte, error) {
	fmt.Print("7. Dispatching compute... ")
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, fmt.Errorf("create encoder: %w", err)
	}
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return nil, fmt.Errorf("begin compute pass: %w", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bindGroup, nil)
	pass.Dispatch((numElements+63)/64, 1, 1)
	if err := pass.End(); err != nil {
		return nil, fmt.Errorf("end compute pass: %w", err)
	}
	encoder.CopyBufferToBuffer(bufs.output, 0, bufs.staging, 0, bufSize)
	cmdBuf, err := encoder.Finish()
	if err != nil {
		return nil, fmt.Errorf("finish encoder: %w", err)
	}
	if err := device.Queue().Submit(cmdBuf); err != nil {
		return nil, fmt.Errorf("submit: %w", err)
	}
	fmt.Println("OK")

	fmt.Print("8. Reading results... ")
	resultBytes := make([]byte, bufSize)
	if err := device.Queue().ReadBuffer(bufs.staging, 0, resultBytes); err != nil {
		return nil, fmt.Errorf("read buffer: %w", err)
	}
	fmt.Println("OK")
	return resultBytes, nil
}

func verifyResults(resultBytes []byte) error {
	const tolerance = 0.001
	mismatches := 0

	for i := uint32(0); i < numElements; i++ {
		bits := binary.LittleEndian.Uint32(resultBytes[i*4:])
		got := math.Float32frombits(bits)
		want := float32(i+1) * scaleFactor
		if math.Abs(float64(got-want)) > tolerance {
			if mismatches < 5 {
				fmt.Printf("  MISMATCH [%d]: got %.4f, want %.4f\n", i, got, want)
			}
			mismatches++
		}
	}

	fmt.Println()
	fmt.Println("Sample results (first 8):")
	for i := uint32(0); i < 8; i++ {
		bits := binary.LittleEndian.Uint32(resultBytes[i*4:])
		got := math.Float32frombits(bits)
		fmt.Printf("  [%d] %.1f * %.1f = %.1f\n", i, float32(i+1), scaleFactor, got)
	}

	fmt.Println()
	if mismatches == 0 {
		fmt.Printf("PASS: all %d elements match (tolerance=%.4f)\n", numElements, tolerance)
		return nil
	}

	fmt.Printf("FAIL: %d/%d mismatches\n", mismatches, numElements)
	return fmt.Errorf("%d elements mismatched", mismatches)
}
