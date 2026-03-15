package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// newTestSurface creates a test Surface with a noop HAL backend.
// Returns the core Surface, a core Device (with HAL), and a noop Queue.
func newTestSurface(t *testing.T) (*Surface, *Device, hal.Queue) {
	t.Helper()

	api := noop.API{}
	inst, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	halSurface, err := inst.CreateSurface(0, 0)
	if err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}

	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("no adapters returned by noop backend")
	}

	openDev, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Adapter.Open: %v", err)
	}

	device := NewDevice(
		openDev.Device,
		nil, // adapter not needed for surface tests
		0,
		gputypes.DefaultLimits(),
		"test-device",
	)

	coreSurface := NewSurface(halSurface, "test-surface")
	return coreSurface, device, openDev.Queue
}

// testSurfaceConfig returns a default SurfaceConfiguration for testing.
func testSurfaceConfig() *hal.SurfaceConfiguration {
	return &hal.SurfaceConfiguration{
		Width:       800,
		Height:      600,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}
}

func TestSurfaceNewUnconfigured(t *testing.T) {
	surface, _, _ := newTestSurface(t)

	if surface.State() != SurfaceStateUnconfigured {
		t.Errorf("new surface state = %d, want SurfaceStateUnconfigured (%d)",
			surface.State(), SurfaceStateUnconfigured)
	}
	if surface.Config() != nil {
		t.Error("new surface config should be nil")
	}
}

func TestSurfaceConfigure(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	err := surface.Configure(device, config)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after Configure = %d, want SurfaceStateConfigured (%d)",
			surface.State(), SurfaceStateConfigured)
	}
	if surface.Config() == nil {
		t.Error("config should not be nil after Configure")
	}
	if surface.Config().Width != 800 || surface.Config().Height != 600 {
		t.Errorf("config dimensions = %dx%d, want 800x600",
			surface.Config().Width, surface.Config().Height)
	}
}

func TestSurfaceConfigureNilDevice(t *testing.T) {
	surface, _, _ := newTestSurface(t)
	config := testSurfaceConfig()

	err := surface.Configure(nil, config)
	if !errors.Is(err, ErrSurfaceNilDevice) {
		t.Errorf("Configure(nil device) = %v, want ErrSurfaceNilDevice", err)
	}
}

func TestSurfaceConfigureNilConfig(t *testing.T) {
	surface, device, _ := newTestSurface(t)

	err := surface.Configure(device, nil)
	if !errors.Is(err, ErrSurfaceNilConfig) {
		t.Errorf("Configure(nil config) = %v, want ErrSurfaceNilConfig", err)
	}
}

func TestSurfaceAcquirePresent(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Acquire
	result, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}
	if result == nil || result.Texture == nil {
		t.Fatal("AcquireTexture returned nil result or texture")
	}
	if surface.State() != SurfaceStateAcquired {
		t.Errorf("state after Acquire = %d, want SurfaceStateAcquired (%d)",
			surface.State(), SurfaceStateAcquired)
	}

	// Present
	if err := surface.Present(queue); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after Present = %d, want SurfaceStateConfigured (%d)",
			surface.State(), SurfaceStateConfigured)
	}
}

func TestSurfaceDoubleAcquire(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// First acquire succeeds
	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("first AcquireTexture: %v", err)
	}

	// Second acquire fails
	_, err = surface.AcquireTexture(nil)
	if !errors.Is(err, ErrSurfaceAlreadyAcquired) {
		t.Errorf("second AcquireTexture = %v, want ErrSurfaceAlreadyAcquired", err)
	}
}

func TestSurfacePresentWithoutAcquire(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	err := surface.Present(queue)
	if !errors.Is(err, ErrSurfaceNoTextureAcquired) {
		t.Errorf("Present without acquire = %v, want ErrSurfaceNoTextureAcquired", err)
	}
}

func TestSurfaceAcquireWithoutConfigure(t *testing.T) {
	surface, _, _ := newTestSurface(t)

	_, err := surface.AcquireTexture(nil)
	if !errors.Is(err, ErrSurfaceNotConfigured) {
		t.Errorf("AcquireTexture unconfigured = %v, want ErrSurfaceNotConfigured", err)
	}
}

func TestSurfaceUnconfigureWhileAcquired(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Unconfigure while acquired — should discard and return to unconfigured
	surface.Unconfigure()

	if surface.State() != SurfaceStateUnconfigured {
		t.Errorf("state after Unconfigure = %d, want SurfaceStateUnconfigured (%d)",
			surface.State(), SurfaceStateUnconfigured)
	}
	if surface.Config() != nil {
		t.Error("config should be nil after Unconfigure")
	}
}

func TestSurfaceReconfigure(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	// First configure
	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("first Configure: %v", err)
	}

	// Reconfigure with different dimensions
	config2 := &hal.SurfaceConfiguration{
		Width:       1024,
		Height:      768,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}
	if err := surface.Configure(device, config2); err != nil {
		t.Fatalf("second Configure: %v", err)
	}

	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after reconfigure = %d, want SurfaceStateConfigured", surface.State())
	}
	if surface.Config().Width != 1024 || surface.Config().Height != 768 {
		t.Errorf("config dimensions = %dx%d, want 1024x768",
			surface.Config().Width, surface.Config().Height)
	}
}

func TestSurfaceConfigureWhileAcquired(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Configure while acquired should fail
	err = surface.Configure(device, config)
	if !errors.Is(err, ErrSurfaceConfigureWhileAcquired) {
		t.Errorf("Configure while acquired = %v, want ErrSurfaceConfigureWhileAcquired", err)
	}
}

func TestSurfacePrepareFrame(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	called := false
	surface.SetPrepareFrame(func() (uint32, uint32, bool) {
		called = true
		return 800, 600, false // no change
	})

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	if !called {
		t.Error("PrepareFrame hook was not called")
	}
}

func TestSurfacePrepareFrameReconfigure(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// PrepareFrame reports new dimensions
	surface.SetPrepareFrame(func() (uint32, uint32, bool) {
		return 1920, 1080, true // changed
	})

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Config should have been updated
	if surface.Config().Width != 1920 || surface.Config().Height != 1080 {
		t.Errorf("config after PrepareFrame = %dx%d, want 1920x1080",
			surface.Config().Width, surface.Config().Height)
	}
}

func TestSurfaceDiscardTexture(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	surface.DiscardTexture()

	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after DiscardTexture = %d, want SurfaceStateConfigured", surface.State())
	}

	// Should be able to acquire again after discard
	_, err = surface.AcquireTexture(nil)
	if err != nil {
		t.Errorf("AcquireTexture after discard: %v", err)
	}
}

func TestSurfaceDiscardWithoutAcquire(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// DiscardTexture when not acquired should be a no-op
	surface.DiscardTexture()

	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after no-op DiscardTexture = %d, want SurfaceStateConfigured", surface.State())
	}
}

func TestSurfaceUnconfigureWhenUnconfigured(t *testing.T) {
	surface, _, _ := newTestSurface(t)

	// Unconfigure when already unconfigured should be a no-op
	surface.Unconfigure()

	if surface.State() != SurfaceStateUnconfigured {
		t.Errorf("state after no-op Unconfigure = %d, want SurfaceStateUnconfigured", surface.State())
	}
}
