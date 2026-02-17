// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// sdfShaderWGSL is a compute shader that computes signed distance field
// for a circle. Each thread computes the SDF value for one pixel.
const sdfShaderWGSL = `
@group(0) @binding(0) var<storage, read_write> output: array<f32>;

struct Params {
    center_x: f32,
    center_y: f32,
    radius: f32,
    width: u32,
}
@group(0) @binding(1) var<uniform> params: Params;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let idx = id.x;
    if (idx >= arrayLength(&output)) {
        return;
    }
    let x = f32(idx % params.width);
    let y = f32(idx / params.width);
    let dx = x - params.center_x;
    let dy = y - params.center_y;
    let dist = sqrt(dx * dx + dy * dy) - params.radius;
    output[idx] = dist;
}
`

// sdfCPUReference computes the CPU reference SDF for a circle at (cx,cy)
// with the given radius, for a grid of width x height pixels.
func sdfCPUReference(cx, cy, radius float32, width, height uint32) []float32 {
	result := make([]float32, width*height)
	for y := uint32(0); y < height; y++ {
		for x := uint32(0); x < width; x++ {
			dx := float32(x) - cx
			dy := float32(y) - cy
			dist := float32(math.Sqrt(float64(dx*dx+dy*dy))) - radius
			result[y*width+x] = dist
		}
	}
	return result
}

// tryCreateVulkanDevice attempts to create a Vulkan device for testing.
// Returns nil values and skips if Vulkan is not available (e.g., headless CI).
func tryCreateVulkanDevice(t *testing.T) (hal.Device, hal.Queue, func()) {
	t.Helper()

	// Initialize Vulkan library
	if err := vk.Init(); err != nil {
		t.Skipf("Vulkan not available: %v", err)
		return nil, nil, nil
	}

	backend := Backend{}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{
		Backends: gputypes.BackendsVulkan,
	})
	if err != nil {
		t.Skipf("Vulkan instance creation failed: %v", err)
		return nil, nil, nil
	}

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		t.Skipf("no Vulkan adapters found")
		return nil, nil, nil
	}

	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		instance.Destroy()
		t.Skipf("failed to open Vulkan device: %v", err)
		return nil, nil, nil
	}

	cleanup := func() {
		_ = openDev.Device.WaitIdle()
		openDev.Device.Destroy()
		instance.Destroy()
	}

	return openDev.Device, openDev.Queue, cleanup
}

