//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

type surfaceCreateTestInstance struct {
	surface hal.Surface
	err     error
	calls   []hal.SurfaceTarget
}

func (i *surfaceCreateTestInstance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	i.calls = append(i.calls, target)
	return i.surface, i.err
}

func (*surfaceCreateTestInstance) EnumerateAdapters(hal.Surface) []hal.ExposedAdapter { return nil }
func (*surfaceCreateTestInstance) Destroy()                                           {}

func TestCreateHALSurfacesTriesEveryBackendAndSucceedsIfAny(t *testing.T) {
	target := hal.SurfaceTarget{
		Kind:          hal.SurfaceTargetXlibWindow,
		DisplayHandle: 1,
		WindowHandle:  2,
	}
	unsupported := &surfaceCreateTestInstance{err: hal.ErrUnsupportedSurfaceTarget}
	vulkanSurface := &noop.Surface{}
	vulkan := &surfaceCreateTestInstance{surface: vulkanSurface}
	softwareSurface := &noop.Surface{}
	software := &surfaceCreateTestInstance{surface: softwareSurface}

	surfaces, firstBackend, err := createHALSurfaces([]core.HALInstanceEntry{
		{Backend: gputypes.BackendGL, Instance: unsupported},
		{Backend: gputypes.BackendVulkan, Instance: vulkan},
		{Backend: gputypes.BackendEmpty, Instance: software},
	}, target)
	if err != nil {
		t.Fatalf("createHALSurfaces: %v", err)
	}
	if firstBackend != gputypes.BackendVulkan {
		t.Fatalf("first successful backend = %v, want Vulkan", firstBackend)
	}
	if len(surfaces) != 2 {
		t.Fatalf("surface count = %d, want 2", len(surfaces))
	}
	if surfaces[gputypes.BackendVulkan] != vulkanSurface {
		t.Fatal("Vulkan surface was not retained under the Vulkan backend")
	}
	if surfaces[gputypes.BackendEmpty] != softwareSurface {
		t.Fatal("software surface was not retained under the software backend")
	}
	for name, instance := range map[string]*surfaceCreateTestInstance{
		"unsupported": unsupported,
		"Vulkan":      vulkan,
		"software":    software,
	} {
		if len(instance.calls) != 1 || instance.calls[0] != target {
			t.Fatalf("%s calls = %+v, want target once", name, instance.calls)
		}
	}
}

func TestCreateHALSurfacesReportsFailureOnlyWhenAllBackendsFail(t *testing.T) {
	regularErr := errors.New("driver rejected surface")
	_, _, err := createHALSurfaces([]core.HALInstanceEntry{
		{Backend: gputypes.BackendGL, Instance: &surfaceCreateTestInstance{err: hal.ErrUnsupportedSurfaceTarget}},
		{Backend: gputypes.BackendVulkan, Instance: &surfaceCreateTestInstance{err: regularErr}},
	}, hal.SurfaceTarget{Kind: hal.SurfaceTargetXlibWindow, DisplayHandle: 1, WindowHandle: 2})
	if !errors.Is(err, regularErr) {
		t.Fatalf("createHALSurfaces error = %v, want joined driver error", err)
	}
	if errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("mixed backend failure = %v, must not collapse to ErrUnsupportedSurfaceTarget", err)
	}
}

func TestCreateHALSurfacesMapsAllUnsupportedFailures(t *testing.T) {
	_, _, err := createHALSurfaces([]core.HALInstanceEntry{
		{Backend: gputypes.BackendGL, Instance: &surfaceCreateTestInstance{err: hal.ErrUnsupportedSurfaceTarget}},
		{Backend: gputypes.BackendVulkan, Instance: &surfaceCreateTestInstance{err: hal.ErrUnsupportedSurfaceTarget}},
	}, hal.SurfaceTarget{Kind: hal.SurfaceTargetAndroidNativeWindow, WindowHandle: 1})
	if !errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("createHALSurfaces error = %v, want ErrUnsupportedSurfaceTarget", err)
	}
}

func TestEnsureHALSurfaceSwitchesToRetainedBackendSurface(t *testing.T) {
	first := &noop.Surface{}
	second := &noop.Surface{}
	surface := &Surface{
		core:           core.NewSurface(first, "multi-backend-switch"),
		currentBackend: gputypes.BackendVulkan,
		surfaceCreated: true,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: first,
			gputypes.BackendGL:     second,
		},
	}

	if err := surface.ensureHALSurface(gputypes.BackendGL); err != nil {
		t.Fatalf("ensureHALSurface: %v", err)
	}
	if got := surface.HAL(); got != second {
		t.Fatalf("active HAL surface = %v, want retained GL surface %v", got, second)
	}
	if surface.currentBackend != gputypes.BackendGL {
		t.Fatalf("current backend = %v, want GL", surface.currentBackend)
	}

	surface.Release()
}

func TestHALSurfaceForBackendDoesNotFollowActiveBackend(t *testing.T) {
	vulkanSurface := &noop.Surface{}
	glSurface := &noop.Surface{}
	surface := &Surface{
		core:           core.NewSurface(vulkanSurface, "adapter-capabilities"),
		currentBackend: gputypes.BackendVulkan,
		surfaceCreated: true,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: vulkanSurface,
			gputypes.BackendGL:     glSurface,
		},
	}
	defer surface.Release()

	if got := surface.halSurfaceForBackend(gputypes.BackendVulkan); got != vulkanSurface {
		t.Fatalf("Vulkan adapter surface = %v, want retained Vulkan surface %v", got, vulkanSurface)
	}
	if got := surface.halSurfaceForBackend(gputypes.BackendGL); got != glSurface {
		t.Fatalf("GL adapter surface = %v, want retained GL surface %v", got, glSurface)
	}

	if err := surface.ensureHALSurface(gputypes.BackendGL); err != nil {
		t.Fatalf("ensureHALSurface(GL): %v", err)
	}
	if got := surface.halSurfaceForBackend(gputypes.BackendVulkan); got != vulkanSurface {
		t.Fatalf("Vulkan adapter surface after GL activation = %v, want %v", got, vulkanSurface)
	}
	if got := surface.halSurfaceForBackend(gputypes.BackendGL); got != glSurface {
		t.Fatalf("GL adapter surface after GL activation = %v, want %v", got, glSurface)
	}
	if got := surface.halSurfaceForBackend(gputypes.BackendDX12); got != nil {
		t.Fatalf("missing DX12 adapter surface = %v, want nil", got)
	}

	legacyRaw := &noop.Surface{}
	legacy := &Surface{core: core.NewSurface(legacyRaw, "legacy-single-hal")}
	defer legacy.Release()
	if got := legacy.halSurfaceForBackend(gputypes.BackendGL); got != legacyRaw {
		t.Fatalf("legacy single-HAL adapter surface = %v, want active surface %v", got, legacyRaw)
	}
}
