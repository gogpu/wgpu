// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Command compute-copy demonstrates GPU buffer copying via a compute shader.
// It uploads an array of float32 values, dispatches a shader that copies
// each element from source to destination (with a scale factor), and reads
// back the results for CPU verification.
//
// The example is headless (no window required) and works on any Vulkan GPU.
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
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
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("=== Compute Shader: Scaled Copy ===")
	fmt.Println()

	// Step 1: Initialize Vulkan
	fmt.Print("1. Initializing Vulkan... ")
	if err := vk.Init(); err != nil {
		return fmt.Errorf("vk.Init: %w", err)
	}
	fmt.Println("OK")

	// Step 2: Create device
	fmt.Print("2. Creating device... ")
	device, queue, cleanup, err := createDevice()
	if err != nil {
		return fmt.Errorf("createDevice: %w", err)
	}
	defer cleanup()
	fmt.Println("OK")

	// Step 3: Prepare input data [1.0, 2.0, 3.0, ..., numElements]
	inputData := make([]byte, bufSize)
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], math.Float32bits(float32(i+1)))
	}
	fmt.Printf("3. Input: %d float32 elements, scale = %.1f\n", numElements, scaleFactor)

	// Step 4: Create GPU buffers
	fmt.Print("4. Creating buffers... ")
	bufs, err := createBuffers(device, queue, inputData)
	if err != nil {
		return err
	}
	defer bufs.destroy(device)
	fmt.Println("OK")

	// Step 5: Create compute pipeline
	fmt.Print("5. Creating compute pipeline... ")
	pipe, err := createPipeline(device, bufs)
	if err != nil {
		return err
	}
	defer pipe.destroy(device)
	fmt.Println("OK")

	// Step 6: Dispatch
	fmt.Print("6. Dispatching compute... ")
	if err := dispatch(device, queue, pipe.pipeline, pipe.bindGroup, bufs.output, bufs.staging); err != nil {
		return err
	}
	fmt.Println("OK")

	// Step 7: Read back and verify
	fmt.Print("7. Reading results... ")
	resultBytes := make([]byte, bufSize)
	if err := queue.ReadBuffer(bufs.staging, 0, resultBytes); err != nil {
		return fmt.Errorf("read buffer: %w", err)
	}
	fmt.Println("OK")

	return verifyResults(resultBytes)
}

// buffers holds all GPU buffers for the compute example.
type buffers struct {
	input   hal.Buffer
	output  hal.Buffer
	staging hal.Buffer
	uniform hal.Buffer
}

func (b *buffers) destroy(device hal.Device) {
	device.DestroyBuffer(b.input)
	device.DestroyBuffer(b.output)
	device.DestroyBuffer(b.staging)
	device.DestroyBuffer(b.uniform)
}

// pipelineResources holds all pipeline objects.
type pipelineResources struct {
	shader         hal.ShaderModule
	bgLayout       hal.BindGroupLayout
	bindGroup      hal.BindGroup
	pipelineLayout hal.PipelineLayout
	pipeline       hal.ComputePipeline
}

func (p *pipelineResources) destroy(device hal.Device) {
	device.DestroyComputePipeline(p.pipeline)
	device.DestroyPipelineLayout(p.pipelineLayout)
	device.DestroyBindGroup(p.bindGroup)
	device.DestroyBindGroupLayout(p.bgLayout)
	device.DestroyShaderModule(p.shader)
}

func createBuffers(device hal.Device, queue hal.Queue, inputData []byte) (*buffers, error) {
	inputBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "src", Size: bufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create input buffer: %w", err)
	}

	outputBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "dst", Size: bufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		device.DestroyBuffer(inputBuf)
		return nil, fmt.Errorf("create output buffer: %w", err)
	}

	stagingBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "staging", Size: bufSize,
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
	})
	if err != nil {
		device.DestroyBuffer(inputBuf)
		device.DestroyBuffer(outputBuf)
		return nil, fmt.Errorf("create staging buffer: %w", err)
	}

	// Params: count (u32) + scale (f32) = 8 bytes
	uniformData := make([]byte, uniformSize)
	binary.LittleEndian.PutUint32(uniformData[0:4], numElements)
	binary.LittleEndian.PutUint32(uniformData[4:8], math.Float32bits(scaleFactor))

	uniformBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "params", Size: uniformSize,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		device.DestroyBuffer(inputBuf)
		device.DestroyBuffer(outputBuf)
		device.DestroyBuffer(stagingBuf)
		return nil, fmt.Errorf("create uniform buffer: %w", err)
	}

	queue.WriteBuffer(inputBuf, 0, inputData)
	queue.WriteBuffer(uniformBuf, 0, uniformData)

	return &buffers{
		input:   inputBuf,
		output:  outputBuf,
		staging: stagingBuf,
		uniform: uniformBuf,
	}, nil
}

