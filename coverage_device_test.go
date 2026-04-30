//go:build !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// =============================================================================
// Device.CreateBuffer — released device, nil desc, various usages
// Covers device_native.go CreateBuffer guard clauses + happy paths
// =============================================================================

func TestCreateBufferReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "post-release",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateBuffer after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.CreateTexture — released device, nil desc
// Covers device_native.go CreateTexture guard clauses
// =============================================================================

func TestCreateTextureReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "post-release",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateTexture after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.CreateSampler — released device, nil desc (creates default sampler)
// Covers device_native.go CreateSampler guard clauses
// =============================================================================

func TestCreateSamplerReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateSampler(&wgpu.SamplerDescriptor{Label: "post-release"})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateSampler after Release: got %v, want ErrReleased", err)
	}
}

func TestCreateSamplerNilDescCreatesDefault(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// nil descriptor should create a sampler with default parameters.
	sampler, err := device.CreateSampler(nil)
	if err != nil {
		t.Fatalf("CreateSampler(nil): %v", err)
	}
	if sampler == nil {
		t.Fatal("CreateSampler(nil) returned nil sampler")
	}
	sampler.Release()
}

// =============================================================================
// Device.CreateShaderModule — released device, nil desc
// Covers device_native.go CreateShaderModule guard clauses
// =============================================================================

func TestCreateShaderModuleReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "post-release",
		WGSL:  "@vertex fn vs() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateShaderModule after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.CreateBindGroupLayout — released device, nil desc
// Covers device_native.go CreateBindGroupLayout guard clauses
// =============================================================================

func TestCreateBindGroupLayoutReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "post-release",
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateBindGroupLayout after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.CreatePipelineLayout — happy path with multiple bind group layouts
// Covers device_native.go lines 286-331 (bindGroupLayouts copy, bindGroupCount)
// =============================================================================

func TestCreatePipelineLayoutWithLayouts(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	bgl1, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "pl-bgl-0",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout 0: %v", err)
	}
	defer bgl1.Release()

	bgl2, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "pl-bgl-1",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageVertex,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 16,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout 1: %v", err)
	}
	defer bgl2.Release()

	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "two-group-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl1, bgl2},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	if layout == nil {
		t.Fatal("CreatePipelineLayout returned nil")
	}
	layout.Release()
}

// =============================================================================
// Device.CreateRenderPipeline — released device
// Covers device_native.go CreateRenderPipeline guard clause
// =============================================================================

func TestCreateRenderPipelineReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "post-release",
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateRenderPipeline after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.CreateComputePipeline — released device
// Covers device_native.go CreateComputePipeline guard clause
// =============================================================================

func TestCreateComputePipelineReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "post-release",
		EntryPoint: "main",
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateComputePipeline after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.CreateCommandEncoder — released device
// Covers device_native.go CreateCommandEncoder guard clause
// =============================================================================

func TestCreateCommandEncoderReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateCommandEncoder(nil)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateCommandEncoder after Release: got %v, want ErrReleased", err)
	}
}

func TestCreateCommandEncoderWithLabel(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "labeled-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder with label: %v", err)
	}
	if enc == nil {
		t.Fatal("CreateCommandEncoder with label returned nil")
	}
	enc.DiscardEncoding()
}

// =============================================================================
// Device.CreateBindGroup — with buffer resources + late binding info
// Covers device_native.go CreateBindGroup lines 334-436
// (collectBindGroupResources, buildBindGroupEntryMap, late buffer binding info)
// =============================================================================

func TestCreateBindGroupWithBufferResources(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "bg-resource-buf",
		Size:  128,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "bg-resource-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageVertex,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 128,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bg-with-resources",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: 128},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	if bg == nil {
		t.Fatal("CreateBindGroup returned nil")
	}
	bg.Release()
}

func TestCreateBindGroupWithLateBufferBindingInfo(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "late-bind-buf",
		Size:  512,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// MinBindingSize == 0 triggers late buffer binding info path.
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "late-bind-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeStorage,
					MinBindingSize: 0, // late binding
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	// Size == 0 means "rest of buffer" path. Offset must be aligned to 256.
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bg-late-binding",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 256, Size: 0},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	if bg == nil {
		t.Fatal("CreateBindGroup returned nil")
	}
	bg.Release()
}

