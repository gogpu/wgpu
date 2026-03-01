package noop_test

import (
	"sync"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// TestNoopBackendVariant tests the backend variant identification.
func TestNoopBackendVariant(t *testing.T) {
	api := noop.API{}
	if api.Variant() != gputypes.BackendEmpty {
		t.Errorf("expected BackendEmpty, got %v", api.Variant())
	}
}

// TestNoopCreateInstance tests instance creation.
func TestNoopCreateInstance(t *testing.T) {
	api := noop.API{}
	desc := &hal.InstanceDescriptor{
		Backends: gputypes.BackendsPrimary,
		Flags:    0,
	}

	instance, err := api.CreateInstance(desc)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	if instance == nil {
		t.Fatal("expected non-nil instance")
	}

	// Cleanup
	instance.Destroy()
}

// TestNoopCreateInstance_NilDescriptor tests that nil descriptor is handled.
func TestNoopCreateInstance_NilDescriptor(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance with nil descriptor failed: %v", err)
	}
	if instance == nil {
		t.Fatal("expected non-nil instance even with nil descriptor")
	}
	instance.Destroy()
}

// TestNoopEnumerateAdapters tests adapter enumeration.
func TestNoopEnumerateAdapters(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("expected at least one adapter")
	}

	// Verify adapter properties
	adapter := adapters[0]
	if adapter.Adapter == nil {
		t.Error("expected non-nil adapter")
	}
	if adapter.Info.Name != "Noop Adapter" {
		t.Errorf("expected adapter name 'Noop Adapter', got %q", adapter.Info.Name)
	}
	if adapter.Info.Backend != gputypes.BackendEmpty {
		t.Errorf("expected backend BackendEmpty, got %v", adapter.Info.Backend)
	}
	if adapter.Info.DeviceType != gputypes.DeviceTypeOther {
		t.Errorf("expected device type Other, got %v", adapter.Info.DeviceType)
	}
}

// TestNoopEnumerateAdapters_WithSurfaceHint tests enumeration with surface hint.
func TestNoopEnumerateAdapters_WithSurfaceHint(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	// Create a surface
	surface, err := instance.CreateSurface(0, 0)
	if err != nil {
		t.Fatalf("CreateSurface failed: %v", err)
	}
	defer surface.Destroy()

	// Enumerate with surface hint
	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Fatal("expected at least one adapter compatible with surface")
	}
}

// TestNoopCreateSurface tests surface creation.
func TestNoopCreateSurface(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	tests := []struct {
		name          string
		displayHandle uintptr
		windowHandle  uintptr
	}{
		{"zero handles", 0, 0},
		{"non-zero handles", 0x1234, 0x5678},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			surface, err := instance.CreateSurface(tt.displayHandle, tt.windowHandle)
			if err != nil {
				t.Fatalf("CreateSurface failed: %v", err)
			}
			if surface == nil {
				t.Fatal("expected non-nil surface")
			}
			surface.Destroy()
		})
	}
}

// TestNoopAdapterOpen tests opening a device from an adapter.
func TestNoopAdapterOpen(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter

	tests := []struct {
		name     string
		features gputypes.Features
		limits   gputypes.Limits
	}{
		{"default limits", 0, gputypes.DefaultLimits()},
		{"zero limits", 0, gputypes.Limits{}},
		{"with features", gputypes.Features(gputypes.FeatureDepthClipControl), gputypes.DefaultLimits()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openDevice, err := adapter.Open(tt.features, tt.limits)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			if openDevice.Device == nil {
				t.Error("expected non-nil device")
			}
			if openDevice.Queue == nil {
				t.Error("expected non-nil queue")
			}
			openDevice.Device.Destroy()
		})
	}
}

