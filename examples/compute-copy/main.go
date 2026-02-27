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

	// Step 4: Prepare input data [1.0, 2.0, 3.0, ..., numElements]
	inputData := make([]byte, bufSize)
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], math.Float32bits(float32(i+1)))
	}
	fmt.Printf("4. Input: %d float32 elements, scale = %.1f\n", numElements, scaleFactor)

	// Step 5: Create GPU buffers
	fmt.Print("5. Creating buffers... ")
	inputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "src",
		Size:  bufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create input buffer: %w", err)
	}
	defer inputBuf.Release()

	outputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "dst",
		Size:  bufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return fmt.Errorf("create output buffer: %w", err)
	}
	defer outputBuf.Release()

	stagingBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging",
		Size:  bufSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return fmt.Errorf("create staging buffer: %w", err)
	}
	defer stagingBuf.Release()

	// Params: count (u32) + scale (f32) = 8 bytes
	uniformData := make([]byte, uniformSize)
	binary.LittleEndian.PutUint32(uniformData[0:4], numElements)
	binary.LittleEndian.PutUint32(uniformData[4:8], math.Float32bits(scaleFactor))

	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "params",
		Size:  uniformSize,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create uniform buffer: %w", err)
	}
	defer uniformBuf.Release()

	device.Queue().WriteBuffer(inputBuf, 0, inputData)
	device.Queue().WriteBuffer(uniformBuf, 0, uniformData)
	fmt.Println("OK")

	// Step 6: Create compute pipeline
	fmt.Print("6. Creating compute pipeline... ")
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "copy-shader",
		WGSL:  copyShaderWGSL,
	})
	if err != nil {
		return fmt.Errorf("create shader: %w", err)
	}
	defer shader.Release()

	bgLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "copy-bgl",
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
		Label:  "copy-bg",
		Layout: bgLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: inputBuf, Size: bufSize},
			{Binding: 1, Buffer: outputBuf, Size: bufSize},
			{Binding: 2, Buffer: uniformBuf, Size: uniformSize},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group: %w", err)
	}
	defer bindGroup.Release()

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "copy-pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout},
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	defer pipelineLayout.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "copy-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		return fmt.Errorf("create compute pipeline: %w", err)
	}
	defer pipeline.Release()
	fmt.Println("OK")

	// Step 7: Dispatch compute
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
	pass.Dispatch((numElements+63)/64, 1, 1)
	if err := pass.End(); err != nil {
		return fmt.Errorf("end compute pass: %w", err)
	}

	encoder.CopyBufferToBuffer(outputBuf, 0, stagingBuf, 0, bufSize)

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish encoder: %w", err)
	}

	if err := device.Queue().Submit(cmdBuf); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fmt.Println("OK")

	// Step 8: Read back and verify
	fmt.Print("8. Reading results... ")
	resultBytes := make([]byte, bufSize)
	if err := device.Queue().ReadBuffer(stagingBuf, 0, resultBytes); err != nil {
		return fmt.Errorf("read buffer: %w", err)
	}
	fmt.Println("OK")

	return verifyResults(resultBytes)
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

	// Print sample results
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
