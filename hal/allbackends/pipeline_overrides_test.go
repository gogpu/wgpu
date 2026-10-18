// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/wgpu"

	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// These tests run the override cases (override_cases_test.go) through the
// public API, on the adapter that RequestAdapter picks for every backend, and
// check the results by readback. TestOverridesAllAdapters runs them on every
// adapter through the HAL.

// overrideBackend selects one backend for a public API test.
type overrideBackend struct {
	name     string
	backends gputypes.Backends
	backend  gputypes.Backend // the adapter's backend; ignored for software
	software bool             // request the CPU fallback adapter
	dxil     bool             // DX12 with naga's DXIL backend instead of HLSL+FXC
}

var overrideBackends = []overrideBackend{
	{name: "vulkan", backends: gputypes.BackendsVulkan, backend: gputypes.BackendVulkan},
	{name: "dx12-fxc", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12},
	{name: "dx12-dxil", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12, dxil: true},
	{name: "gles", backends: gputypes.BackendsGL, backend: gputypes.BackendGL},
	{name: "metal", backends: gputypes.BackendsMetal, backend: gputypes.BackendMetal},
	// Primary, not All: with GL in the mask every software subtest would also
	// create and destroy a GLES instance, and destroying another GLES
	// instance in one process can crash in wglDeleteContext (an existing GLES
	// bug, not one of these tests).
	{name: "software", backends: gputypes.BackendsPrimary, software: true},
}

// forEachOverrideBackend runs fn with a device of every backend that has an
// adapter here. A backend without one is skipped.
func forEachOverrideBackend(t *testing.T, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	for _, b := range overrideBackends {
		t.Run(b.name, func(t *testing.T) {
			// DX12 reads the variable when the device is created.
			if b.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			fn(t, openOverrideDevice(t, b))
		})
	}
}

func openOverrideDevice(t *testing.T, b overrideBackend) *wgpu.Device {
	t.Helper()
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: b.backends})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)

	// Low power picks the integrated GPU on a hybrid machine.
	// TestOverridesAllAdapters covers the other adapters.
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference:      gputypes.PowerPreferenceLowPower,
		ForceFallbackAdapter: b.software,
	})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)

	info := adapter.Info()
	isCPU := info.DeviceType == gputypes.DeviceTypeCPU
	if b.software != isCPU || (!b.software && info.Backend != b.backend) {
		t.Skipf("no %s adapter (got %s, backend %v)", b.name, info.Name, info.Backend)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	t.Logf("adapter: %s (%v)", info.Name, info.Backend)
	return device
}

// TestOverrideCompute runs every override case through the public API: the
// workgroup size regression (BUG-OVERRIDE-WORKGROUP-001), override values in
// arithmetic from defaults and from pipeline constants by @id and by name,
// and an array sized by an anonymous override.
func TestOverrideCompute(t *testing.T) {
	forEachOverrideBackend(t, func(t *testing.T, device *wgpu.Device) {
		for _, c := range overrideCases() {
			t.Run(c.name, func(t *testing.T) {
				out, err := runPublicCompute(t, device, c.wgsl, c.constants, c.inputs)
				if err != nil {
					t.Fatal(err)
				}
				checkCase(t, &c, out, func(wgsl string) ([][]uint32, error) {
					return runPublicCompute(t, device, wgsl, nil, c.inputs)
				})
			})
		}
	})
}