// TestNoopAdapterCapabilities tests adapter capability queries.
func TestNoopAdapterCapabilities(t *testing.T) {
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter

	// Test texture format capabilities
	caps := adapter.TextureFormatCapabilities(gputypes.TextureFormatRGBA8Unorm)
	if caps.Flags == 0 {
		t.Error("expected non-zero texture format capabilities")
	}

	// Test surface capabilities
	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	surfaceCaps := adapter.SurfaceCapabilities(surface)
	if surfaceCaps == nil {
		t.Fatal("expected non-nil surface capabilities")
		return
	}
	if len(surfaceCaps.Formats) == 0 {
		t.Error("expected at least one supported format")
	}
	if len(surfaceCaps.PresentModes) == 0 {
		t.Error("expected at least one present mode")
	}
	if len(surfaceCaps.AlphaModes) == 0 {
		t.Error("expected at least one alpha mode")
	}
}

// TestNoopCreateBuffer tests buffer creation.
func TestNoopCreateBuffer(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	tests := []struct {
		name string
		desc *hal.BufferDescriptor
	}{
		{
			"basic buffer",
			&hal.BufferDescriptor{
				Label: "test buffer",
				Size:  256,
				Usage: gputypes.BufferUsageVertex,
			},
		},
		{
			"mapped buffer",
			&hal.BufferDescriptor{
				Label:            "mapped buffer",
				Size:             1024,
				Usage:            gputypes.BufferUsageCopyDst,
				MappedAtCreation: true,
			},
		},
		{
			"zero size buffer",
			&hal.BufferDescriptor{
				Size:  0,
				Usage: gputypes.BufferUsageUniform,
			},
		},
		{
			"large buffer",
			&hal.BufferDescriptor{
				Size:  1024 * 1024 * 10, // 10 MB
				Usage: gputypes.BufferUsageStorage,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer, err := device.CreateBuffer(tt.desc)
			if err != nil {
				t.Fatalf("CreateBuffer failed: %v", err)
			}
			if buffer == nil {
				t.Fatal("expected non-nil buffer")
			}
			device.DestroyBuffer(buffer)
		})
	}
}

// TestNoopCreateTexture tests texture creation.
func TestNoopCreateTexture(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	tests := []struct {
		name string
		desc *hal.TextureDescriptor
	}{
		{
			"2D texture",
			&hal.TextureDescriptor{
				Label:         "2D texture",
				Size:          hal.Extent3D{Width: 512, Height: 512, DepthOrArrayLayers: 1},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     gputypes.TextureDimension2D,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageRenderAttachment,
			},
		},
		{
			"3D texture",
			&hal.TextureDescriptor{
				Size:          hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 32},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     gputypes.TextureDimension3D,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				Usage:         gputypes.TextureUsageTextureBinding,
			},
		},
		{
			"MSAA texture",
			&hal.TextureDescriptor{
				Size:          hal.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
				MipLevelCount: 1,
				SampleCount:   4,
				Dimension:     gputypes.TextureDimension2D,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				Usage:         gputypes.TextureUsageRenderAttachment,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			texture, err := device.CreateTexture(tt.desc)
			if err != nil {
				t.Fatalf("CreateTexture failed: %v", err)
			}
			if texture == nil {
				t.Fatal("expected non-nil texture")
			}
			device.DestroyTexture(texture)
		})
	}
}

// TestNoopCreateSampler tests sampler creation.
func TestNoopCreateSampler(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	tests := []struct {
		name string
		desc *hal.SamplerDescriptor
	}{
		{
			"default sampler",
			&hal.SamplerDescriptor{
				Label:        "default",
				AddressModeU: gputypes.AddressModeClampToEdge,
				AddressModeV: gputypes.AddressModeClampToEdge,
				AddressModeW: gputypes.AddressModeClampToEdge,
				MagFilter:    gputypes.FilterModeLinear,
				MinFilter:    gputypes.FilterModeLinear,
				MipmapFilter: gputypes.FilterModeLinear,
			},
		},
		{
			"comparison sampler",
			&hal.SamplerDescriptor{
				Compare:   gputypes.CompareFunctionLess,
				MagFilter: gputypes.FilterModeNearest,
				MinFilter: gputypes.FilterModeNearest,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler, err := device.CreateSampler(tt.desc)
			if err != nil {
				t.Fatalf("CreateSampler failed: %v", err)
			}
			if sampler == nil {
				t.Fatal("expected non-nil sampler")
			}
			device.DestroySampler(sampler)
		})
	}
}

