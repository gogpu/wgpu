//go:build !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

// =============================================================================
// NewDeviceFromHAL — nil device + nil queue
// Covers wrap.go lines 19-25 (nil guard clauses)
// =============================================================================

func TestNewDeviceFromHALBothNil(t *testing.T) {
	_, err := wgpu.NewDeviceFromHAL(nil, nil, 0, wgpu.DefaultLimits(), "both-nil")
	if err == nil {
		t.Fatal("NewDeviceFromHAL(nil, nil) should fail")
	}
}

// =============================================================================
// NewTextureFromHAL — format preservation for multiple formats
// Covers wrap.go lines 78-80
// =============================================================================

func TestNewTextureFromHALFormats(t *testing.T) {
	formats := []struct {
		name   string
		format wgpu.TextureFormat
	}{
		{"RGBA8Unorm", wgpu.TextureFormatRGBA8Unorm},
		{"BGRA8Unorm", wgpu.TextureFormatBGRA8Unorm},
		{"Depth32Float", wgpu.TextureFormatDepth32Float},
	}

	for _, tt := range formats {
		t.Run(tt.name, func(t *testing.T) {
			tex := wgpu.NewTextureFromHAL(nil, nil, tt.format)
			if tex == nil {
				t.Fatal("NewTextureFromHAL returned nil")
			}
			if got := tex.Format(); got != tt.format {
				t.Errorf("Format() = %v, want %v", got, tt.format)
			}
		})
	}
}

// =============================================================================
// NewTextureViewFromHAL + NewSamplerFromHAL — nil args
// Covers wrap.go lines 83-89
// =============================================================================

func TestNewTextureViewFromHALNilArgs(t *testing.T) {
	view := wgpu.NewTextureViewFromHAL(nil, nil)
	if view == nil {
		t.Fatal("NewTextureViewFromHAL returned nil")
	}
}

func TestNewSamplerFromHALNilArgs(t *testing.T) {
	sampler := wgpu.NewSamplerFromHAL(nil, nil)
	if sampler == nil {
		t.Fatal("NewSamplerFromHAL returned nil")
	}
}

// =============================================================================
// NewSurfaceFromHAL — with label
// Covers wrap.go lines 69-74
// =============================================================================

func TestNewSurfaceFromHALWithLabel(t *testing.T) {
	surface := wgpu.NewSurfaceFromHAL(nil, "labeled-surface")
	if surface == nil {
		t.Fatal("NewSurfaceFromHAL returned nil")
	}
}

// =============================================================================
// HalDevice / HalQueue — edge cases on live and released device
// Covers wrap.go lines 92-109
// =============================================================================

func TestHalDeviceOnLiveDevice(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// On a live device with HAL, HalDevice should return non-nil.
	hal := device.HalDevice()
	if hal == nil {
		t.Error("HalDevice() should return non-nil on live device")
	}
}

func TestHalQueueOnLiveDevice(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	q := device.HalQueue()
	if q == nil {
		t.Error("HalQueue() should return non-nil on live device")
	}
}

func TestHalDeviceOnReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	hal := device.HalDevice()
	if hal != nil {
		t.Error("HalDevice() on released device should return nil")
	}
}

func TestHalQueueOnReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	// Queue on released device — should not panic.
	_ = device.HalQueue()
}

// =============================================================================
// Device.CreateCommandEncoder — pool acquire/release integration
// Covers device_native.go lines 698-742
// =============================================================================

func TestCreateAndDiscardMultipleEncoders(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// Create, discard, repeat — exercises pool acquire/release cycle.
	for i := 0; i < 5; i++ {
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("iteration %d: CreateCommandEncoder: %v", i, err)
		}
		enc.DiscardEncoding()
	}
}

