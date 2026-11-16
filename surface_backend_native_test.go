//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/hal/software"
	instancepkg "github.com/gogpu/wgpu/internal/instance"
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
	hal.RegisterBackend(software.NewBackend())
	// Software-only instance. These tests drive synthetic platform targets (a
	// fake HWND, Xlib window or CAMetalLayer pointer); handing one to a real
	// hardware backend is invalid input that crashes instead of erroring on
	// macOS, where Metal retains the layer pointer. The mask names the software
	// backend (ADR-083 §2.3).
	instance, err := CreateInstance(&InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	if instance.impl.Core().HALInstanceForBackend(gputypes.BackendSoftware) == nil {
		t.Fatal("software HAL instance was not registered")
	}
	return instance
}

func currentPlatformSurfaceTarget(t *testing.T) SurfaceTargetUnsafe {
	t.Helper()
	switch runtime.GOOS {
	case platformWindows:
		return SurfaceTargetFromWindowsHWND(1, 2)
	case platformDarwin:
		return SurfaceTargetFromMetalLayer(2)
	case platformLinux:
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
	softwareInstance := &surfaceCreateTestInstance{surface: softwareSurface}

	surfaces, firstBackend, err := createHALSurfaces([]core.HALInstanceEntry{
		{Backend: gputypes.BackendGL, Instance: unsupported},
		{Backend: gputypes.BackendVulkan, Instance: vulkan},
		{Backend: gputypes.BackendSoftware, Instance: softwareInstance},
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
	if surfaces[gputypes.BackendSoftware] != softwareSurface {
		t.Fatal("software surface was not retained under the software backend")
	}
	for name, instance := range map[string]*surfaceCreateTestInstance{
		"unsupported": unsupported,
		"Vulkan":      vulkan,
		"software":    softwareInstance,
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

// TestEnsureHALSurfaceSwitchesToRetainedBackendSurface,
// TestHALSurfaceForBackendDoesNotFollowActiveBackend,
// TestCreateHALSurfaceForTargetMapsBackendFailures,
// TestCreateAndEnsureHALSurfaceCoverDeferredPaths,
// TestEnsureHALSurfaceUnconfiguresBeforeSwitch and
// TestSurfaceBackendViewsHandleNilReleasedAndLegacySurfaces moved to
// internal/surface with the implementation they exercise (ADR-070 S10).
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
			goos: platformWindows, display: 1, window: 2,
			want: SurfaceTargetFromWindowsHWND(1, 2),
		},
		{
			name: "Darwin",
			goos: platformDarwin, display: 1, window: 2,
			want: SurfaceTargetFromMetalLayer(2),
		},
		{
			name: "Linux Xlib",
			goos: platformLinux, display: 1, window: 2,
			want: SurfaceTargetFromXlibWindow(1, 2),
		},
		{
			name: "Linux Wayland", goos: platformLinux, waylandDisplay: "wayland-0", display: 1, window: 2,
			want: SurfaceTargetFromWaylandSurface(1, 2),
		},
		{
			name: "Android",
			goos: platformAndroid, display: 1, window: 2,
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
				DisplayHandle{},
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
	if legacy.impl.Core().RawSurface() == nil {
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
	if safeSurface.impl.HALSurfacesForAdapterRequest()[gputypes.BackendSoftware] == nil {
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

	empty := &Instance{impl: instancepkg.New(core.NewInstanceWithMock(nil))}
	t.Cleanup(empty.impl.Release)
	if surface, err := empty.CreateSurfaceUnsafe(target); surface != nil || err == nil {
		t.Fatalf("CreateSurfaceUnsafe without HAL = (%v, %v), want nil error result", surface, err)
	}

	instance := newSoftwareSurfaceTestInstance(t)
	if surface, err := instance.createSurface(SurfaceTargetFromWebCanvasID(1), nil); surface != nil ||
		!errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("native web target = (%v, %v), want ErrUnsupportedSurfaceTarget", surface, err)
	}

	if runtime.GOOS != platformAndroid {
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
	impl := instancepkg.New(core.NewInstanceWithMock(nil))
	impl.Release()
	instance := &Instance{impl: impl}
	t.Cleanup(impl.Release)
	first := &surfaceDestroyCounter{}
	second := &surfaceDestroyCounter{}

	surface, err := instance.adoptCreatedSurface(
		nil,
		map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: first,
			gputypes.BackendGL:     second,
		},
		gputypes.BackendVulkan,
		hal.SurfaceTarget{Kind: hal.SurfaceTargetAndroidNativeWindow, WindowHandle: 1},
	)
	if surface != nil || !errors.Is(err, ErrReleased) {
		t.Fatalf("adoptCreatedSurface = (%v, %v), want ErrReleased", surface, err)
	}
	if first.destroys != 1 || second.destroys != 1 {
		t.Fatalf("destroy counts = (%d, %d), want (1, 1)", first.destroys, second.destroys)
	}
}