// TestNoopCreateBindGroupLayout tests bind group layout creation.
func TestNoopCreateBindGroupLayout(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	desc := &hal.BindGroupLayoutDescriptor{
		Label: "test layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex,
				Buffer: &gputypes.BufferBindingLayout{
					Type:             gputypes.BufferBindingTypeUniform,
					HasDynamicOffset: false,
					MinBindingSize:   64,
				},
			},
		},
	}

	layout, err := device.CreateBindGroupLayout(desc)
	if err != nil {
		t.Fatalf("CreateBindGroupLayout failed: %v", err)
	}
	if layout == nil {
		t.Fatal("expected non-nil bind group layout")
	}
	device.DestroyBindGroupLayout(layout)
}

// TestNoopCreateShaderModule tests shader module creation.
func TestNoopCreateShaderModule(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	tests := []struct {
		name string
		desc *hal.ShaderModuleDescriptor
	}{
		{
			"WGSL shader",
			&hal.ShaderModuleDescriptor{
				Label: "wgsl shader",
				Source: hal.ShaderSource{
					WGSL: "@vertex fn main() -> @builtin(position) vec4<f32> { return vec4<f32>(0.0); }",
				},
			},
		},
		{
			"SPIR-V shader",
			&hal.ShaderModuleDescriptor{
				Label: "spirv shader",
				Source: hal.ShaderSource{
					SPIRV: []uint32{0x07230203, 0x00010000}, // Magic number + version
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := device.CreateShaderModule(tt.desc)
			if err != nil {
				t.Fatalf("CreateShaderModule failed: %v", err)
			}
			if module == nil {
				t.Fatal("expected non-nil shader module")
			}
			device.DestroyShaderModule(module)
		})
	}
}

// TestNoopCreateRenderPipeline tests render pipeline creation.
func TestNoopCreateRenderPipeline(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	// Create required resources
	layout, _ := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{})
	defer device.DestroyPipelineLayout(layout)

	module, _ := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Source: hal.ShaderSource{WGSL: "@vertex fn vs() {}"},
	})
	defer device.DestroyShaderModule(module)

	desc := &hal.RenderPipelineDescriptor{
		Label:  "test pipeline",
		Layout: layout,
		Vertex: hal.VertexState{
			Module:     module,
			EntryPoint: "vs",
		},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList,
		},
		Multisample: gputypes.MultisampleState{
			Count: 1,
			Mask:  0xFFFFFFFF,
		},
	}

	pipeline, err := device.CreateRenderPipeline(desc)
	if err != nil {
		t.Fatalf("CreateRenderPipeline failed: %v", err)
	}
	if pipeline == nil {
		t.Fatal("expected non-nil render pipeline")
	}
	device.DestroyRenderPipeline(pipeline)
}

// TestNoopCreateComputePipeline tests compute pipeline creation.
func TestNoopCreateComputePipeline(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	// Create required resources
	layout, _ := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{})
	defer device.DestroyPipelineLayout(layout)

	module, _ := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Source: hal.ShaderSource{WGSL: "@compute fn main() {}"},
	})
	defer device.DestroyShaderModule(module)

	desc := &hal.ComputePipelineDescriptor{
		Label:  "test compute pipeline",
		Layout: layout,
		Compute: hal.ComputeState{
			Module:     module,
			EntryPoint: "main",
		},
	}

	pipeline, err := device.CreateComputePipeline(desc)
	if err != nil {
		t.Fatalf("CreateComputePipeline failed: %v", err)
	}
	if pipeline == nil {
		t.Fatal("expected non-nil compute pipeline")
	}
	device.DestroyComputePipeline(pipeline)
}

