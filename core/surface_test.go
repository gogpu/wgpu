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

func TestSurfaceAcquisitionLeaseExpiresAtLifecycleBoundaries(t *testing.T) {
	surface, device, queue := newTestSurface(t)
	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, first, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if !surface.AcquisitionValid(first) {
		t.Fatal("first lease was not valid after acquire")
	}
	if err := surface.Present(queue); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if surface.AcquisitionValid(first) {
		t.Fatal("presented lease remained valid")
	}

	_, second, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if second == first || !surface.AcquisitionValid(second) {
		t.Fatalf("second lease = %d, first = %d, valid = %v", second, first, surface.AcquisitionValid(second))
	}
	surface.DiscardTexture()
	if surface.AcquisitionValid(second) {
		t.Fatal("discarded lease remained valid")
	}

	_, third, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("third acquire: %v", err)
	}
	surface.Unconfigure()
	if surface.AcquisitionValid(third) {
		t.Fatal("unconfigured lease remained valid")
	}

	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("reconfigure: %v", err)
	}
	_, fourth, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("fourth acquire: %v", err)
	}
	surface.Destroy()
	if surface.AcquisitionValid(fourth) {
		t.Fatal("destroyed lease remained valid")
	}
}

func TestSurfaceAcquisitionLeaseWrapSkipsZero(t *testing.T) {
	surface, device, _ := newTestSurface(t)
	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	surface.nextAcquisition = ^uint64(0)
	_, lease, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("AcquireTextureWithLease: %v", err)
	}
	if lease != 1 {
		t.Fatalf("wrapped lease = %d, want 1", lease)
	}
	if !surface.AcquisitionValid(lease) {
		t.Fatal("wrapped non-zero lease was not valid")
	}
	if surface.AcquisitionValid(0) {
		t.Fatal("zero lease was accepted")
	}
	var nilSurface *Surface
	if nilSurface.AcquisitionValid(lease) {
		t.Fatal("nil surface accepted a lease")
	}
	surface.DiscardTexture()
}

func TestSurfaceRawReplacementRetiresPriorGeneration(t *testing.T) {
	var nilSurface *Surface
	if nilSurface.RawSurface() != nil {
		t.Fatal("nil surface exposed a raw surface")
	}

	surface, device, _ := newTestSurface(t)
	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	_, lease, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("AcquireTextureWithLease: %v", err)
	}
	replacement := &noop.Surface{}
	surface.SetRawSurface(replacement)

	if surface.RawSurface() != replacement {
		t.Fatal("RawSurface did not expose the replacement surface")
	}
	if surface.AcquisitionValid(lease) {
		t.Fatal("raw-surface replacement retained the prior acquisition")
	}
	if surface.State() != SurfaceStateUnconfigured || surface.Config() != nil {
		t.Fatalf("replacement state = (%v, %v), want unconfigured with nil config", surface.State(), surface.Config())
	}
}

func TestSurfaceRetireDeviceInvalidatesOnlyMatchingDevice(t *testing.T) {
	var nilSurface *Surface
	nilSurface.RetireDevice(nil)

	surface, device, _ := newTestSurface(t)
	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	_, lease, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("AcquireTextureWithLease: %v", err)
	}

	other := NewDevice(&noop.Device{}, nil, 0, gputypes.DefaultLimits(), "other-device")
	defer other.Destroy()
	surface.RetireDevice(nil)
	surface.RetireDevice(other)
	if !surface.AcquisitionValid(lease) {
		t.Fatal("unrelated device retirement invalidated the acquisition")
	}

	surface.RetireDevice(device)
	if surface.AcquisitionValid(lease) {
		t.Fatal("matching device retirement retained the acquisition")
	}
	if surface.State() != SurfaceStateUnconfigured || surface.Config() != nil {
		t.Fatalf("retired state = (%v, %v), want unconfigured with nil config", surface.State(), surface.Config())
	}
}

type surfaceDestroyObserver struct {
	noop.Surface
	discards     int
	unconfigures int
	destroys     int
}

func (s *surfaceDestroyObserver) DiscardTexture(hal.SurfaceTexture) { s.discards++ }
func (s *surfaceDestroyObserver) Unconfigure(hal.Device)            { s.unconfigures++ }
func (s *surfaceDestroyObserver) Destroy()                          { s.destroys++ }

func TestSurfaceDestroyRetiresAcquisitionBeforePlatformSurface(t *testing.T) {
	_, device, _ := newTestSurface(t)
	raw := &surfaceDestroyObserver{}
	surface := NewSurface(raw, "destroy-order-test")
	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, _, err := surface.AcquireTextureWithLease(nil); err != nil {
		t.Fatalf("AcquireTextureWithLease: %v", err)
	}

	surface.Destroy()
	surface.Destroy()
	if raw.discards != 1 || raw.unconfigures != 1 || raw.destroys != 1 {
		t.Fatalf("destroy calls = discard:%d unconfigure:%d destroy:%d; want 1,1,1",
			raw.discards, raw.unconfigures, raw.destroys)
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

func TestPresentPixels_PreservesAcquiredOnUnsupported(t *testing.T) {
	// On backends that don't support PixelPresenter (like noop), PresentPixels
	// should return an error WITHOUT discarding the acquired texture. This
	// preserves the surface state so the normal render→Present path still works.
	surface, device, _ := newTestSurface(t)
	config := testSurfaceConfig()

	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture: %v", err)
	}
	if surface.State() != SurfaceStateAcquired {
		t.Fatalf("state = %d, want SurfaceStateAcquired", surface.State())
	}

	// PresentPixels on noop should fail without side effects.
	err = surface.PresentPixels([]byte{0, 0, 0, 0}, 1, 1, nil)
	if err == nil {
		t.Error("PresentPixels on noop should return error")
	}

	// State must stay Acquired — texture NOT discarded on unsupported backend.
	if surface.State() != SurfaceStateAcquired {
		t.Errorf("state after PresentPixels = %d, want SurfaceStateAcquired", surface.State())
	}
}

type pixelPresentingTestSurface struct {
	noop.Surface
	discards int
	presents int
}

func (s *pixelPresentingTestSurface) DiscardTexture(texture hal.SurfaceTexture) {
	s.discards++
	s.Surface.DiscardTexture(texture)
}

func (s *pixelPresentingTestSurface) PresentPixels([]byte, uint32, uint32, []image.Rectangle) error {
	s.presents++
	return nil
}

func TestPresentPixelsExpiresActiveAcquisition(t *testing.T) {
	_, device, _ := newTestSurface(t)
	raw := &pixelPresentingTestSurface{}
	surface := NewSurface(raw, "pixel-presenting-test")
	if err := surface.Configure(device, testSurfaceConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	_, lease, err := surface.AcquireTextureWithLease(nil)
	if err != nil {
		t.Fatalf("AcquireTextureWithLease: %v", err)
	}

	if err := surface.PresentPixels([]byte{0, 0, 0, 0}, 1, 1, nil); err != nil {
		t.Fatalf("PresentPixels: %v", err)
	}
	if surface.State() != SurfaceStateConfigured {
		t.Fatalf("surface state = %v, want configured", surface.State())
	}
	if surface.AcquisitionValid(lease) {
		t.Fatal("PresentPixels left the replaced acquisition valid")
	}
	if raw.discards != 1 || raw.presents != 1 {
		t.Fatalf("backend calls = %d discards, %d presents; want one each", raw.discards, raw.presents)
	}
}