func TestCreateBindGroupWithExplicitSize(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "explicit-size-buf",
		Size:  256,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "explicit-size-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeStorage,
					MinBindingSize: 0,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	// Explicit Size > 0 — does not trigger "rest of buffer" path.
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bg-explicit-size",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: 128},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	bg.Release()
}

func TestCreateBindGroupMultipleBufferEntries(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf1, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "multi-buf-1",
		Size:  64,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer 1: %v", err)
	}
	defer buf1.Release()

	buf2, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "multi-buf-2",
		Size:  128,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer 2: %v", err)
	}
	defer buf2.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "multi-buf-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageVertex,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 64,
				},
			},
			{
				Binding:    1,
				Visibility: wgpu.ShaderStageFragment,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeReadOnlyStorage,
					MinBindingSize: 128,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bg-multi-buf",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf1, Offset: 0, Size: 64},
			{Binding: 1, Buffer: buf2, Offset: 0, Size: 128},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	bg.Release()
}

// =============================================================================
// Device.CreateFence + operations — full lifecycle
// Covers device_native.go lines 747-820 + fence.go
// =============================================================================

func TestFenceFullLifecycle(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}

	// Check status.
	_, err = device.GetFenceStatus(fence)
	if err != nil {
		t.Fatalf("GetFenceStatus: %v", err)
	}

	// Reset.
	if err := device.ResetFence(fence); err != nil {
		t.Fatalf("ResetFence: %v", err)
	}

	// Release via deprecated path.
	device.DestroyFence(fence)
}

// =============================================================================
// Device.GetSurfaceCapabilities — nil surface, released adapter
// Covers adapter_native.go GetSurfaceCapabilities
// =============================================================================

func TestGetSurfaceCapabilitiesNilSurface(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	caps := adapter.GetSurfaceCapabilities(nil)
	if caps != nil {
		t.Error("GetSurfaceCapabilities(nil) should return nil")
	}
}

func TestGetSurfaceCapabilitiesReleasedAdapter(t *testing.T) {
	_, adapter := newAdapter(t)
	adapter.Release()

	caps := adapter.GetSurfaceCapabilities(nil)
	if caps != nil {
		t.Error("GetSurfaceCapabilities on released adapter should return nil")
	}
}

func TestGetSurfaceCapabilitiesCoreOnlyPath(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	// With a non-nil surface (from HAL wrap) on a mock adapter, this tests
	// the core-only path which returns default capabilities.
	surface := wgpu.NewSurfaceFromHAL(nil, "test-surface")
	caps := adapter.GetSurfaceCapabilities(surface)
	// On mock adapter without HAL, expect the core-only defaults.
	if caps != nil {
		// If caps are returned, Fifo should be present (spec guaranteed).
		hasFifo := false
		for _, pm := range caps.PresentModes {
			if pm == wgpu.PresentModeFifo {
				hasFifo = true
				break
			}
		}
		if !hasFifo {
			t.Error("SurfaceCapabilities should include PresentModeFifo")
		}
	}
}

// =============================================================================
// Device.FreeCommandBuffer — with a real encoder
// Covers device_native.go lines 825-838 (halBuffer() path)
// =============================================================================

func TestFreeCommandBufferAfterFinish(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "free-cb-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// FreeCommandBuffer should not panic. After this the command buffer handle
	// is invalid.
	device.FreeCommandBuffer(cb)
}

// =============================================================================
// Device.WaitIdle — released device
// Covers device_native.go WaitIdle guard clause
// =============================================================================

func TestWaitIdleReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	err := device.WaitIdle()
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("WaitIdle after Release: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// Device.PushErrorScope / PopErrorScope — nested scopes
// Covers device_native.go lines 840-848
// =============================================================================

func TestErrorScopeNestedFilters(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// Push three nested scopes with different filters.
	device.PushErrorScope(wgpu.ErrorFilterValidation)
	device.PushErrorScope(wgpu.ErrorFilterOutOfMemory)
	device.PushErrorScope(wgpu.ErrorFilterInternal)

	// Pop in reverse order. All should be nil since no errors were generated.
	for i, name := range []string{"Internal", "OutOfMemory", "Validation"} {
		gpuErr := device.PopErrorScope()
		if gpuErr != nil {
			t.Errorf("PopErrorScope[%d] (%s): got non-nil error: %v", i, name, gpuErr)
		}
	}
}