// TestNoopCreateFence tests fence creation and operations.
func TestNoopCreateFence(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence failed: %v", err)
	}
	if fence == nil {
		t.Fatal("expected non-nil fence")
	}
	defer device.DestroyFence(fence)

	// Test fence wait (should return immediately)
	ok, err := device.Wait(fence, 0, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !ok {
		t.Error("expected Wait to succeed immediately for value 0")
	}
}

// TestNoopQueueSubmit tests command buffer submission.
func TestNoopQueueSubmit(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	// Create command encoder and buffer
	encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test"})
	_ = encoder.BeginEncoding("test")
	cmdBuffer, _ := encoder.EndEncoding()

	// Submit without fence
	err := queue.Submit([]hal.CommandBuffer{cmdBuffer}, nil, 0)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Submit with fence
	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	err = queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1)
	if err != nil {
		t.Fatalf("Submit with fence failed: %v", err)
	}

	// Verify fence was signaled
	ok, _ := device.Wait(fence, 1, 100*time.Millisecond)
	if !ok {
		t.Error("expected fence to be signaled after submit")
	}
}

// TestNoopQueueWriteBuffer tests immediate buffer writes.
func TestNoopQueueWriteBuffer(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	// Create a mapped buffer
	buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{
		Size:             256,
		Usage:            gputypes.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	defer device.DestroyBuffer(buffer)

	// Write data
	data := []byte{1, 2, 3, 4}
	if err := queue.WriteBuffer(buffer, 0, data); err != nil {
		t.Fatalf("WriteBuffer failed: %v", err)
	}
}

// TestNoopQueueWriteTexture tests immediate texture writes.
func TestNoopQueueWriteTexture(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	texture, _ := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageCopyDst,
	})
	defer device.DestroyTexture(texture)

	data := make([]byte, 4*4*4) // 4x4 RGBA
	dst := &hal.ImageCopyTexture{
		Texture:  texture,
		MipLevel: 0,
		Aspect:   gputypes.TextureAspectAll,
	}
	layout := &hal.ImageDataLayout{
		BytesPerRow: 16,
	}
	size := &hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}

	if err := queue.WriteTexture(dst, data, layout, size); err != nil {
		t.Fatalf("WriteTexture failed: %v", err)
	}
}

// TestNoopCommandEncoder tests command encoder operations.
func TestNoopCommandEncoder(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "test encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	// Begin encoding
	err = encoder.BeginEncoding("test")
	if err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}

	// Test various commands
	buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{Size: 256})
	defer device.DestroyBuffer(buffer)

	encoder.ClearBuffer(buffer, 0, 256)
	encoder.CopyBufferToBuffer(buffer, buffer, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 128, Size: 64},
	})

	// End encoding
	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}
	if cmdBuffer == nil {
		t.Fatal("expected non-nil command buffer")
	}
}

// TestNoopRenderPass tests render pass operations.
func TestNoopRenderPass(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	// Create resources
	texture, _ := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	defer device.DestroyTexture(texture)

	view, _ := device.CreateTextureView(texture, &hal.TextureViewDescriptor{})
	defer device.DestroyTextureView(view)

	encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = encoder.BeginEncoding("test")

	// Begin render pass
	renderPass := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0.0, G: 0.0, B: 0.0, A: 1.0},
			},
		},
	})

	// Test render pass commands
	renderPass.SetViewport(0, 0, 256, 256, 0, 1)
	renderPass.SetScissorRect(0, 0, 256, 256)
	renderPass.Draw(3, 1, 0, 0)
	renderPass.End()

	_, _ = encoder.EndEncoding()
}

// TestNoopComputePass tests compute pass operations.
func TestNoopComputePass(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = encoder.BeginEncoding("test")

	// Begin compute pass
	computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{
		Label: "test compute",
	})

	// Test compute pass commands
	computePass.Dispatch(8, 8, 1)
	computePass.End()

	_, _ = encoder.EndEncoding()
}

