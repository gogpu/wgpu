//go:build !(js && wasm)

package core

import (
	"errors"
	"image"
	"strings"
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

// --- Damage-aware present tests (ADR-017 Phase 1) ---

func TestPresent_NilDamage_IdenticalToLegacy(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Acquire
	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Present with nil damage — must behave identically to legacy Present.
	if err := surface.PresentWithDamage(queue, nil); err != nil {
		t.Fatalf("PresentWithDamage(nil): %v", err)
	}
	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after PresentWithDamage(nil) = %d, want SurfaceStateConfigured",
			surface.State())
	}
}

func TestPresent_WithDamageRects(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Present with damage rects — noop backend accepts and ignores them.
	rects := []image.Rectangle{
		image.Rect(10, 20, 100, 80),
		image.Rect(200, 300, 400, 500),
	}
	if err := surface.PresentWithDamage(queue, rects); err != nil {
		t.Fatalf("PresentWithDamage(rects): %v", err)
	}
	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after PresentWithDamage(rects) = %d, want SurfaceStateConfigured",
			surface.State())
	}
}

func TestPresent_EmptySlice_SameAsNil(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Present with empty slice — must behave identically to nil.
	if err := surface.PresentWithDamage(queue, []image.Rectangle{}); err != nil {
		t.Fatalf("PresentWithDamage(empty): %v", err)
	}
	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after PresentWithDamage(empty) = %d, want SurfaceStateConfigured",
			surface.State())
	}
}

func TestPresent_LegacyCallsNewPath(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// Legacy Present() must work unchanged — it internally calls PresentWithDamage(nil).
	if err := surface.Present(queue); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after Present = %d, want SurfaceStateConfigured",
			surface.State())
	}
}

func TestPresentWithDamage_WithoutAcquire(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// PresentWithDamage without acquire should fail with same error as Present.
	err := surface.PresentWithDamage(queue, nil)
	if !errors.Is(err, ErrSurfaceNoTextureAcquired) {
		t.Errorf("PresentWithDamage without acquire = %v, want ErrSurfaceNoTextureAcquired", err)
	}
}

// --- Extended damage-aware present tests ---

func TestPresent_MixedUsage_PresentThenPresentWithDamage(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Frame 1: legacy Present.
	if _, err := surface.AcquireTexture(nil); err != nil {
		t.Fatalf("AcquireTexture (frame 1): %v", err)
	}
	if err := surface.Present(queue); err != nil {
		t.Fatalf("Present (frame 1): %v", err)
	}

	// Frame 2: PresentWithDamage.
	if _, err := surface.AcquireTexture(nil); err != nil {
		t.Fatalf("AcquireTexture (frame 2): %v", err)
	}
	rects := []image.Rectangle{image.Rect(10, 10, 100, 100)}
	if err := surface.PresentWithDamage(queue, rects); err != nil {
		t.Fatalf("PresentWithDamage (frame 2): %v", err)
	}

	// Frame 3: back to legacy Present.
	if _, err := surface.AcquireTexture(nil); err != nil {
		t.Fatalf("AcquireTexture (frame 3): %v", err)
	}
	if err := surface.Present(queue); err != nil {
		t.Fatalf("Present (frame 3): %v", err)
	}

	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after mixed presents = %d, want SurfaceStateConfigured",
			surface.State())
	}
}

func TestPresent_DamageRectsVariousPatterns(t *testing.T) {
	// Table-driven test verifying PresentWithDamage accepts various rect patterns.
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	tests := []struct {
		name  string
		rects []image.Rectangle
	}{
		{"nil", nil},
		{"empty_slice", []image.Rectangle{}},
		{"single_small", []image.Rectangle{image.Rect(0, 0, 10, 10)}},
		{"full_surface", []image.Rectangle{image.Rect(0, 0, 800, 600)}},
		{"multiple", []image.Rectangle{
			image.Rect(0, 0, 100, 100),
			image.Rect(200, 200, 400, 400),
			image.Rect(500, 300, 700, 500),
		}},
		{"overlapping", []image.Rectangle{
			image.Rect(10, 10, 100, 100),
			image.Rect(50, 50, 150, 150),
		}},
		{"out_of_bounds", []image.Rectangle{
			image.Rect(-10, -10, 810, 610), // extends past all edges
		}},
		{"zero_size", []image.Rectangle{
			image.Rect(50, 50, 50, 50), // empty rect
		}},
		{"inverted_rect", []image.Rectangle{
			image.Rect(100, 100, 50, 50), // Min > Max
		}},
		{"negative_origin", []image.Rectangle{
			image.Rect(-100, -100, 50, 50),
		}},
		{"single_pixel", []image.Rectangle{
			image.Rect(400, 300, 401, 301),
		}},
		{"many_small_rects", func() []image.Rectangle {
			rects := make([]image.Rectangle, 20)
			for i := range rects {
				x := i * 40
				rects[i] = image.Rect(x, 0, x+30, 30)
			}
			return rects
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := surface.AcquireTexture(nil); err != nil {
				t.Fatalf("AcquireTexture: %v", err)
			}
			if err := surface.PresentWithDamage(queue, tt.rects); err != nil {
				t.Fatalf("PresentWithDamage(%s): %v", tt.name, err)
			}
			if surface.State() != SurfaceStateConfigured {
				t.Errorf("state after %s = %d, want SurfaceStateConfigured",
					tt.name, surface.State())
			}
		})
	}
}

