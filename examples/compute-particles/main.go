// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Command compute-particles demonstrates a GPU particle simulation using a
// compute shader. Particles attract toward the center and repel from sampled
// neighbors. Results are read back to CPU each step.
//
// The example is headless (no window required) and works on any supported GPU.
// For a windowed version with real-time rendering, see gogpu/examples/particles.
//
// Usage: CGO_ENABLED=0 go run .
package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand/v2"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	_ "github.com/gogpu/wgpu/hal/allbackends"
)

const numParticles = 1024
const particleBytes = 16 // x, y, vx, vy — 4x float32
const bufSize = uint64(numParticles * particleBytes)

// Compute shader: each thread updates one particle.
// Reads all particles (for neighbor interaction), writes one output.
const shaderWGSL = `
struct Particle { pos: vec2<f32>, vel: vec2<f32>, }
struct Params { dt: f32, count: u32, }

@group(0) @binding(0) var<storage, read> pin: array<Particle>;
@group(0) @binding(1) var<storage, read_write> pout: array<Particle>;
@group(0) @binding(2) var<uniform> params: Params;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    if (i >= params.count) { return; }
    var p = pin[i];

    // Attract toward center
    p.vel += -p.pos * 0.3 * params.dt;

    // Repel from sampled neighbors
    let step = max(params.count / 16u, 1u);
    for (var j = 0u; j < params.count; j += step) {
        if (j == i) { continue; }
        let d = p.pos - pin[j].pos;
        let dist = max(length(d), 0.01);
        p.vel += normalize(d) / (dist * dist) * 0.0001 * params.dt;
    }

    p.vel *= 0.995;
    p.pos += p.vel * params.dt;

    // Wrap around [-1, 1]
    if (p.pos.x > 1.0) { p.pos.x -= 2.0; }
    if (p.pos.x < -1.0) { p.pos.x += 2.0; }
    if (p.pos.y > 1.0) { p.pos.y -= 2.0; }
    if (p.pos.y < -1.0) { p.pos.y += 2.0; }

    pout[i] = p;
}
`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func uploadInitialData(device *wgpu.Device, inputBuf, uniformBuf *wgpu.Buffer) error {
	initData := make([]byte, bufSize)
	for i := 0; i < numParticles; i++ {
		off := i * particleBytes
		binary.LittleEndian.PutUint32(initData[off:], math.Float32bits(rand.Float32()*2-1))           //nolint:gosec // example code
		binary.LittleEndian.PutUint32(initData[off+4:], math.Float32bits(rand.Float32()*2-1))         //nolint:gosec // example code
		binary.LittleEndian.PutUint32(initData[off+8:], math.Float32bits((rand.Float32()-0.5)*0.02))  //nolint:gosec // example code
		binary.LittleEndian.PutUint32(initData[off+12:], math.Float32bits((rand.Float32()-0.5)*0.02)) //nolint:gosec // example code
	}
	if err := device.Queue().WriteBuffer(inputBuf, 0, initData); err != nil {
		return fmt.Errorf("write input: %w", err)
	}
	paramData := make([]byte, 8)
	binary.LittleEndian.PutUint32(paramData[0:], math.Float32bits(0.016)) // dt ~60fps
	binary.LittleEndian.PutUint32(paramData[4:], numParticles)
	if err := device.Queue().WriteBuffer(uniformBuf, 0, paramData); err != nil {
		return fmt.Errorf("write params: %w", err)
	}
	return nil
}