// TestNoopComputeE2E tests the full compute pipeline end-to-end:
// shader → bind group → pipeline → dispatch → submit → readback.
func TestNoopComputeE2E(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	// 1. Create compute shader module
	module, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "compute-shader",
		Source: hal.ShaderSource{
			WGSL: `@group(0) @binding(0) var<storage, read_write> data: array<u32>;
@compute @workgroup_size(64) fn main(@builtin(global_invocation_id) id: vec3<u32>) {
  data[id.x] = id.x * 2u;
}`,
		},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule failed: %v", err)
	}
	defer device.DestroyShaderModule(module)

	// 2. Create storage buffer with initial data
	const bufferSize = 256 * 4 // 256 uint32 values
	buffer, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "storage-buffer",
		Size:             bufferSize,
		Usage:            gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	defer device.DestroyBuffer(buffer)

	// Write initial data (zeros)
	initialData := make([]byte, bufferSize)
	if err := queue.WriteBuffer(buffer, 0, initialData); err != nil {
		t.Fatalf("WriteBuffer failed: %v", err)
	}

	// 3. Create bind group layout
	bgLayout, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "compute-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout failed: %v", err)
	}
	defer device.DestroyBindGroupLayout(bgLayout)

	// 4. Create bind group
	bg, err := device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "compute-bg",
		Layout: bgLayout,
		Entries: []gputypes.BindGroupEntry{
			{
				Binding:  0,
				Resource: gputypes.BufferBinding{Buffer: 0, Offset: 0, Size: bufferSize},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup failed: %v", err)
	}
	defer device.DestroyBindGroup(bg)

	// 5. Create pipeline layout
	pipelineLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "compute-pl",
		BindGroupLayouts: []hal.BindGroupLayout{bgLayout},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout failed: %v", err)
	}
	defer device.DestroyPipelineLayout(pipelineLayout)

	// 6. Create compute pipeline
	pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "compute-pipeline",
		Layout: pipelineLayout,
		Compute: hal.ComputeState{
			Module:     module,
			EntryPoint: "main",
		},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline failed: %v", err)
	}
	defer device.DestroyComputePipeline(pipeline)

	// 7. Record and submit compute commands
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "compute-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	if err := encoder.BeginEncoding("compute-pass"); err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}

	computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{
		Label: "compute",
	})
	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bg, nil)
	computePass.Dispatch(4, 1, 1) // 4 workgroups × 64 threads = 256 invocations
	computePass.End()

	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}

	// 8. Submit with fence
	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence failed: %v", err)
	}
	defer device.DestroyFence(fence)

	if err := queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// 9. Wait for completion
	ok, err := device.Wait(fence, 1, time.Second)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !ok {
		t.Fatal("fence not signaled after submit")
	}

	// 10. Read back results
	result := make([]byte, bufferSize)
	if err := queue.ReadBuffer(buffer, 0, result); err != nil {
		t.Fatalf("ReadBuffer failed: %v", err)
	}

	// Note: noop backend doesn't execute the shader, so results won't be
	// modified. This test verifies the full API flow completes without errors.
}

// TestNoopWriteReadBufferRoundTrip tests WriteBuffer followed by ReadBuffer.
func TestNoopWriteReadBufferRoundTrip(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	buffer, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "roundtrip-buffer",
		Size:             64,
		Usage:            gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	defer device.DestroyBuffer(buffer)

	// Write test pattern
	writeData := make([]byte, 64)
	for i := range writeData {
		writeData[i] = byte(i * 3)
	}
	if err := queue.WriteBuffer(buffer, 0, writeData); err != nil {
		t.Fatalf("WriteBuffer failed: %v", err)
	}

	// Read back
	readData := make([]byte, 64)
	if err := queue.ReadBuffer(buffer, 0, readData); err != nil {
		t.Fatalf("ReadBuffer failed: %v", err)
	}

	// Verify round-trip
	for i := range writeData {
		if readData[i] != writeData[i] {
			t.Errorf("byte %d: got %d, want %d", i, readData[i], writeData[i])
		}
	}
}