func TestPresent_DamageAfterReconfigure(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Present once.
	if _, err := surface.AcquireTexture(nil); err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}
	if err := surface.Present(queue); err != nil {
		t.Fatalf("Present: %v", err)
	}

	// Reconfigure to different dimensions.
	newConfig := &hal.SurfaceConfiguration{
		Width:       1024,
		Height:      768,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}
	if err := surface.Configure(device, newConfig); err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}

	// PresentWithDamage should work with new dimensions.
	if _, err := surface.AcquireTexture(nil); err != nil {
		t.Fatalf("AcquireTexture after reconfigure: %v", err)
	}
	rects := []image.Rectangle{image.Rect(100, 100, 500, 500)}
	if err := surface.PresentWithDamage(queue, rects); err != nil {
		t.Fatalf("PresentWithDamage after reconfigure: %v", err)
	}

	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state = %d, want SurfaceStateConfigured", surface.State())
	}
}

func TestPresent_DamageWithNilQueue(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, err := surface.AcquireTexture(nil); err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}

	// PresentWithDamage with nil queue should panic (nil pointer dereference)
	// or be handled gracefully. This mirrors the behavior of Present with nil queue.
	// We verify the contract is the same for both methods.
	panicked := false
	func() {
		defer func() {
			if v := recover(); v != nil {
				panicked = true
			}
		}()
		_ = surface.PresentWithDamage(nil, nil)
	}()

	if !panicked {
		// If it didn't panic, that's also acceptable — it means the implementation
		// handles nil queue gracefully. Either way, the test documents the behavior.
		t.Log("PresentWithDamage(nil queue) did not panic — nil queue handled gracefully")
	}
}

// --- PresentPixels tests ---

func TestPresentPixels_Unconfigured(t *testing.T) {
	surface, _, _ := newTestSurface(t)

	err := surface.PresentPixels([]byte{0, 0, 0, 0}, 1, 1, nil)
	if !errors.Is(err, ErrSurfaceNotConfigured) {
		t.Errorf("PresentPixels unconfigured = %v, want ErrSurfaceNotConfigured", err)
	}
}

func TestPresentPixels_UnsupportedBackend(t *testing.T) {
	// noop backend does not implement PresentPixels — should return error.
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	err := surface.PresentPixels([]byte{0, 0, 0, 0}, 1, 1, nil)
	if err == nil {
		t.Error("PresentPixels on noop backend should return error")
	}
	// Should mention "not supported"
	if err != nil && !strings.Contains(err.Error(), "not supported") {
		t.Errorf("PresentPixels error = %q, want to contain 'not supported'", err.Error())
	}
}

func TestPresentPixels_DiscardsAcquiredTexture(t *testing.T) {
	// After AcquireTexture, PresentPixels should discard the stale texture
	// and succeed (on a backend that supports it). Since noop doesn't support
	// PresentPixels, we verify the state transition: acquired → configured,
	// then the "not supported" error comes from the duck-type check.
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Acquire a texture
	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}
	if surface.State() != SurfaceStateAcquired {
		t.Fatalf("state = %d, want SurfaceStateAcquired", surface.State())
	}

	// PresentPixels discards the acquired texture, then fails because noop
	// doesn't implement the interface.
	err = surface.PresentPixels([]byte{0, 0, 0, 0}, 1, 1, nil)
	if err == nil {
		t.Error("PresentPixels on noop should return error")
	}

	// State should be Configured (texture was discarded before the duck-type check).
	if surface.State() != SurfaceStateConfigured {
		t.Errorf("state after PresentPixels = %d, want SurfaceStateConfigured", surface.State())
	}

	// Should be able to acquire again (proves discard happened).
	_, err = surface.AcquireTexture(nil)
	if err != nil {
		t.Errorf("AcquireTexture after PresentPixels: %v", err)
	}
}
