//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// The ported tests of surface_backend_native_test.go drive the implementation's
// own methods, so they live here with the implementation (ADR-070 S10). They
// use local doubles instead of internal/hal/noop and the software backend: a
// test import is a graph edge, and the golden has no surface->hal/noop row.

// testHALSurface is the surface domain's hal.Surface double: the six methods
// core.Surface and the implementation call, and nothing else.
type testHALSurface struct {
	destroys int
}

func (s *testHALSurface) Destroy() { s.destroys++ }

func (*testHALSurface) Configure(hal.Device, *hal.SurfaceConfiguration) error { return nil }

func (*testHALSurface) Unconfigure(hal.Device) {}

func (*testHALSurface) AcquireTexture(hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return &hal.AcquiredSurfaceTexture{Texture: &testHALSurfaceTexture{}}, nil
}

func (*testHALSurface) DiscardTexture(hal.SurfaceTexture) {}

func (*testHALSurface) ActualExtent() (uint32, uint32) { return 0, 0 }

// testHALSurfaceTexture is the acquired image a testHALSurface hands out. The
// implementation never calls through it, so it embeds the interface.
type testHALSurfaceTexture struct{ hal.SurfaceTexture }

// testHALInstance is the hal.Instance double createHALSurface drives.
type testHALInstance struct {
	surface hal.Surface
	err     error
	calls   []hal.SurfaceTarget
}

func (i *testHALInstance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	i.calls = append(i.calls, target)
	return i.surface, i.err
}

func (*testHALInstance) EnumerateAdapters(hal.Surface) []hal.ExposedAdapter { return nil }
func (*testHALInstance) Destroy()                                           {}

// testHALDevice is the hal.Device a core.Device wraps in these tests. Nothing
// calls through it: Limits{} keeps core's indirect validation off, and the
// surface double ignores the device. Destroy is implemented so a core device
// teardown is a no-op rather than a nil-interface call.
type testHALDevice struct{ hal.Device }

func (*testHALDevice) Destroy() {}

// TestEnsureHALSurfaceSwitchesToRetainedBackendSurface pins that an active
// backend switch reuses the retained surface of the target backend.
func TestEnsureHALSurfaceSwitchesToRetainedBackendSurface(t *testing.T) {
	first := &testHALSurface{}
	second := &testHALSurface{}
	s := &Surface{
		core:           core.NewSurface(first, "multi-backend-switch"),
		currentBackend: gputypes.BackendVulkan,
		surfaceCreated: true,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: first,
			gputypes.BackendGL:     second,
		},
	}

	if err := s.ensureHALSurface(gputypes.BackendGL, nil); err != nil {
		t.Fatalf("ensureHALSurface: %v", err)
	}
	if got := s.core.RawSurface(); got != second {
		t.Fatalf("active HAL surface = %v, want retained GL surface %v", got, second)
	}
	if s.currentBackend != gputypes.BackendGL {
		t.Fatalf("current backend = %v, want GL", s.currentBackend)
	}

	s.Release()
}

// TestHALSurfaceForBackendDoesNotFollowActiveBackend pins the adapter-facing
// view: the surface of a backend is its retained one, never the active one.
func TestHALSurfaceForBackendDoesNotFollowActiveBackend(t *testing.T) {
	vulkanSurface := &testHALSurface{}
	glSurface := &testHALSurface{}
	s := &Surface{
		core:           core.NewSurface(vulkanSurface, "adapter-capabilities"),
		currentBackend: gputypes.BackendVulkan,
		surfaceCreated: true,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: vulkanSurface,
			gputypes.BackendGL:     glSurface,
		},
	}
	defer s.Release()

	if got := s.HALSurfaceForBackend(gputypes.BackendVulkan); got != vulkanSurface {
		t.Fatalf("Vulkan adapter surface = %v, want retained Vulkan surface %v", got, vulkanSurface)
	}
	if got := s.HALSurfaceForBackend(gputypes.BackendGL); got != glSurface {
		t.Fatalf("GL adapter surface = %v, want retained GL surface %v", got, glSurface)
	}

	if err := s.ensureHALSurface(gputypes.BackendGL, nil); err != nil {
		t.Fatalf("ensureHALSurface(GL): %v", err)
	}
	if got := s.HALSurfaceForBackend(gputypes.BackendVulkan); got != vulkanSurface {
		t.Fatalf("Vulkan adapter surface after GL activation = %v, want %v", got, vulkanSurface)
	}
	if got := s.HALSurfaceForBackend(gputypes.BackendGL); got != glSurface {
		t.Fatalf("GL adapter surface after GL activation = %v, want %v", got, glSurface)
	}
	if got := s.HALSurfaceForBackend(gputypes.BackendDX12); got != nil {
		t.Fatalf("missing DX12 adapter surface = %v, want nil", got)
	}

	legacyRaw := &testHALSurface{}
	legacy := &Surface{core: core.NewSurface(legacyRaw, "legacy-single-hal")}
	defer legacy.Release()
	if got := legacy.HALSurfaceForBackend(gputypes.BackendGL); got != legacyRaw {
		t.Fatalf("legacy single-HAL adapter surface = %v, want active surface %v", got, legacyRaw)
	}
}