func run() error {
	fmt.Println("=== GPU Particle Simulation (headless) ===")
	fmt.Println()

	// 1. Instance → Adapter → Device
	fmt.Print("1. Creating instance... ")
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("CreateInstance: %w", err)
	}
	defer instance.Release()
	fmt.Println("OK")

	fmt.Print("2. Requesting adapter... ")
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return fmt.Errorf("RequestAdapter: %w", err)
	}
	defer adapter.Release()
	fmt.Printf("OK (%s)\n", adapter.Info().Name)

	fmt.Print("3. Creating device... ")
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("RequestDevice: %w", err)
	}
	defer device.Release()
	fmt.Println("OK")

	// 2. Create shader
	fmt.Print("4. Compiling shader... ")
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "particle-shader",
		WGSL:  shaderWGSL,
	})
	if err != nil {
		return fmt.Errorf("CreateShaderModule: %w", err)
	}
	defer shader.Release()
	fmt.Println("OK")

	// 3. Create buffers
	fmt.Print("5. Creating buffers... ")
	inputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "particles-in",
		Size:  bufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return fmt.Errorf("create input buffer: %w", err)
	}
	defer inputBuf.Release()

	outputBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "particles-out",
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

	uniformBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "params",
		Size:  8, // dt (f32) + count (u32)
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create uniform buffer: %w", err)
	}
	defer uniformBuf.Release()

	if err := uploadInitialData(device, inputBuf, uniformBuf); err != nil {
		return err
	}
	fmt.Println("OK")

	// 5. Create pipeline and simulate
	fmt.Print("6. Creating compute pipeline... ")
	pipeline, bg, cleanup, err := createPipeline(device, shader, inputBuf, outputBuf, uniformBuf)
	if err != nil {
		return err
	}
	defer cleanup()
	fmt.Println("OK")

	fmt.Println()
	return simulate(device, pipeline, bg, inputBuf, outputBuf, stagingBuf)
}

func createPipeline(device *wgpu.Device, shader *wgpu.ShaderModule, inputBuf, outputBuf, uniformBuf *wgpu.Buffer) (*wgpu.ComputePipeline, *wgpu.BindGroup, func(), error) {
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "particle-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 8}},
		},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create bgl: %w", err)
	}
	pl, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "particle-pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		bgl.Release()
		return nil, nil, nil, fmt.Errorf("create pipeline layout: %w", err)
	}
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "particle-pipeline", Layout: pl, Module: shader, EntryPoint: "main",
	})
	if err != nil {
		pl.Release()
		bgl.Release()
		return nil, nil, nil, fmt.Errorf("create pipeline: %w", err)
	}
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "particle-bg", Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: inputBuf, Size: bufSize},
			{Binding: 1, Buffer: outputBuf, Size: bufSize},
			{Binding: 2, Buffer: uniformBuf, Size: 8},
		},
	})
	if err != nil {
		pipeline.Release()
		pl.Release()
		bgl.Release()
		return nil, nil, nil, fmt.Errorf("create bind group: %w", err)
	}
	cleanup := func() {
		bg.Release()
		pipeline.Release()
		pl.Release()
		bgl.Release()
	}
	return pipeline, bg, cleanup, nil
}

func simulate(device *wgpu.Device, pipeline *wgpu.ComputePipeline, bg *wgpu.BindGroup, inputBuf, outputBuf, stagingBuf *wgpu.Buffer) error {
	workgroups := uint32((numParticles + 63) / 64)

	for step := 0; step < 10; step++ {
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
		pass.Dispatch(workgroups, 1, 1)
		if err := pass.End(); err != nil {
			return fmt.Errorf("end compute pass: %w", err)
		}

		// Copy output → staging for readback, output → input for next step
		encoder.CopyBufferToBuffer(outputBuf, 0, stagingBuf, 0, bufSize)
		encoder.CopyBufferToBuffer(outputBuf, 0, inputBuf, 0, bufSize)

		cmds, err := encoder.Finish()
		if err != nil {
			return fmt.Errorf("finish encoder: %w", err)
		}
		if err := device.Queue().Submit(cmds); err != nil {
			return fmt.Errorf("submit: %w", err)
		}

		// Read back particle[0] position
		result := make([]byte, bufSize)
		if err := device.Queue().ReadBuffer(stagingBuf, 0, result); err != nil {
			return fmt.Errorf("read buffer: %w", err)
		}
		x := math.Float32frombits(binary.LittleEndian.Uint32(result[0:4]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(result[4:8]))
		vx := math.Float32frombits(binary.LittleEndian.Uint32(result[8:12]))
		vy := math.Float32frombits(binary.LittleEndian.Uint32(result[12:16]))
		fmt.Printf("Step %2d: particle[0] pos=(%.3f, %.3f) vel=(%.4f, %.4f)\n", step, x, y, vx, vy)
	}

	fmt.Println()
	fmt.Println("PASS: GPU particle simulation completed")
	return nil
}