func TestCreateAndFinishMultipleEncoders(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	var cmdBufs []*wgpu.CommandBuffer

	// Create and finish several encoders — exercises pool pressure.
	for i := 0; i < 3; i++ {
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("iteration %d: CreateCommandEncoder: %v", i, err)
		}
		cb, err := enc.Finish()
		if err != nil {
			t.Fatalf("iteration %d: Finish: %v", i, err)
		}
		cmdBufs = append(cmdBufs, cb)
	}

	// Release all command buffers — encoders return to pool.
	for _, cb := range cmdBufs {
		cb.Release()
	}
}

// =============================================================================
// Device.Features and Device.Limits — stability
// Covers device_native.go lines 51-58
// =============================================================================

func TestDeviceFeaturesStable(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	f1 := device.Features()
	f2 := device.Features()
	if f1 != f2 {
		t.Errorf("Features() should be stable: %v vs %v", f1, f2)
	}
}

func TestDeviceLimitsStable(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	l1 := device.Limits()
	l2 := device.Limits()
	if l1.MaxBufferSize != l2.MaxBufferSize {
		t.Error("Limits() MaxBufferSize should be stable")
	}
	if l1.MaxBindGroups != l2.MaxBindGroups {
		t.Error("Limits() MaxBindGroups should be stable")
	}
}

// =============================================================================
// Released device — all Create* methods return ErrReleased (table-driven)
// Covers device_native.go guard clauses comprehensively
// =============================================================================

func TestAllCreateMethodsOnReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"CreateBuffer", func() error {
			_, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 64, Usage: wgpu.BufferUsageVertex})
			return err
		}},
		{"CreateTexture", func() error {
			_, err := device.CreateTexture(&wgpu.TextureDescriptor{
				Size: wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1,
				Format: wgpu.TextureFormatRGBA8Unorm, Usage: wgpu.TextureUsageTextureBinding,
			})
			return err
		}},
		{"CreateSampler", func() error {
			_, err := device.CreateSampler(nil)
			return err
		}},
		{"CreateShaderModule", func() error {
			_, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: "test"})
			return err
		}},
		{"CreateBindGroupLayout", func() error {
			_, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{})
			return err
		}},
		{"CreatePipelineLayout", func() error {
			_, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{})
			return err
		}},
		{"CreateBindGroup", func() error {
			_, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{})
			return err
		}},
		{"CreateRenderPipeline", func() error {
			_, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{})
			return err
		}},
		{"CreateComputePipeline", func() error {
			_, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{EntryPoint: "main"})
			return err
		}},
		{"CreateCommandEncoder", func() error {
			_, err := device.CreateCommandEncoder(nil)
			return err
		}},
		{"CreateFence", func() error {
			_, err := device.CreateFence()
			return err
		}},
		{"WaitIdle", func() error {
			return device.WaitIdle()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if !errors.Is(err, wgpu.ErrReleased) {
				t.Errorf("got %v, want ErrReleased", err)
			}
		})
	}
}

// =============================================================================
// Nil descriptor tests — table-driven
// Covers device_native.go nil-desc guard clauses
// =============================================================================

func TestAllCreateMethodsWithNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"CreateBuffer(nil)", func() error {
			_, err := device.CreateBuffer(nil)
			return err
		}},
		{"CreateTexture(nil)", func() error {
			_, err := device.CreateTexture(nil)
			return err
		}},
		{"CreateShaderModule(nil)", func() error {
			_, err := device.CreateShaderModule(nil)
			return err
		}},
		{"CreateBindGroupLayout(nil)", func() error {
			_, err := device.CreateBindGroupLayout(nil)
			return err
		}},
		{"CreatePipelineLayout(nil)", func() error {
			_, err := device.CreatePipelineLayout(nil)
			return err
		}},
		{"CreateBindGroup(nil)", func() error {
			_, err := device.CreateBindGroup(nil)
			return err
		}},
		{"CreateRenderPipeline(nil)", func() error {
			_, err := device.CreateRenderPipeline(nil)
			return err
		}},
		{"CreateComputePipeline(nil)", func() error {
			_, err := device.CreateComputePipeline(nil)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Error("expected error for nil descriptor")
			}
		})
	}
}
