//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
	"github.com/gogpu/wgpu/hal/software"
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

type surfaceTargetTestProvider struct {
	target SurfaceTargetUnsafe
	calls  int
}

func (p *surfaceTargetTestProvider) SurfaceTarget() (SurfaceTargetUnsafe, error) {
	p.calls++
	return p.target, nil
}

type surfaceDestroyCounter struct {
	noop.Surface
	destroys int
}

func (s *surfaceDestroyCounter) Destroy() {
	s.destroys++
}

func newSoftwareSurfaceTestInstance(t *testing.T) *Instance {
	t.Helper()
	hal.RegisterBackend(software.API{})
	instance, err := CreateInstance(&InstanceDescriptor{Backends: BackendsAll})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	if instance.core.HALInstanceForBackend(gputypes.BackendEmpty) == nil {
		t.Fatal("software HAL instance was not registered")
	}
	return instance
}

func currentPlatformSurfaceTarget(t *testing.T) SurfaceTargetUnsafe {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		return SurfaceTargetFromWindowsHWND(1, 2)
	case "darwin":
		return SurfaceTargetFromMetalLayer(2)
	case "linux":
		return SurfaceTargetFromXlibWindow(1, 2)
	default:
		t.Skipf("no software window target for %s", runtime.GOOS)
		return SurfaceTargetUnsafe{}
	}
}

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

	_, _, err = createHALSurfaces([]core.HALInstanceEntry{
		{Backend: gputypes.BackendVulkan, Instance: &surfaceCreateTestInstance{}},
	}, hal.SurfaceTarget{Kind: hal.SurfaceTargetXlibWindow, DisplayHandle: 1, WindowHandle: 2})
	if err == nil || errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("nil backend surface error = %v, want ordinary backend failure", err)
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

