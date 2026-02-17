// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Command compute-sum demonstrates a parallel reduction (sum) using a GPU
// compute shader. It uploads an array of uint32 values to the GPU, dispatches
// a compute shader that sums contiguous pairs, and reads back the partial
// results. The final summation is performed on the CPU.
//
// The example is headless (no window required) and works on any Vulkan GPU.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
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
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("=== Compute Shader: Parallel Sum ===")
	fmt.Println()

	// Step 1: Initialize Vulkan
	fmt.Print("1. Initializing Vulkan... ")
	if err := vk.Init(); err != nil {
		return fmt.Errorf("vk.Init: %w", err)
	}
	fmt.Println("OK")

	// Step 2: Create instance and device (headless, no surface)
	fmt.Print("2. Creating device... ")
	device, queue, cleanup, err := createDevice()
	if err != nil {
		return fmt.Errorf("createDevice: %w", err)
	}
	defer cleanup()
	fmt.Println("OK")

	// Step 3: Prepare input data [1, 2, 3, ..., numElements]
	inputData := make([]byte, inputBufSize)
	var cpuSum uint32
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], i+1)
		cpuSum += i + 1
	}
	fmt.Printf("3. Input: %d elements, CPU sum = %d\n", numElements, cpuSum)

	// Step 4: Create GPU buffers and upload data
	fmt.Print("4. Creating buffers... ")
	bufs, err := createBuffers(device, queue, inputData)
	if err != nil {
		return err
	}
	defer bufs.destroy(device)
	fmt.Println("OK")

	// Step 5: Create shader and pipeline
	fmt.Print("5. Creating compute pipeline... ")
	pipe, err := createPipeline(device, bufs)
	if err != nil {
		return err
	}
	defer pipe.destroy(device)
	fmt.Println("OK")

	// Step 6: Dispatch and read back
	fmt.Print("6. Dispatching compute... ")
	if err := dispatch(device, queue, pipe.pipeline, pipe.bindGroup, bufs.output, bufs.staging); err != nil {
		return err
	}
	fmt.Println("OK")

	// Step 7: Read results
	fmt.Print("7. Reading results... ")
	gpuSum, err := readAndSum(queue, bufs.staging)
	if err != nil {
		return err
	}
	fmt.Println("OK")

	// Step 8: Verify
	return verify(cpuSum, gpuSum)
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
		Label: "input",
		Size:  inputBufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create input buffer: %w", err)
	}

	outputBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "output",
		Size:  outputBufSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		device.DestroyBuffer(inputBuf)
		return nil, fmt.Errorf("create output buffer: %w", err)
	}

	stagingBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "staging",
		Size:  stagingBufSize,
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
	})
	if err != nil {
		device.DestroyBuffer(inputBuf)
		device.DestroyBuffer(outputBuf)
		return nil, fmt.Errorf("create staging buffer: %w", err)
	}

	uniformData := make([]byte, 4)
	binary.LittleEndian.PutUint32(uniformData, outCount)
	uniformBuf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "params",
		Size:  4,
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
		Label:  "sum-shader",
		Source: hal.ShaderSource{WGSL: sumShaderWGSL},
	})
	if err != nil {
		return nil, fmt.Errorf("create shader: %w", err)
	}

	bgLayout, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "sum-bgl",
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
		Label:  "sum-bg",
		Layout: bgLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: bufs.input.NativeHandle(), Offset: 0, Size: inputBufSize}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: bufs.output.NativeHandle(), Offset: 0, Size: outputBufSize}},
			{Binding: 2, Resource: gputypes.BufferBinding{Buffer: bufs.uniform.NativeHandle(), Offset: 0, Size: 4}},
		},
	})
	if err != nil {
		device.DestroyBindGroupLayout(bgLayout)
		device.DestroyShaderModule(shaderModule)
		return nil, fmt.Errorf("create bind group: %w", err)
	}

	plLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "sum-pl",
		BindGroupLayouts: []hal.BindGroupLayout{bgLayout},
	})
	if err != nil {
		device.DestroyBindGroup(bg)
		device.DestroyBindGroupLayout(bgLayout)
		device.DestroyShaderModule(shaderModule)
		return nil, fmt.Errorf("create pipeline layout: %w", err)
	}

	pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "sum-pipeline",
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
		return nil, fmt.Errorf("create compute pipeline: %w", err)
	}

	return &pipelineResources{
		shader:         shaderModule,
		bgLayout:       bgLayout,
		bindGroup:      bg,
		pipelineLayout: plLayout,
		pipeline:       pipeline,
	}, nil
}

func dispatch(
	device hal.Device, queue hal.Queue,
	pipeline hal.ComputePipeline, bg hal.BindGroup,
	outputBuf, stagingBuf hal.Buffer,
) error {
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "sum-enc"})
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	if err := encoder.BeginEncoding("sum"); err != nil {
		return fmt.Errorf("begin encoding: %w", err)
	}

	pass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "sum"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch((outCount+63)/64, 1, 1)
	pass.End()

	encoder.CopyBufferToBuffer(outputBuf, stagingBuf, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: outputBufSize},
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

func readAndSum(queue hal.Queue, stagingBuf hal.Buffer) (uint32, error) {
	resultBytes := make([]byte, outputBufSize)
	if err := queue.ReadBuffer(stagingBuf, 0, resultBytes); err != nil {
		return 0, fmt.Errorf("read buffer: %w", err)
	}

	var gpuSum uint32
	for i := 0; i < outCount; i++ {
		gpuSum += binary.LittleEndian.Uint32(resultBytes[i*4:])
	}
	return gpuSum, nil
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
