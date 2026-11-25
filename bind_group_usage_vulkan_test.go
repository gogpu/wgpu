// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && !rust && !(js && wasm)

package wgpu

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogpu/gputypes"

	// Register the Vulkan HAL backend: these tests need a backend that pools
	// command encoders and batches queue writes. Registering it changes
	// adapter selection for the whole package, hence the integration tag.
	_ "github.com/gogpu/wgpu/internal/hal/vulkan"
)

// newVulkanTrackerTestDevice returns a device on the Vulkan backend, which
// batches queue writes and pools command encoders, skipping when this machine
// cannot provide one.
func newVulkanTrackerTestDevice(t *testing.T) *Device {
	t.Helper()
	instance, err := CreateInstance(&InstanceDescriptor{Backends: BackendsVulkan})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	if got := adapter.Info().Backend; got != gputypes.BackendVulkan {
		t.Skipf("adapter backend is %v, want Vulkan", got)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	if device.impl.Core() == nil || device.impl.Core().Tracker() == nil {
		t.Skip("device has no tracker (no HAL integration)")
	}
	return device
}

// validationCounter counts Vulkan validation-layer errors (logged at Error
// level with a "vulkan: " prefix) and drops all other log records.
type validationCounter struct {
	errors *atomic.Int64
	t      *testing.T
}

func (h validationCounter) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h validationCounter) Handle(_ context.Context, rec slog.Record) error { //nolint:gocritic // signature fixed by slog.Handler
	if strings.HasPrefix(rec.Message, "vulkan: ") {
		h.errors.Add(1)
		h.t.Log(rec.Message)
	}
	return nil
}

func (h validationCounter) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h validationCounter) WithGroup(string) slog.Handler      { return h }

const storageWriteWGSL = `
@group(0) @binding(0) var output: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(4, 4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    textureStore(output, vec2<i32>(id.xy), vec4<f32>(0.0, 1.0, 0.0, 1.0));
}
`

const sampleWGSL = `
@group(0) @binding(0) var input: texture_2d<f32>;

@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    let uv = vec2<f32>(f32((index << 1u) & 2u), f32(index & 2u));
    return vec4<f32>(uv * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    return textureLoad(input, vec2<i32>(position.xy), 0);
}
`