// TestNoopWriteReadBufferWithOffset tests WriteBuffer/ReadBuffer with non-zero offset.
func TestNoopWriteReadBufferWithOffset(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	buffer, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "offset-buffer",
		Size:             256,
		Usage:            gputypes.BufferUsageStorage,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	defer device.DestroyBuffer(buffer)

	// Write at offset 128
	writeData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	if err := queue.WriteBuffer(buffer, 128, writeData); err != nil {
		t.Fatalf("WriteBuffer failed: %v", err)
	}

	// Read back at same offset
	readData := make([]byte, 4)
	if err := queue.ReadBuffer(buffer, 128, readData); err != nil {
		t.Fatalf("ReadBuffer failed: %v", err)
	}

	for i := range writeData {
		if readData[i] != writeData[i] {
			t.Errorf("byte %d: got 0x%02X, want 0x%02X", i, readData[i], writeData[i])
		}
	}
}

// TestNoopSurfaceConfigure tests surface configuration.
func TestNoopSurfaceConfigure(t *testing.T) {
	api := noop.API{}
	instance, _ := api.CreateInstance(nil)
	defer instance.Destroy()

	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDevice, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDevice.Device.Destroy()

	config := &hal.SurfaceConfiguration{
		Width:       800,
		Height:      600,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeFifo,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}

	err := surface.Configure(openDevice.Device, config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	// Unconfigure
	surface.Unconfigure(openDevice.Device)
}

// TestNoopSurfaceAcquireTexture tests surface texture acquisition.
func TestNoopSurfaceAcquireTexture(t *testing.T) {
	api := noop.API{}
	instance, _ := api.CreateInstance(nil)
	defer instance.Destroy()

	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDevice, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDevice.Device.Destroy()

	// Configure surface
	config := &hal.SurfaceConfiguration{
		Width:  800,
		Height: 600,
		Format: gputypes.TextureFormatBGRA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	}
	_ = surface.Configure(openDevice.Device, config)
	defer surface.Unconfigure(openDevice.Device)

	// Acquire texture
	fence, _ := openDevice.Device.CreateFence()
	defer openDevice.Device.DestroyFence(fence)

	acquired, err := surface.AcquireTexture(fence)
	if err != nil {
		t.Fatalf("AcquireTexture failed: %v", err)
	}
	if acquired == nil {
		t.Fatal("expected non-nil acquired texture")
		return
	}
	if acquired.Texture == nil {
		t.Fatal("expected non-nil texture")
	}
	if acquired.Suboptimal {
		t.Error("expected suboptimal to be false")
	}

	// Discard the texture
	surface.DiscardTexture(acquired.Texture)
}

// TestNoopFenceValue tests fence value operations.
func TestNoopFenceValue(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	// Cast to noop fence to access GetValue/Signal
	noopFence, ok := fence.(*noop.Fence)
	if !ok {
		t.Fatal("expected *noop.Fence")
	}

	// Initial value should be 0
	if val := noopFence.GetValue(); val != 0 {
		t.Errorf("expected initial value 0, got %d", val)
	}

	// Signal with value
	noopFence.Signal(42)
	if val := noopFence.GetValue(); val != 42 {
		t.Errorf("expected value 42 after signal, got %d", val)
	}

	// Wait should succeed for value <= current
	if !noopFence.Wait(42, time.Second) {
		t.Error("expected Wait(42) to succeed")
	}
	if !noopFence.Wait(10, time.Second) {
		t.Error("expected Wait(10) to succeed")
	}

	// Wait should fail for value > current
	if noopFence.Wait(100, time.Millisecond) {
		t.Error("expected Wait(100) to fail")
	}
}

// TestNoopFenceWait tests fence waiting.
func TestNoopFenceWait(t *testing.T) {
	device, cleanup := createTestDevice(t)
	defer cleanup()

	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	// Wait with timeout (should return immediately)
	ok, err := device.Wait(fence, 0, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !ok {
		t.Error("expected Wait to succeed for value 0")
	}

	// Wait for future value (should fail)
	ok, err = device.Wait(fence, 100, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if ok {
		t.Error("expected Wait to timeout for future value")
	}
}

// TestNoopFullLifecycle tests complete workflow from instance to rendering.
func TestNoopFullLifecycle(t *testing.T) {
	// Create instance
	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	// Create surface
	surface, err := instance.CreateSurface(0, 0)
	if err != nil {
		t.Fatalf("CreateSurface failed: %v", err)
	}
	defer surface.Destroy()

	// Enumerate adapters
	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Fatal("no adapters found")
	}

	// Open device
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer openDevice.Device.Destroy()

	device := openDevice.Device
	queue := openDevice.Queue

	// Configure surface
	config := &hal.SurfaceConfiguration{
		Width:  800,
		Height: 600,
		Format: gputypes.TextureFormatBGRA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	}
	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	defer surface.Unconfigure(device)

	// Create resources
	buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{
		Size:  256,
		Usage: gputypes.BufferUsageVertex,
	})
	defer device.DestroyBuffer(buffer)

	texture, _ := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	defer device.DestroyTexture(texture)

	view, _ := device.CreateTextureView(texture, &hal.TextureViewDescriptor{})
	defer device.DestroyTextureView(view)

	// Create pipeline
	layout, _ := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{})
	defer device.DestroyPipelineLayout(layout)

	module, _ := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Source: hal.ShaderSource{WGSL: "@vertex fn vs() -> @builtin(position) vec4<f32> { return vec4<f32>(); }"},
	})
	defer device.DestroyShaderModule(module)

	pipeline, _ := device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Layout: layout,
		Vertex: hal.VertexState{
			Module:     module,
			EntryPoint: "vs",
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	defer device.DestroyRenderPipeline(pipeline)

	// Record commands
	encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = encoder.BeginEncoding("frame")

	renderPass := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0.0, G: 0.0, B: 0.0, A: 1.0},
			},
		},
	})
	renderPass.SetPipeline(pipeline)
	renderPass.SetVertexBuffer(0, buffer, 0)
	renderPass.Draw(3, 1, 0, 0)
	renderPass.End()

	cmdBuffer, _ := encoder.EndEncoding()

	// Submit
	fence, _ := device.CreateFence()
	defer device.DestroyFence(fence)

	err = queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait
	ok, err := device.Wait(fence, 1, time.Second)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !ok {
		t.Error("expected fence to be signaled")
	}
}

