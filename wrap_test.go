//go:build !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

// =============================================================================
// wrap.go coverage — NewDeviceFromHAL, NewSurfaceFromHAL, etc.
// Covers wrap.go 0% coverage (37 missed lines)
// =============================================================================

func TestNewDeviceFromHALNilDeviceError(t *testing.T) {
	_, err := wgpu.NewDeviceFromHAL(nil, nil, 0, wgpu.DefaultLimits(), "nil-device")
	if err == nil {
		t.Fatal("NewDeviceFromHAL(nil device, nil queue) should fail")
	}
}

func TestNewSurfaceFromHAL(t *testing.T) {
	surface := wgpu.NewSurfaceFromHAL(nil, "test-surface")
	if surface == nil {
		t.Fatal("NewSurfaceFromHAL returned nil")
	}
}

func TestNewTextureFromHAL(t *testing.T) {
	tex := wgpu.NewTextureFromHAL(nil, nil, wgpu.TextureFormatRGBA8Unorm)
	if tex == nil {
		t.Fatal("NewTextureFromHAL returned nil")
	}
}

func TestNewTextureViewFromHAL(t *testing.T) {
	view := wgpu.NewTextureViewFromHAL(nil, nil)
	if view == nil {
		t.Fatal("NewTextureViewFromHAL returned nil")
	}
}

func TestNewSamplerFromHAL(t *testing.T) {
	sampler := wgpu.NewSamplerFromHAL(nil, nil)
	if sampler == nil {
		t.Fatal("NewSamplerFromHAL returned nil")
	}
}

func TestDeviceHalDeviceAndQueue(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// HalDevice may return nil or non-nil depending on backend.
	// The test ensures no panic.
	_ = device.HalDevice()
	_ = device.HalQueue()
}

func TestDeviceHalDeviceReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	// HalDevice on released device should return nil without panic.
	hal := device.HalDevice()
	if hal != nil {
		t.Error("HalDevice on released device should return nil")
	}
}

func TestDeviceHalQueueReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	// HalQueue on released device — queue may be freed or nil.
	// Should not panic.
	_ = device.HalQueue()
}