// TestStorageTextureWriteThenSampleOnVulkan is ray-traced-triangle without
// the ray tracing: a dispatch writes a storage texture and a later render pass
// of the same encoder samples it. Neither pass's bind group texture used to be
// usage-tracked, so the texture was never transitioned out of UNDEFINED
// before the dispatch (VUID-vkCmdDraw-None-09600 at submit) nor from GENERAL
// to SHADER_READ_ONLY_OPTIMAL before the draw (VUID-vkCmdDraw-imageLayout-00344).
// The test fails on any validation error and checks the rendered color.
func TestStorageTextureWriteThenSampleOnVulkan(t *testing.T) {
	var validationErrors atomic.Int64
	previous := Logger()
	SetLogger(slog.New(validationCounter{errors: &validationErrors, t: t}))
	defer SetLogger(previous)

	instance, err := CreateInstance(&InstanceDescriptor{Backends: BackendsVulkan, Flags: gputypes.InstanceFlagsDebug})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	defer adapter.Release()
	if got := adapter.Info().Backend; got != gputypes.BackendVulkan {
		t.Skipf("adapter backend is %v, want Vulkan", got)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	defer device.Release()

	const size = 16
	storage, err := device.CreateTexture(&TextureDescriptor{
		Label:         "storage",
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageStorageBinding | TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture(storage): %v", err)
	}
	defer storage.Release()
	storageView, err := device.CreateTextureView(storage, nil)
	if err != nil {
		t.Fatalf("CreateTextureView(storage): %v", err)
	}
	defer storageView.Release()
	target, err := device.CreateTexture(&TextureDescriptor{
		Label:         "target",
		Size:          Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageRenderAttachment | TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture(target): %v", err)
	}
	defer target.Release()
	targetView, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView(target): %v", err)
	}
	defer targetView.Release()
	readback, err := device.CreateBuffer(&BufferDescriptor{
		Label: "readback",
		Size:  256 * size,
		Usage: BufferUsageCopyDst | BufferUsageMapRead,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(readback): %v", err)
	}
	defer readback.Release()

	writeShader, err := device.CreateShaderModule(&ShaderModuleDescriptor{Label: "storage-write", WGSL: storageWriteWGSL})
	if err != nil {
		t.Fatalf("CreateShaderModule(write): %v", err)
	}
	defer writeShader.Release()
	sampleShader, err := device.CreateShaderModule(&ShaderModuleDescriptor{Label: "sample", WGSL: sampleWGSL})
	if err != nil {
		t.Fatalf("CreateShaderModule(sample): %v", err)
	}
	defer sampleShader.Release()

	writeLayout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: ShaderStageCompute,
			StorageTexture: &gputypes.StorageTextureBindingLayout{
				Access:        gputypes.StorageTextureAccessWriteOnly,
				Format:        TextureFormatRGBA8Unorm,
				ViewDimension: gputypes.TextureViewDimension2D,
			},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout(write): %v", err)
	}
	defer writeLayout.Release()
	sampleLayout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: ShaderStageFragment,
			Texture: &gputypes.TextureBindingLayout{
				SampleType:    gputypes.TextureSampleTypeFloat,
				ViewDimension: gputypes.TextureViewDimension2D,
			},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout(sample): %v", err)
	}
	defer sampleLayout.Release()

	writeGroup, err := device.CreateBindGroup(&BindGroupDescriptor{
		Layout: writeLayout, Entries: []BindGroupEntry{{Binding: 0, TextureView: storageView}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup(write): %v", err)
	}
	defer writeGroup.Release()
	sampleGroup, err := device.CreateBindGroup(&BindGroupDescriptor{
		Layout: sampleLayout, Entries: []BindGroupEntry{{Binding: 0, TextureView: storageView}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup(sample): %v", err)
	}
	defer sampleGroup.Release()

	writePipelineLayout, err := device.CreatePipelineLayout(&PipelineLayoutDescriptor{BindGroupLayouts: []*BindGroupLayout{writeLayout}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout(write): %v", err)
	}
	defer writePipelineLayout.Release()
	samplePipelineLayout, err := device.CreatePipelineLayout(&PipelineLayoutDescriptor{BindGroupLayouts: []*BindGroupLayout{sampleLayout}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout(sample): %v", err)
	}
	defer samplePipelineLayout.Release()
	writePipeline, err := device.CreateComputePipeline(&ComputePipelineDescriptor{
		Layout: writePipelineLayout, Module: writeShader, EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	defer writePipeline.Release()
	samplePipeline, err := device.CreateRenderPipeline(&RenderPipelineDescriptor{
		Layout:      samplePipelineLayout,
		Vertex:      VertexState{Module: sampleShader, EntryPoint: "vs_main"},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.DefaultMultisampleState(),
		Fragment: &FragmentState{
			Module:     sampleShader,
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{{Format: TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer samplePipeline.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	cpass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	cpass.SetPipeline(writePipeline)
	cpass.SetBindGroup(0, writeGroup, nil)
	cpass.Dispatch(1, 1, 1)
	if err := cpass.End(); err != nil {
		t.Fatalf("ComputePass.End: %v", err)
	}
	runColorPass(t, enc, targetView, gputypes.LoadOpClear, func(pass *RenderPassEncoder) {
		pass.SetPipeline(samplePipeline)
		pass.SetBindGroup(0, sampleGroup, nil)
		pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	})
	enc.CopyTextureToBuffer(target, readback, []BufferTextureCopy{{
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: size},
		TextureBase:  ImageCopyTexture{Texture: target},
		Size:         Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
	}})
	cb := mustFinish(t, enc)
	// compute, transit (storage -> sampled), render pass, readback copy
	if got := len(cb.impl.HALBufferList()); got != 4 {
		t.Errorf("HAL command buffers = %d, want 4", got)
	}
	if _, err := device.Queue().Submit(cb); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := readback.Map(ctx, MapModeRead, 0, readback.Size()); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := readback.MappedRange(0, readback.Size())
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	// The dispatch covers the 4x4 texels the render pass samples at (0, 0)
	// to (3, 3); (1, 1) is well inside.
	pixel := rng.Bytes()[256+4 : 256+8]
	if want := []byte{0, 255, 0, 255}; !bytes.Equal(pixel, want) {
		t.Errorf("pixel (1, 1) = %v, want %v", pixel, want)
	}
	if err := readback.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	if n := validationErrors.Load(); n > 0 {
		t.Errorf("%d Vulkan validation error(s) logged", n)
	}
}
