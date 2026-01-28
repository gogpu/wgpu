package hal_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/hal"
	_ "github.com/gogpu/wgpu/hal/noop" // Import for side effect of registering noop backend
	"github.com/gogpu/gputypes"
)

// TestErrZeroArea verifies that ErrZeroArea is defined correctly.
func TestErrZeroArea(t *testing.T) {
	// ErrZeroArea should be a defined error
	if hal.ErrZeroArea == nil {
		t.Fatal("ErrZeroArea should not be nil")
	}

	// Error message should be descriptive
	msg := hal.ErrZeroArea.Error()
	if msg == "" {
		t.Error("ErrZeroArea should have a non-empty message")
	}

	// Should contain relevant keywords
	if !containsAny(msg, "zero", "width", "height", "non-zero") {
		t.Errorf("ErrZeroArea message should mention dimensions: %s", msg)
	}
}

// TestErrZeroArea_IsComparable verifies that ErrZeroArea can be compared with errors.Is.
func TestErrZeroArea_IsComparable(t *testing.T) {
	// Wrap the error
	wrapped := &wrappedError{err: hal.ErrZeroArea}

	// errors.Is should find the underlying error
	if !errors.Is(wrapped, hal.ErrZeroArea) {
		t.Error("errors.Is should find ErrZeroArea in wrapped error")
	}
}

// TestSurfaceConfigureZeroDimensions_Vulkan tests that Vulkan Surface.Configure
// returns ErrZeroArea when dimensions are zero.
func TestSurfaceConfigureZeroDimensions_Vulkan(t *testing.T) {
	// Skip if Vulkan backend is not available
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		t.Skip("Vulkan backend not available")
	}

	instance, err := backend.CreateInstance(nil)
	if err != nil {
		t.Skipf("Vulkan instance creation failed: %v", err)
	}
	defer instance.Destroy()

	surface, err := instance.CreateSurface(0, 0)
	if err != nil {
		t.Skipf("Surface creation failed: %v", err)
	}
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Skip("No Vulkan adapters available")
	}

	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Skipf("Device creation failed: %v", err)
	}
	defer openDevice.Device.Destroy()

	// Test with zero width
	config := &hal.SurfaceConfiguration{
		Width:       0,
		Height:      600,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeFifo,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}

	err = surface.Configure(openDevice.Device, config)
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Errorf("Configure with width=0 should return ErrZeroArea, got: %v", err)
	}

	// Test with zero height
	config.Width = 800
	config.Height = 0

	err = surface.Configure(openDevice.Device, config)
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Errorf("Configure with height=0 should return ErrZeroArea, got: %v", err)
	}

	// Test with both zero
	config.Width = 0
	config.Height = 0

	err = surface.Configure(openDevice.Device, config)
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Errorf("Configure with width=0, height=0 should return ErrZeroArea, got: %v", err)
	}
}

// TestSurfaceConfigureValidDimensions verifies that valid dimensions work.
func TestSurfaceConfigureValidDimensions(t *testing.T) {
	// Use noop backend which should accept any dimensions
	backend, ok := hal.GetBackend(gputypes.BackendEmpty)
	if !ok {
		t.Fatal("noop backend should be available")
	}

	instance, err := backend.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	defer instance.Destroy()

	surface, err := instance.CreateSurface(0, 0)
	if err != nil {
		t.Fatalf("CreateSurface failed: %v", err)
	}
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Fatal("expected at least one adapter")
	}

	openDevice, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer openDevice.Device.Destroy()

	// Valid dimensions should succeed
	config := &hal.SurfaceConfiguration{
		Width:       800,
		Height:      600,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeFifo,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}

	err = surface.Configure(openDevice.Device, config)
	if err != nil {
		t.Errorf("Configure with valid dimensions should succeed, got: %v", err)
	}
}

// wrappedError is a helper for testing error wrapping.
type wrappedError struct {
	err error
}

func (w *wrappedError) Error() string {
	return "wrapped: " + w.err.Error()
}

func (w *wrappedError) Unwrap() error {
	return w.err
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			substr == "" ||
			findSubstring(s, substr) >= 0)
}

// findSubstring finds substr in s (case-insensitive).
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// toLower converts ASCII uppercase to lowercase.
func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}