// TestNoopConcurrentAccess tests concurrent access to noop backend.
func TestNoopConcurrentAccess(t *testing.T) {
	device, queue, cleanup := createTestDeviceAndQueue(t)
	defer cleanup()

	var wg sync.WaitGroup
	numGoroutines := 10
	numOpsPerGoroutine := 100

	// Test concurrent buffer creation
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				buffer, _ := device.CreateBuffer(&hal.BufferDescriptor{Size: 256})
				device.DestroyBuffer(buffer)
			}
		}()
	}
	wg.Wait()

	// Test concurrent submissions
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				encoder, _ := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
				_ = encoder.BeginEncoding("test")
				cmdBuffer, _ := encoder.EndEncoding()
				_ = queue.Submit([]hal.CommandBuffer{cmdBuffer}, nil, 0)
			}
		}()
	}
	wg.Wait()
}

// Helper functions

func createTestDevice(t *testing.T) (hal.Device, func()) {
	t.Helper()

	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	adapters := instance.EnumerateAdapters(nil)
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		instance.Destroy()
		t.Fatalf("Open failed: %v", err)
	}

	cleanup := func() {
		openDevice.Device.Destroy()
		instance.Destroy()
	}

	return openDevice.Device, cleanup
}

func createTestDeviceAndQueue(t *testing.T) (hal.Device, hal.Queue, func()) {
	t.Helper()

	api := noop.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	adapters := instance.EnumerateAdapters(nil)
	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		instance.Destroy()
		t.Fatalf("Open failed: %v", err)
	}

	cleanup := func() {
		openDevice.Device.Destroy()
		instance.Destroy()
	}

	return openDevice.Device, openDevice.Queue, cleanup
}
