//go:build !rust && !(js && wasm)

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

func TestDeviceHalDeviceOnLive(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	_ = device.HalDevice()
}

func TestDeviceHalDeviceReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	hal := device.HalDevice()
	if hal != nil {
		t.Error("HalDevice on released device should return nil")
	}
}