// TestCreateHALSurfaceForTargetMapsBackendFailures pins the re-creation error
// shapes. The public unsupported-target sentinel is the root package's, so the
// detail the implementation returns carries only the HAL sentinel; the root
// declaration layer restores the public one (mapSurfaceConfigureError).
func TestCreateHALSurfaceForTargetMapsBackendFailures(t *testing.T) {
	target := hal.SurfaceTarget{
		Kind:          hal.SurfaceTargetXlibWindow,
		DisplayHandle: 1,
		WindowHandle:  2,
	}
	wantSurface := &testHALSurface{}
	regularErr := errors.New("driver rejected surface")
	tests := []struct {
		name     string
		instance *testHALInstance
		want     hal.Surface
		wantErr  error
	}{
		{
			name:     "success",
			instance: &testHALInstance{surface: wantSurface},
			want:     wantSurface,
		},
		{
			name:     "unsupported",
			instance: &testHALInstance{err: hal.ErrUnsupportedSurfaceTarget},
			wantErr:  hal.ErrUnsupportedSurfaceTarget,
		},
		{
			name:     "driver error",
			instance: &testHALInstance{err: regularErr},
			wantErr:  regularErr,
		},
		{
			name:     "nil surface",
			instance: &testHALInstance{},
			wantErr:  errors.New("nil surface"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := createHALSurfaceForTarget(test.instance, gputypes.BackendVulkan, target)
			if got != test.want {
				t.Fatalf("surface = %v, want %v", got, test.want)
			}
			if test.name == "nil surface" {
				if err == nil {
					t.Fatal("nil HAL surface returned no error")
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// TestCreateAndEnsureHALSurfaceCoverDeferredPaths pins the deferred creation
// path: the HAL instance is a parameter, a missing one and a missing platform
// target fail before the backend is called, and a successful creation is
// retained. The public sentinel for a Web canvas target is the declaration
// layer's conversion and stays covered by the root tests.
func TestCreateAndEnsureHALSurfaceCoverDeferredPaths(t *testing.T) {
	target := hal.SurfaceTarget{Kind: hal.SurfaceTargetXlibWindow, DisplayHandle: 1, WindowHandle: 2}
	created := &testHALSurface{}
	instance := &testHALInstance{surface: created}

	s := &Surface{
		core:      core.NewSurface(&testHALSurface{}, "deferred-create"),
		target:    target,
		targetSet: true,
	}
	if _, err := s.createHALSurface(gputypes.Backend(255), nil); err == nil {
		t.Fatal("createHALSurface succeeded without a HAL instance")
	}

	noTarget := &Surface{core: core.NewSurface(&testHALSurface{}, "no-target")}
	if _, err := noTarget.createHALSurface(gputypes.BackendSoftware, instance); err == nil {
		t.Fatal("createHALSurface succeeded without a platform target")
	}

	raw, err := s.createHALSurface(gputypes.BackendSoftware, instance)
	if err != nil {
		t.Fatalf("createHALSurface(software): %v", err)
	}
	raw.Destroy()

	if err := s.ensureHALSurface(gputypes.BackendSoftware, instance); err != nil {
		t.Fatalf("ensureHALSurface(software): %v", err)
	}
	if s.halSurfaces[gputypes.BackendSoftware] == nil {
		t.Fatal("ensureHALSurface did not retain the lazily created surface")
	}
	if s.currentBackend != gputypes.BackendSoftware || !s.surfaceCreated {
		t.Fatalf("active surface = (%v, %v), want software/created", s.currentBackend, s.surfaceCreated)
	}
	s.Release()

	failing := &Surface{
		core:      core.NewSurface(&testHALSurface{}, "deferred-failure"),
		target:    target,
		targetSet: true,
	}
	if err := failing.ensureHALSurface(gputypes.BackendSoftware, &testHALInstance{err: errors.New("driver rejected surface")}); err == nil {
		t.Fatal("ensureHALSurface reported success for a failing backend")
	}
	failing.Release()
}

// TestEnsureHALSurfaceUnconfiguresBeforeSwitch pins that a backend switch
// unconfigures the core surface before pointing it at the new HAL surface.
func TestEnsureHALSurfaceUnconfiguresBeforeSwitch(t *testing.T) {
	first := &testHALSurface{}
	second := &testHALSurface{}
	coreSurface := core.NewSurface(first, "configured-switch")
	coreDevice := core.NewDevice(&testHALDevice{}, nil, 0, gputypes.Limits{}, "configured-switch")
	t.Cleanup(coreDevice.Destroy)
	if err := coreSurface.Configure(coreDevice, &hal.SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	s := &Surface{
		core:           coreSurface,
		currentBackend: gputypes.BackendVulkan,
		surfaceCreated: true,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: first,
			gputypes.BackendGL:     second,
		},
	}

	if err := s.ensureHALSurface(gputypes.BackendGL, nil); err != nil {
		t.Fatalf("ensureHALSurface: %v", err)
	}
	if coreSurface.State() != core.SurfaceStateUnconfigured {
		t.Fatalf("surface state = %v, want unconfigured", coreSurface.State())
	}
	s.Release()
}

// TestSurfaceBackendViewsHandleNilReleasedAndLegacySurfaces pins the nil,
// released and legacy answers of the adapter-facing views.
func TestSurfaceBackendViewsHandleNilReleasedAndLegacySurfaces(t *testing.T) {
	var nilSurface *Surface
	if got := nilSurface.HALSurfaceForBackend(gputypes.BackendVulkan); got != nil {
		t.Fatalf("nil surface HAL = %v, want nil", got)
	}
	if got := nilSurface.HALSurfacesForAdapterRequest(); got != nil {
		t.Fatalf("nil surface adapter map = %v, want nil", got)
	}

	raw := &testHALSurface{}
	vulkan := &testHALSurface{}
	s := &Surface{
		core:           core.NewSurface(raw, "backend-views"),
		currentBackend: gputypes.BackendGL,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: vulkan,
		},
	}
	copied := s.HALSurfacesForAdapterRequest()
	if copied[gputypes.BackendVulkan] != vulkan {
		t.Fatalf("copied Vulkan surface = %v, want %v", copied[gputypes.BackendVulkan], vulkan)
	}
	delete(copied, gputypes.BackendVulkan)
	if s.halSurfaces[gputypes.BackendVulkan] != vulkan {
		t.Fatal("adapter surface map aliases the owned map")
	}
	s.Release()
	if got := s.HALSurfaceForBackend(gputypes.BackendVulkan); got != nil {
		t.Fatalf("released surface HAL = %v, want nil", got)
	}
	if got := s.HALSurfacesForAdapterRequest(); got != nil {
		t.Fatalf("released surface adapter map = %v, want nil", got)
	}

	legacyRaw := &testHALSurface{}
	legacy := &Surface{
		core:           core.NewSurface(legacyRaw, "legacy-adapter-map"),
		currentBackend: gputypes.BackendGL,
	}
	legacyMap := legacy.HALSurfacesForAdapterRequest()
	if len(legacyMap) != 1 || legacyMap[gputypes.BackendGL] != legacyRaw {
		t.Fatalf("legacy adapter map = %v, want GL raw surface", legacyMap)
	}
	legacy.Release()

	empty := &Surface{core: core.NewSurface(nil, "empty-adapter-map")}
	if got := empty.HALSurfacesForAdapterRequest(); len(got) != 0 {
		t.Fatalf("empty adapter map = %v, want empty", got)
	}
	empty.Release()
}