// TestComputeSDFIntegration exercises the full compute pipeline:
// WGSL source -> shader module (naga compiles internally) -> compute pipeline ->
// buffer creation -> bind group -> dispatch -> CopyBufferToBuffer -> ReadBuffer ->
// CPU verification with tolerance.
//
// The test is skipped if no Vulkan GPU is available (headless CI).
//
func TestComputeSDFIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}

	device, queue, cleanup := tryCreateVulkanDevice(t)
	if device == nil {
		return
	}
	defer cleanup()

	// Grid parameters
	const (
		gridWidth   = 16
		gridHeight  = 16
		totalPixels = gridWidth * gridHeight
		centerX     = 8.0
		centerY     = 8.0
		radius      = 5.0
	)

	// Step 1: Create shader module from WGSL (naga compiles internally)
	shaderModule, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "sdf-compute",
		Source: hal.ShaderSource{WGSL: sdfShaderWGSL},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule failed: %v", err)
	}
	defer device.DestroyShaderModule(shaderModule)

	// Step 2: Create storage buffer for output (totalPixels * 4 bytes for f32)
	outputBufferSize := uint64(totalPixels * 4)
	outputBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf-output",
		Size:  outputBufferSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer (output) failed: %v", err)
	}
	defer device.DestroyBuffer(outputBuffer)

	// Step 3: Create staging buffer for readback (host-visible)
	stagingBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf-staging",
		Size:  outputBufferSize,
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
	})
	if err != nil {
		t.Fatalf("CreateBuffer (staging) failed: %v", err)
	}
	defer device.DestroyBuffer(stagingBuffer)

	// Step 4: Create uniform buffer with circle parameters
	// Layout: center_x (f32), center_y (f32), radius (f32), width (u32) = 16 bytes
	uniformData := make([]byte, 16)
	binary.LittleEndian.PutUint32(uniformData[0:4], math.Float32bits(centerX))
	binary.LittleEndian.PutUint32(uniformData[4:8], math.Float32bits(centerY))
	binary.LittleEndian.PutUint32(uniformData[8:12], math.Float32bits(radius))
	binary.LittleEndian.PutUint32(uniformData[12:16], gridWidth)

	uniformBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "sdf-params",
		Size:  16,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer (uniform) failed: %v", err)
	}
	defer device.DestroyBuffer(uniformBuffer)

	// Write uniform data
	queue.WriteBuffer(uniformBuffer, 0, uniformData)

	// Step 5: Create bind group layout
	bgLayout, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "sdf-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeUniform,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout failed: %v", err)
	}
	defer device.DestroyBindGroupLayout(bgLayout)

	// Step 6: Create bind group with buffer bindings
	bg, err := device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "sdf-bg",
		Layout: bgLayout,
		Entries: []gputypes.BindGroupEntry{
			{
				Binding:  0,
				Resource: gputypes.BufferBinding{Buffer: outputBuffer.NativeHandle(), Offset: 0, Size: outputBufferSize},
			},
			{
				Binding:  1,
				Resource: gputypes.BufferBinding{Buffer: uniformBuffer.NativeHandle(), Offset: 0, Size: 16},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup failed: %v", err)
	}
	defer device.DestroyBindGroup(bg)

	// Step 7: Create pipeline layout
	pipelineLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "sdf-pl",
		BindGroupLayouts: []hal.BindGroupLayout{bgLayout},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout failed: %v", err)
	}
	defer device.DestroyPipelineLayout(pipelineLayout)

	// Step 8: Create compute pipeline
	pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "sdf-pipeline",
		Layout: pipelineLayout,
		Compute: hal.ComputeState{
			Module:     shaderModule,
			EntryPoint: "main",
		},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline failed: %v", err)
	}
	defer device.DestroyComputePipeline(pipeline)

	// Step 9: Record and submit compute commands
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "sdf-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	if err := encoder.BeginEncoding("sdf-compute"); err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}

	computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{
		Label: "sdf",
	})
	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bg, nil)
	// totalPixels / 64 workgroups (256 / 64 = 4)
	computePass.Dispatch((totalPixels+63)/64, 1, 1)
	computePass.End()

	// Copy output to staging buffer for readback
	encoder.CopyBufferToBuffer(outputBuffer, stagingBuffer, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: outputBufferSize},
	})

	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}

	// Step 10: Submit with fence and wait
	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence failed: %v", err)
	}
	defer device.DestroyFence(fence)

	if err := queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	ok, err := device.Wait(fence, 1, 5*time.Second)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !ok {
		t.Fatal("fence not signaled after 5s timeout")
	}

	// Step 11: Read back results
	resultBytes := make([]byte, outputBufferSize)
	if err := queue.ReadBuffer(stagingBuffer, 0, resultBytes); err != nil {
		t.Fatalf("ReadBuffer failed: %v", err)
	}

	// Step 12: Parse results and compare with CPU reference
	gpuResults := make([]float32, totalPixels)
	for i := 0; i < totalPixels; i++ {
		bits := binary.LittleEndian.Uint32(resultBytes[i*4 : (i+1)*4])
		gpuResults[i] = math.Float32frombits(bits)
	}

	cpuResults := sdfCPUReference(centerX, centerY, radius, gridWidth, gridHeight)

	const tolerance = 0.01
	mismatchCount := 0
	for i := 0; i < totalPixels; i++ {
		diff := float32(math.Abs(float64(gpuResults[i] - cpuResults[i])))
		if diff > tolerance {
			if mismatchCount < 5 {
				x := i % gridWidth
				y := i / gridWidth
				t.Errorf("pixel (%d,%d): GPU=%.4f, CPU=%.4f, diff=%.4f",
					x, y, gpuResults[i], cpuResults[i], diff)
			}
			mismatchCount++
		}
	}

	if mismatchCount > 0 {
		t.Errorf("total mismatches: %d/%d (tolerance=%.4f)", mismatchCount, totalPixels, tolerance)
	} else {
		t.Logf("all %d pixels match within tolerance %.4f", totalPixels, tolerance)
	}

	// Spot-check known values
	// Center pixel (8,8): distance should be -radius = -5.0
	centerIdx := 8*gridWidth + 8
	if math.Abs(float64(gpuResults[centerIdx]-(-radius))) > tolerance {
		t.Errorf("center pixel SDF: got %.4f, want %.4f", gpuResults[centerIdx], -radius)
	}

	// Corner pixel (0,0): distance = sqrt(8^2 + 8^2) - 5 = sqrt(128) - 5 ~ 6.31
	expectedCorner := float32(math.Sqrt(128)) - radius
	if math.Abs(float64(gpuResults[0]-expectedCorner)) > tolerance {
		t.Errorf("corner pixel SDF: got %.4f, want %.4f", gpuResults[0], expectedCorner)
	}
}

// TestComputeSDFAPIFlow tests the compute SDF pipeline API flow using
// unit-level patterns. This verifies API contract without GPU execution.
func TestComputeSDFAPIFlow(t *testing.T) {
	t.Run("pipeline descriptor validation", func(t *testing.T) {
		device := &Device{handle: 0, cmds: nil}

		// nil descriptor
		if _, err := device.CreateComputePipeline(nil); err == nil {
			t.Error("expected error for nil descriptor")
		}

		// nil compute module
		desc := &hal.ComputePipelineDescriptor{
			Compute: hal.ComputeState{Module: nil, EntryPoint: "main"},
		}
		if _, err := device.CreateComputePipeline(desc); err == nil {
			t.Error("expected error for nil compute module")
		}
	})

	t.Run("SDF CPU reference correctness", func(t *testing.T) {
		result := sdfCPUReference(4, 4, 2, 8, 8)
		if len(result) != 64 {
			t.Fatalf("expected 64 results, got %d", len(result))
		}

		// Center should be -radius
		centerIdx := 4*8 + 4
		expected := float32(-2.0)
		if math.Abs(float64(result[centerIdx]-expected)) > 0.01 {
			t.Errorf("center SDF: got %.4f, want %.4f", result[centerIdx], expected)
		}

		// On the circle boundary (distance=radius from center) should be ~0
		// Pixel (6,4): distance from (4,4) = 2, radius = 2, SDF = 0
		boundaryIdx := 4*8 + 6
		if math.Abs(float64(result[boundaryIdx])) > 0.01 {
			t.Errorf("boundary SDF: got %.4f, want ~0", result[boundaryIdx])
		}

		// Outside pixel (0,0): distance from (4,4) = sqrt(32) ~ 5.66, SDF ~ 3.66
		outsideExpected := float32(math.Sqrt(32)) - 2
		if math.Abs(float64(result[0]-outsideExpected)) > 0.01 {
			t.Errorf("outside SDF: got %.4f, want %.4f", result[0], outsideExpected)
		}
	})
}
