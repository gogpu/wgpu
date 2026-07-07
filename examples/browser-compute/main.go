//go:build js && wasm

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"syscall/js"

	"github.com/gogpu/wgpu"
)

const computeShader = `
@group(0) @binding(0) var<storage, read> input: array<f32>;
@group(0) @binding(1) var<storage, read_write> output: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3u) {
    let i = id.x;
    if (i < arrayLength(&input)) {
        output[i] = input[i] * 2.0;
    }
}
`

func main() {
	log := func(msg string) {
		js.Global().Get("document").Call("getElementById", "log").Set("innerHTML",
			js.Global().Get("document").Call("getElementById", "log").Get("innerHTML").String()+msg+"<br>")
		fmt.Println(msg)
	}

	log("wgpu browser COMPUTE test starting...")

	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		log("FAIL: CreateInstance: " + err.Error())
		return
	}

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		log("FAIL: RequestAdapter: " + err.Error())
		return
	}
	log(fmt.Sprintf("OK: Adapter — %s", adapter.Info().Name))

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		log("FAIL: RequestDevice: " + err.Error())
		return
	}
	log("OK: Device created")

	// Create compute shader
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "double-shader",
		WGSL:  computeShader,
	})
	if err != nil {
		log("FAIL: CreateShaderModule: " + err.Error())
		return
	}
	log("OK: Compute shader created")

	// Create bind group layout
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "compute-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStagesCompute, Buffer: &wgpu.BufferBindingLayout{Type: wgpu.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStagesCompute, Buffer: &wgpu.BufferBindingLayout{Type: wgpu.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		log("FAIL: CreateBindGroupLayout: " + err.Error())
		return
	}

	// Create pipeline layout
	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "compute-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		log("FAIL: CreatePipelineLayout: " + err.Error())
		return
	}

	// Create compute pipeline
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:  "double-pipeline",
		Layout: pipelineLayout,
		Compute: wgpu.ProgrammableStageDescriptor{
			Module:     shader,
			EntryPoint: "main",
		},
	})
	if err != nil {
		log("FAIL: CreateComputePipeline: " + err.Error())
		return
	}
	log("OK: Compute pipeline created")

	// Input data: [1.0, 2.0, 3.0, 4.0]
	inputData := make([]byte, 4*4)
	for i, v := range []float32{1.0, 2.0, 3.0, 4.0} {
		binary.LittleEndian.PutUint32(inputData[i*4:], math.Float32bits(v))
	}

	inputBuf, _ := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "input",
		Size:  uint64(len(inputData)),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	device.Queue().WriteBuffer(inputBuf, 0, inputData)

	outputBuf, _ := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "output",
		Size:  uint64(len(inputData)),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})

	readBuf, _ := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "readback",
		Size:  uint64(len(inputData)),
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})

	// Create bind group
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "compute-bg",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: inputBuf, Size: uint64(len(inputData))},
			{Binding: 1, Buffer: outputBuf, Size: uint64(len(inputData))},
		},
	})
	if err != nil {
		log("FAIL: CreateBindGroup: " + err.Error())
		return
	}
	log("OK: Buffers + BindGroup created")

	// Dispatch compute
	encoder, _ := device.CreateCommandEncoder(nil)
	pass := encoder.BeginComputePass(nil)
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.DispatchWorkgroups(1, 1, 1)
	pass.End()

	// Copy output → readback
	encoder.CopyBufferToBuffer(outputBuf, 0, readBuf, 0, uint64(len(inputData)))

	cmdBuf, _ := encoder.Finish()
	_, err = device.Queue().Submit(cmdBuf)
	if err != nil {
		log("FAIL: Submit: " + err.Error())
		return
	}
	log("OK: Compute dispatched + submitted")

	// Read back results
	err = readBuf.MapAsync(wgpu.MapModeRead, 0, uint64(len(inputData)), func(status wgpu.BufferMapAsyncStatus) {
		if status != wgpu.BufferMapAsyncStatusSuccess {
			log(fmt.Sprintf("FAIL: MapAsync status=%d", status))
			return
		}

		mapped := readBuf.GetMappedRange(0, uint64(len(inputData)))
		results := make([]float32, 4)
		for i := range results {
			results[i] = math.Float32frombits(binary.LittleEndian.Uint32(mapped[i*4:]))
		}
		readBuf.Unmap()

		log(fmt.Sprintf("Results: %v", results))

		// Verify: input * 2.0
		expected := []float32{2.0, 4.0, 6.0, 8.0}
		pass := true
		for i := range expected {
			if results[i] != expected[i] {
				log(fmt.Sprintf("FAIL: results[%d] = %f, expected %f", i, results[i], expected[i]))
				pass = false
			}
		}
		if pass {
			log("")
			log("ALL COMPUTE TESTS PASSED — browser WebGPU compute works!")
			log("Born ML can run inference in browser via WASM + WebGPU.")
		}
	})
	if err != nil {
		log("FAIL: MapAsync: " + err.Error())
		return
	}

	// Keep alive for async callback
	select {}
}
