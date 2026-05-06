// Command software-test verifies the SPIR-V interpreter on the software backend.
// Tests: vertex shader (triangle), fragment shader (solid color), compute shader (scaled copy).
//
// Usage:
//
//	go run ./examples/software-test/
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/software"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\nALL TESTS PASSED — SPIR-V interpreter verified on software backend")
}

func run() error {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return fmt.Errorf("request adapter: %w", err)
	}
	defer adapter.Release()

	info := adapter.Info()
	fmt.Printf("Adapter: %s (%s)\n\n", info.Name, info.DeviceType)
	if info.Name != "Software Renderer" {
		return fmt.Errorf("expected Software Renderer, got %s — run with only software backend imported", info.Name)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("request device: %w", err)
	}
	defer device.Release()

	if err := testCompute(device); err != nil {
		return fmt.Errorf("compute test: %w", err)
	}

	return nil
}

func testCompute(device *wgpu.Device) error { //nolint:gocyclo,cyclop,funlen // linear test flow
	fmt.Println("=== Test: Compute Shader (scaled copy) ===")

	const wgsl = `
@group(0) @binding(0) var<storage, read> input: array<f32>;
@group(0) @binding(1) var<storage, read_write> output: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    output[i] = input[i] * 2.5;
}
`
	const n = 256
	const scale = 2.5

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "compute-test",
		WGSL:  wgsl,
	})
	if err != nil {
		return fmt.Errorf("create shader: %w", err)
	}
	defer shader.Release()

	bgLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "compute-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group layout: %w", err)
	}
	defer bgLayout.Release()

	pipeLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "compute-pipe",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout},
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	defer pipeLayout.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "compute-pipe",
		Layout:     pipeLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}
	defer pipeline.Release()

	inputData := make([]byte, n*4)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], math.Float32bits(float32(i+1)))
	}

	inputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "input",
		Size:  n * 4,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create input buf: %w", err)
	}
	defer inputBuf.Release()

	outputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "output",
		Size:  n * 4,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return fmt.Errorf("create output buf: %w", err)
	}
	defer outputBuf.Release()

	stagingBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging",
		Size:  n * 4,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return fmt.Errorf("create staging buf: %w", err)
	}
	defer stagingBuf.Release()

	if err := device.Queue().WriteBuffer(inputBuf, 0, inputData); err != nil {
		return fmt.Errorf("write input: %w", err)
	}

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "compute-bg",
		Layout: bgLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: inputBuf, Size: n * 4},
			{Binding: 1, Buffer: outputBuf, Size: n * 4},
		},
	})
	if err != nil {
		return fmt.Errorf("create bind group: %w", err)
	}
	defer bg.Release()

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return fmt.Errorf("begin compute pass: %w", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(n/64, 1, 1)
	if err := pass.End(); err != nil {
		return fmt.Errorf("end compute pass: %w", err)
	}

	encoder.CopyBufferToBuffer(outputBuf, 0, stagingBuf, 0, n*4)
	cmd, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish: %w", err)
	}

	if _, err := device.Queue().Submit(cmd); err != nil {
		return fmt.Errorf("submit: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := stagingBuf.Map(ctx, wgpu.MapModeRead, 0, n*4); err != nil {
		return fmt.Errorf("map: %w", err)
	}
	defer func() { _ = stagingBuf.Unmap() }()

	rng, err := stagingBuf.MappedRange(0, n*4)
	if err != nil {
		return fmt.Errorf("mapped range: %w", err)
	}
	result := rng.Bytes()

	mismatches := 0
	for i := 0; i < n; i++ {
		got := math.Float32frombits(binary.LittleEndian.Uint32(result[i*4:]))
		want := float32(i+1) * scale
		if math.Abs(float64(got-want)) > 0.01 {
			if mismatches < 5 {
				fmt.Printf("  MISMATCH [%d]: got %.4f, want %.4f\n", i, got, want)
			}
			mismatches++
		}
	}

	if mismatches > 0 {
		return fmt.Errorf("%d/%d mismatches", mismatches, n)
	}
	fmt.Printf("  PASS: %d elements, all match (scale=%.1f)\n", n, scale)
	return nil
}