func createPipeline(device hal.Device, bufs *buffers) (*pipelineResources, error) {
	shaderModule, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "copy-shader",
		Source: hal.ShaderSource{WGSL: copyShaderWGSL},
	})
	if err != nil {
		return nil, fmt.Errorf("create shader: %w", err)
	}

	bgLayout, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "copy-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 2, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	})
	if err != nil {
		device.DestroyShaderModule(shaderModule)
		return nil, fmt.Errorf("create bind group layout: %w", err)
	}

	bg, err := device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "copy-bg",
		Layout: bgLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: bufs.input.NativeHandle(), Offset: 0, Size: bufSize}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: bufs.output.NativeHandle(), Offset: 0, Size: bufSize}},
			{Binding: 2, Resource: gputypes.BufferBinding{Buffer: bufs.uniform.NativeHandle(), Offset: 0, Size: uniformSize}},
		},
	})
	if err != nil {
		device.DestroyBindGroupLayout(bgLayout)
		device.DestroyShaderModule(shaderModule)
		return nil, fmt.Errorf("create bind group: %w", err)
	}

	plLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "copy-pl",
		BindGroupLayouts: []hal.BindGroupLayout{bgLayout},
	})
	if err != nil {
		device.DestroyBindGroup(bg)
		device.DestroyBindGroupLayout(bgLayout)
		device.DestroyShaderModule(shaderModule)
		return nil, fmt.Errorf("create pipeline layout: %w", err)
	}

	pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "copy-pipeline",
		Layout: plLayout,
		Compute: hal.ComputeState{
			Module:     shaderModule,
			EntryPoint: "main",
		},
	})
	if err != nil {
		device.DestroyPipelineLayout(plLayout)
		device.DestroyBindGroup(bg)
		device.DestroyBindGroupLayout(bgLayout)
		device.DestroyShaderModule(shaderModule)
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	return &pipelineResources{
		shader:         shaderModule,
		bgLayout:       bgLayout,
		bindGroup:      bg,
		pipelineLayout: plLayout,
		pipeline:       pipeline,
	}, nil
}

// dispatch records and submits the compute commands.
func dispatch(
	device hal.Device, queue hal.Queue,
	pipeline hal.ComputePipeline, bg hal.BindGroup,
	outputBuf, stagingBuf hal.Buffer,
) error {
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "copy-enc"})
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	if err := encoder.BeginEncoding("copy"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	pass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "copy"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch((numElements+63)/64, 1, 1)
	pass.End()

	encoder.CopyBufferToBuffer(outputBuf, stagingBuf, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: bufSize},
	})

	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("end encoding: %w", err)
	}

	fence, err := device.CreateFence()
	if err != nil {
		return fmt.Errorf("create fence: %w", err)
	}
	defer device.DestroyFence(fence)

	if err := queue.Submit([]hal.CommandBuffer{cmdBuf}, fence, 1); err != nil {
		return fmt.Errorf("submit: %w", err)
	}

	ok, err := device.Wait(fence, 1, 5*time.Second)
	if err != nil {
		return fmt.Errorf("wait: %w", err)
	}
	if !ok {
		return fmt.Errorf("fence timeout after 5s")
	}

	return nil
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

// createDevice initializes a headless Vulkan device for compute.
func createDevice() (hal.Device, hal.Queue, func(), error) {
	backend := vulkan.Backend{}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{
		Backends: gputypes.BackendsVulkan,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create instance: %w", err)
	}

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		return nil, nil, nil, fmt.Errorf("no Vulkan adapters found")
	}

	fmt.Printf("   Using: %s\n", adapters[0].Info.Name)

	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		instance.Destroy()
		return nil, nil, nil, fmt.Errorf("open device: %w", err)
	}

	cleanup := func() {
		_ = openDev.Device.WaitIdle()
		openDev.Device.Destroy()
		instance.Destroy()
	}

	return openDev.Device, openDev.Queue, cleanup, nil
}
