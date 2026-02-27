// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Command compute-sum demonstrates a parallel reduction (sum) using a GPU
// compute shader. It uploads an array of uint32 values to the GPU, dispatches
// a compute shader that sums contiguous pairs, and reads back the partial
// results. The final summation is performed on the CPU.
//
// The example is headless (no window required) and works on any supported GPU.
package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Register all available GPU backends (Vulkan, DX12, GLES, Metal, etc.)
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// sumShaderWGSL performs pairwise addition: output[i] = input[2*i] + input[2*i+1].
// Each workgroup thread handles one output element.
const sumShaderWGSL = `
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
    let a = input[2u * i];
    let b = input[2u * i + 1u];
    output[i] = a + b;
}
`

const (
	numElements    = 256
	outCount       = numElements / 2
	inputBufSize   = uint64(numElements * 4)
	outputBufSize  = uint64(outCount * 4)
	stagingBufSize = outputBufSize
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("FATAL: %v", err)
	}
}

func run() error {
	fmt.Println("=== Compute Shader: Parallel Sum ===")
	fmt.Println()

	// Step 1: Create instance
	fmt.Print("1. Creating instance... ")
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("CreateInstance: %w", err)
	}
	defer instance.Release()
	fmt.Println("OK")

	// Step 2: Request adapter
	fmt.Print("2. Requesting adapter... ")
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return fmt.Errorf("RequestAdapter: %w", err)
	}
	defer adapter.Release()
	fmt.Printf("OK (%s)\n", adapter.Info().Name)

	// Step 3: Request device
	fmt.Print("3. Creating device... ")
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("RequestDevice: %w", err)
	}
	defer device.Release()
	fmt.Println("OK")

	// Step 4: Prepare input data [1, 2, 3, ..., numElements]
	inputData := make([]byte, inputBufSize)
	var cpuSum uint32
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], i+1)
		cpuSum += i + 1
	}
	fmt.Printf("4. Input: %d elements, CPU sum = %d\n", numElements, cpuSum)

	// Step 5: Create GPU buffers and upload data
	fmt.Print("5. Creating buffers... ")
	inputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "input",
		Size:  inputBufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create input buffer: %w", err)
	}
	defer inputBuf.Release()

	outputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "output",
		Size:  outputBufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return fmt.Errorf("create output buffer: %w", err)
	}
	defer outputBuf.Release()

	stagingBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging",
		Size:  stagingBufSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer stagingBuf.Release()

	uniformData := make([]byte, 4)
	binary.LittleEndian.PutUint32(uniformData, outCount)
	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "params",
		Size:  4,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create uniform buffer: %w", err)
	}
	defer uniformBuf.Release()

	device.Queue().WriteBuffer(inputBuf, 0, inputData)
	device.Queue().WriteBuffer(uniformBuf, 0, uniformData)
	fmt.Println("OK")

	// Step 6: Create shader and pipeline
	fmt.Print("6. Creating compute pipeline... ")
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "sum-shader",
		WGSL:  sumShaderWGSL,
	})
	if err != nil {
		return fmt.Errorf("create shader: %w", err)
	}
	defer shader.Release()

	bgLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "sum-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group layout: %w", err)
	}
	defer bgLayout.Release()

	bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "sum-bg",
		Layout: bgLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: inputBuf, Size: inputBufSize},
			{Binding: 1, Buffer: outputBuf, Size: outputBufSize},
			{Binding: 2, Buffer: uniformBuf, Size: 4},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group: %w", err)
	}
	defer bindGroup.Release()

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "sum-pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout},
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	defer pipelineLayout.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "sum-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		return fmt.Errorf("create compute pipeline: %w", err)
	}
	defer pipeline.Release()
	fmt.Println("OK")

	// Step 7: Dispatch and read back
	fmt.Print("7. Dispatching compute... ")
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return fmt.Errorf("begin compute pass: %w", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bindGroup, nil)
	pass.Dispatch((outCount+63)/64, 1, 1)
	if err := pass.End(); err != nil {
		return fmt.Errorf("end compute pass: %w", err)
	}

	encoder.CopyBufferToBuffer(outputBuf, 0, stagingBuf, 0, outputBufSize)

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish encoder: %w", err)
	}

	if err := device.Queue().Submit(cmdBuf); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fmt.Println("OK")

	// Step 8: Read results
	fmt.Print("8. Reading results... ")
	resultBytes := make([]byte, outputBufSize)
	if err := device.Queue().ReadBuffer(stagingBuf, 0, resultBytes); err != nil {
		return fmt.Errorf("read buffer: %w", err)
	}

	var gpuSum uint32
	for i := 0; i < outCount; i++ {
		gpuSum += binary.LittleEndian.Uint32(resultBytes[i*4:])
	}
	fmt.Println("OK")

	// Step 9: Verify
	return verify(cpuSum, gpuSum)
}

func verify(cpuSum, gpuSum uint32) error {
	fmt.Println()
	fmt.Printf("CPU reference sum: %d\n", cpuSum)
	fmt.Printf("GPU partial sum:   %d\n", gpuSum)

	if gpuSum == cpuSum {
		fmt.Println("PASS: GPU sum matches CPU reference")
		return nil
	}

	fmt.Printf("FAIL: mismatch (diff = %d)\n", int64(cpuSum)-int64(gpuSum))
	return fmt.Errorf("sum mismatch: GPU=%d, CPU=%d", gpuSum, cpuSum)
}