func TestSurfaceTargetFromLegacyHandlesForPlatform(t *testing.T) {
	tests := []struct {
		name            string
		goos            string
		waylandDisplay  string
		display, window uintptr
		want            SurfaceTargetUnsafe
	}{
		{
			name: "Windows",
			goos: "windows", display: 1, window: 2,
			want: SurfaceTargetFromWindowsHWND(1, 2),
		},
		{
			name: "Darwin",
			goos: "darwin", display: 1, window: 2,
			want: SurfaceTargetFromMetalLayer(2),
		},
		{
			name: "Linux Xlib",
			goos: "linux", display: 1, window: 2,
			want: SurfaceTargetFromXlibWindow(1, 2),
		},
		{
			name: "Linux Wayland", goos: "linux", waylandDisplay: "wayland-0", display: 1, window: 2,
			want: SurfaceTargetFromWaylandSurface(1, 2),
		},
		{
			name: "Android",
			goos: "android", display: 1, window: 2,
			want: SurfaceTargetFromAndroidNativeWindow(2),
		},
		{
			name: "unsupported",
			goos: "plan9", display: 1, window: 2,
			want: SurfaceTargetUnsafe{kind: surfaceTargetInvalid, displayHandle: 1, windowHandle: 2},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := surfaceTargetFromLegacyHandlesForPlatform(
				test.goos,
				test.waylandDisplay,
				test.display,
				test.window,
			)
			if got != test.want {
				t.Fatalf("legacy target = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestTypedSurfaceCreationPathsUseBackendMap(t *testing.T) {
	instance := newSoftwareSurfaceTestInstance(t)
	target := currentPlatformSurfaceTarget(t)

	legacy, err := instance.CreateSurface(target.displayHandle, target.windowHandle)
	if err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}
	if legacy.HAL() == nil {
		t.Fatal("legacy surface has no HAL surface")
	}
	legacy.Release()

	unsafeSurface, err := instance.CreateSurfaceUnsafe(target)
	if err != nil {
		t.Fatalf("CreateSurfaceUnsafe: %v", err)
	}
	if unsafeSurface.targetSource != nil {
		t.Fatal("unsafe surface retained an ownership source")
	}
	unsafeSurface.Release()

	provider := &surfaceTargetTestProvider{target: target}
	safeSurface, err := instance.CreateSurfaceFromTarget(provider)
	if err != nil {
		t.Fatalf("CreateSurfaceFromTarget: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if safeSurface.targetSource != provider {
		t.Fatal("safe surface did not retain its provider")
	}
	if safeSurface.halSurfaces[gputypes.BackendEmpty] == nil {
		t.Fatal("safe surface did not retain the software backend surface")
	}

	adapter, err := instance.RequestAdapter(&RequestAdapterOptions{CompatibleSurface: safeSurface})
	if err != nil {
		t.Fatalf("RequestAdapter(compatible surface): %v", err)
	}
	if capabilities := adapter.GetSurfaceCapabilities(safeSurface); capabilities == nil {
		t.Fatal("compatible software adapter returned no surface capabilities")
	}
	adapter.Release()

	safeSurface.Release()
	if safeSurface.targetSource != nil {
		t.Fatal("released safe surface retained its provider")
	}
}

func TestCreateSurfaceReportsBoundaryFailures(t *testing.T) {
	target := currentPlatformSurfaceTarget(t)

	empty := &Instance{core: core.NewInstanceWithMock(nil)}
	t.Cleanup(empty.core.Destroy)
	if surface, err := empty.CreateSurfaceUnsafe(target); surface != nil || err == nil {
		t.Fatalf("CreateSurfaceUnsafe without HAL = (%v, %v), want nil error result", surface, err)
	}

	instance := newSoftwareSurfaceTestInstance(t)
	if surface, err := instance.createSurface(SurfaceTargetFromWebCanvasID(1), nil); surface != nil ||
		!errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("native web target = (%v, %v), want ErrUnsupportedSurfaceTarget", surface, err)
	}

	if runtime.GOOS != "android" {
		surface, err := instance.CreateSurfaceUnsafe(SurfaceTargetFromAndroidNativeWindow(1))
		if surface != nil {
			surface.Release()
			t.Fatal("desktop software backend accepted Android target")
		}
		if !errors.Is(err, ErrUnsupportedSurfaceTarget) {
			t.Fatalf("Android target error = %v, want ErrUnsupportedSurfaceTarget", err)
		}
	}

	released := newSoftwareSurfaceTestInstance(t)
	released.Release()
	if surface, err := released.CreateSurface(target.displayHandle, target.windowHandle); surface != nil ||
		!errors.Is(err, ErrReleased) {
		t.Fatalf("CreateSurface after release = (%v, %v), want ErrReleased", surface, err)
	}
}

func TestAdoptCreatedSurfaceDestroysEverySurfaceOnReleaseRace(t *testing.T) {
	instance := &Instance{core: core.NewInstanceWithMock(nil), released: true}
	t.Cleanup(instance.core.Destroy)
	first := &surfaceDestroyCounter{}
	second := &surfaceDestroyCounter{}

	surface, err := instance.adoptCreatedSurface(
		SurfaceTargetFromAndroidNativeWindow(1),
		nil,
		map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: first,
			gputypes.BackendGL:     second,
		},
		gputypes.BackendVulkan,
	)
	if surface != nil || !errors.Is(err, ErrReleased) {
		t.Fatalf("adoptCreatedSurface = (%v, %v), want ErrReleased", surface, err)
	}
	if first.destroys != 1 || second.destroys != 1 {
		t.Fatalf("destroy counts = (%d, %d), want (1, 1)", first.destroys, second.destroys)
	}
}

func TestCreateHALSurfaceForTargetMapsBackendFailures(t *testing.T) {
	target := hal.SurfaceTarget{
		Kind:          hal.SurfaceTargetXlibWindow,
		DisplayHandle: 1,
		WindowHandle:  2,
	}
	wantSurface := &noop.Surface{}
	regularErr := errors.New("driver rejected surface")
	tests := []struct {
		name     string
		instance *surfaceCreateTestInstance
		want     hal.Surface
		wantErr  error
	}{
		{
			name:     "success",
			instance: &surfaceCreateTestInstance{surface: wantSurface},
			want:     wantSurface,
		},
		{
			name:     "unsupported",
			instance: &surfaceCreateTestInstance{err: hal.ErrUnsupportedSurfaceTarget},
			wantErr:  ErrUnsupportedSurfaceTarget,
		},
		{
			name:     "driver error",
			instance: &surfaceCreateTestInstance{err: regularErr},
			wantErr:  regularErr,
		},
		{
			name:     "nil surface",
			instance: &surfaceCreateTestInstance{},
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

func TestCreateAndEnsureHALSurfaceCoverDeferredPaths(t *testing.T) {
	instance := newSoftwareSurfaceTestInstance(t)
	target := currentPlatformSurfaceTarget(t)

	surface := &Surface{
		core:     core.NewSurface(&noop.Surface{}, "deferred-create"),
		instance: instance,
		target:   target,
	}
	if _, err := surface.createHALSurface(gputypes.Backend(255)); err == nil {
		t.Fatal("createHALSurface succeeded without a matching HAL instance")
	}
	surface.target = SurfaceTargetFromWebCanvasID(1)
	if _, err := surface.createHALSurface(gputypes.BackendEmpty); !errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("createHALSurface(web target) = %v, want ErrUnsupportedSurfaceTarget", err)
	}
	surface.target = target
	raw, err := surface.createHALSurface(gputypes.BackendEmpty)
	if err != nil {
		t.Fatalf("createHALSurface(software): %v", err)
	}
	raw.Destroy()

	if err := surface.ensureHALSurface(gputypes.BackendEmpty); err != nil {
		t.Fatalf("ensureHALSurface(software): %v", err)
	}
	if surface.halSurfaces[gputypes.BackendEmpty] == nil {
		t.Fatal("ensureHALSurface did not retain the lazily created surface")
	}
	if surface.currentBackend != gputypes.BackendEmpty || !surface.surfaceCreated {
		t.Fatalf("active surface = (%v, %v), want software/created", surface.currentBackend, surface.surfaceCreated)
	}
	surface.Release()

	failing := &Surface{
		core:     core.NewSurface(&noop.Surface{}, "deferred-failure"),
		instance: instance,
		target:   target,
	}
	if err := failing.ensureHALSurface(gputypes.Backend(255)); err == nil {
		t.Fatal("ensureHALSurface succeeded without a matching HAL instance")
	}
	failing.Release()
}

func TestEnsureHALSurfaceUnconfiguresBeforeSwitch(t *testing.T) {
	first := &noop.Surface{}
	second := &noop.Surface{}
	coreSurface := core.NewSurface(first, "configured-switch")
	coreDevice := core.NewDevice(&noop.Device{}, nil, 0, gputypes.DefaultLimits(), "configured-switch")
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
	surface := &Surface{
		core:           coreSurface,
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
	if coreSurface.State() != core.SurfaceStateUnconfigured {
		t.Fatalf("surface state = %v, want unconfigured", coreSurface.State())
	}
	surface.Release()
}

func TestSurfaceBackendViewsHandleNilReleasedAndLegacySurfaces(t *testing.T) {
	var nilSurface *Surface
	if got := nilSurface.halSurfaceForBackend(gputypes.BackendVulkan); got != nil {
		t.Fatalf("nil surface HAL = %v, want nil", got)
	}
	if got := nilSurface.halSurfacesForAdapterRequest(); got != nil {
		t.Fatalf("nil surface adapter map = %v, want nil", got)
	}

	raw := &noop.Surface{}
	vulkan := &noop.Surface{}
	surface := &Surface{
		core:           core.NewSurface(raw, "backend-views"),
		currentBackend: gputypes.BackendGL,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: vulkan,
		},
	}
	copied := surface.halSurfacesForAdapterRequest()
	if copied[gputypes.BackendVulkan] != vulkan {
		t.Fatalf("copied Vulkan surface = %v, want %v", copied[gputypes.BackendVulkan], vulkan)
	}
	delete(copied, gputypes.BackendVulkan)
	if surface.halSurfaces[gputypes.BackendVulkan] != vulkan {
		t.Fatal("adapter surface map aliases the owned map")
	}
	surface.Release()
	if got := surface.halSurfaceForBackend(gputypes.BackendVulkan); got != nil {
		t.Fatalf("released surface HAL = %v, want nil", got)
	}
	if got := surface.halSurfacesForAdapterRequest(); got != nil {
		t.Fatalf("released surface adapter map = %v, want nil", got)
	}

	legacyRaw := &noop.Surface{}
	legacy := &Surface{
		core:           core.NewSurface(legacyRaw, "legacy-adapter-map"),
		currentBackend: gputypes.BackendGL,
	}
	legacyMap := legacy.halSurfacesForAdapterRequest()
	if len(legacyMap) != 1 || legacyMap[gputypes.BackendGL] != legacyRaw {
		t.Fatalf("legacy adapter map = %v, want GL raw surface", legacyMap)
	}
	legacy.Release()

	empty := &Surface{core: core.NewSurface(nil, "empty-adapter-map")}
	if got := empty.halSurfacesForAdapterRequest(); len(got) != 0 {
		t.Fatalf("empty adapter map = %v, want empty", got)
	}
	empty.Release()
}