// runPublicCompute creates a compute pipeline whose binding i is a storage
// buffer with the initial contents inputs[i], dispatches one workgroup and
// returns the buffers. A pipeline creation error is returned.
func runPublicCompute(t *testing.T, device *wgpu.Device, wgsl string, constants map[string]float64, inputs [][]uint32) ([][]uint32, error) {
	t.Helper()
	bgl, pipeline, err := newPublicComputePipeline(t, device, wgsl, constants, len(inputs))
	if err != nil {
		return nil, err
	}

	storage := make([]*wgpu.Buffer, len(inputs))
	staging := make([]*wgpu.Buffer, len(inputs))
	entries := make([]wgpu.BindGroupEntry, len(inputs))
	for i, in := range inputs {
		size := uint64(len(in) * 4)
		storage[i] = newPublicBuffer(t, device, size, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		staging[i] = newPublicBuffer(t, device, size, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		data := make([]byte, size)
		for j, w := range in {
			binary.LittleEndian.PutUint32(data[j*4:], w)
		}
		if err := device.Queue().WriteBuffer(storage[i], 0, data); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		entries[i] = wgpu.BindGroupEntry{Binding: uint32(i), Buffer: storage[i], Size: size}
	}
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "overrides-bg", Layout: bgl, Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	for i, in := range inputs {
		encoder.CopyBufferToBuffer(storage[i], 0, staging[i], 0, uint64(len(in)*4))
	}
	cmd, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	out := make([][]uint32, len(inputs))
	for i, in := range inputs {
		out[i] = readPublicBuffer(t, staging[i], len(in))
	}
	return out, nil
}

func newPublicComputePipeline(t *testing.T, device *wgpu.Device, wgsl string, constants map[string]float64, bindings int) (*wgpu.BindGroupLayout, *wgpu.ComputePipeline, error) {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "overrides", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	entries := make([]gputypes.BindGroupLayoutEntry, bindings)
	for i := range entries {
		entries[i] = gputypes.BindGroupLayoutEntry{
			Binding:    uint32(i),
			Visibility: wgpu.ShaderStageCompute,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
		}
	}
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "overrides-bgl", Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(bgl.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "overrides-pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "overrides",
		Layout:     layout,
		Module:     shader,
		EntryPoint: "main",
		Constants:  constants,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("CreateComputePipeline: %w", err)
	}
	t.Cleanup(pipeline.Release)
	return bgl, pipeline, nil
}

func newPublicBuffer(t *testing.T, device *wgpu.Device, size uint64, usage gputypes.BufferUsage) *wgpu.Buffer {
	t.Helper()
	b, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: "overrides", Size: size, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	t.Cleanup(b.Release)
	return b
}

// readPublicBuffer maps a submitted staging buffer and returns n words.
func readPublicBuffer(t *testing.T, staging *wgpu.Buffer, n int) []uint32 {
	t.Helper()
	size := uint64(n * 4)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := staging.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := staging.MappedRange(0, size)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	out := make([]uint32, n)
	data := rng.Bytes()
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	if err := staging.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	return out
}

// noOverrideShader has no overrides, so every pipeline constant is unknown
// to it.
const noOverrideShader = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 16>;

@compute @workgroup_size(1)
fn main() {
    out[0] = 42u;
}
`

// TestOverrideUnknownConstant checks that a pipeline constant that names no
// override fails pipeline creation (WebGPU "validating GPUProgrammableStage")
// with naga's error, for a module with overrides and for one without.
func TestOverrideUnknownConstant(t *testing.T) {
	forEachOverrideBackend(t, func(t *testing.T, device *wgpu.Device) {
		for _, tc := range []struct {
			name string
			wgsl string
		}{
			{name: "with-overrides", wgsl: overrideArithmeticShader},
			{name: "without-overrides", wgsl: noOverrideShader},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := newPublicComputePipeline(t, device, tc.wgsl, map[string]float64{"nope": 1}, 1)
				if err == nil {
					t.Fatal("CreateComputePipeline with an unknown pipeline constant succeeded, want an error")
				}
				if !errors.Is(err, naga.ErrOverrideNotFound) {
					t.Errorf("error %q is not naga.ErrOverrideNotFound", err)
				}
				var pce *naga.PipelineConstantError
				if !errors.As(err, &pce) || pce.Key != "nope" {
					t.Errorf("error %q has no *naga.PipelineConstantError with key \"nope\"", err)
				}
			})
		}
	})
}

// overrideRenderShader draws a full-target triangle. The vertex stage takes
// the depth and the fragment stage the color from overrides, so both render
// stages must be specialized: every naga backend rejects a module that still
// has overrides.
const overrideRenderShader = `
override depth: f32 = 0.5;
override green: f32 = 1.0;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
    return vec4<f32>(p[i], depth, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, green, 0.0, 1.0);
}
`

const (
	overrideTargetWidth  = 64 // 256 bytes per row, the copy alignment
	overrideTargetHeight = 4
)

// TestOverrideRenderStages renders with overrides in the vertex and the
// fragment stage of one shader module, with their defaults (the public render
// stages have no pipeline constants), and checks a pixel.
func TestOverrideRenderStages(t *testing.T) {
	forEachOverrideBackend(t, func(t *testing.T, device *wgpu.Device) {
		want := [4]byte{0, 255, 0, 255}
		px := renderAndRead(t, device, overrideRenderShader)
		if px == want {
			return
		}
		c := overrideCase{wgsl: overrideRenderShader}
		if control := renderAndRead(t, device, c.controlShader()); control != want {
			t.Skipf("pixel (0,0) = %v, want %v; the control shader without overrides gives %v: not an override failure", px, want, control)
		}
		t.Errorf("pixel (0,0) = %v, want %v; the control shader without overrides passes", px, want)
	})
}

// renderAndRead draws one triangle with wgsl into a new target and returns
// pixel (0,0). The copy is a second submission.
func renderAndRead(t *testing.T, device *wgpu.Device, wgsl string) [4]byte {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "overrides-target",
		Size:          wgpu.Extent3D{Width: overrideTargetWidth, Height: overrideTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	pipeline := newOverrideRenderPipeline(t, device, wgsl)
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	const words = overrideTargetWidth * overrideTargetHeight
	staging := newPublicBuffer(t, device, words*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err = device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: overrideTargetWidth * 4, RowsPerImage: overrideTargetHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: overrideTargetWidth, Height: overrideTargetHeight, DepthOrArrayLayers: 1},
	}})
	cmd, err = enc.Finish()
	if err != nil {
		t.Fatalf("Finish copy: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit copy: %v", err)
	}
	var px [4]byte
	binary.LittleEndian.PutUint32(px[:], readPublicBuffer(t, staging, words)[0])
	return px
}

func newOverrideRenderPipeline(t *testing.T, device *wgpu.Device, wgsl string) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "overrides-render", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "overrides-render-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "overrides-render",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}
