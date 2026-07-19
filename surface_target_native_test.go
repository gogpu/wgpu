//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

func TestCreateSurfaceUnsafeRejectsInvalidTargets(t *testing.T) {
	instance := newInstance(t)
	defer instance.Release()

	tests := []struct {
		name   string
		target wgpu.SurfaceTargetUnsafe
	}{
		{name: "empty", target: wgpu.SurfaceTargetUnsafe{}},
		{name: "Win32 HWND", target: wgpu.SurfaceTargetFromWindowsHWND(0, 0)},
		{name: "Xlib display", target: wgpu.SurfaceTargetFromXlibWindow(0, 1)},
		{name: "Xlib window", target: wgpu.SurfaceTargetFromXlibWindow(1, 0)},
		{name: "Wayland display", target: wgpu.SurfaceTargetFromWaylandSurface(0, 1)},
		{name: "Wayland surface", target: wgpu.SurfaceTargetFromWaylandSurface(1, 0)},
		{name: "Android native window", target: wgpu.SurfaceTargetFromAndroidNativeWindow(0)},
		{name: "Metal layer", target: wgpu.SurfaceTargetFromMetalLayer(0)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			surface, err := instance.CreateSurfaceUnsafe(test.target)
			if surface != nil {
				surface.Release()
				t.Fatal("CreateSurfaceUnsafe returned a surface for an invalid target")
			}
			if !errors.Is(err, wgpu.ErrInvalidSurfaceTarget) {
				t.Fatalf("CreateSurfaceUnsafe error = %v, want ErrInvalidSurfaceTarget", err)
			}
		})
	}
}

type testSurfaceTargetProvider struct {
	target wgpu.SurfaceTargetUnsafe
	err    error
	calls  int
}

func (p *testSurfaceTargetProvider) SurfaceTarget() (wgpu.SurfaceTargetUnsafe, error) {
	p.calls++
	return p.target, p.err
}

func TestCreateSurfaceFromTargetRejectsNilProvider(t *testing.T) {
	instance := newInstance(t)
	defer instance.Release()

	var provider *testSurfaceTargetProvider
	surface, err := instance.CreateSurfaceFromTarget(provider)
	if surface != nil {
		surface.Release()
		t.Fatal("CreateSurfaceFromTarget returned a surface for a nil provider")
	}
	if !errors.Is(err, wgpu.ErrInvalidSurfaceTarget) {
		t.Fatalf("CreateSurfaceFromTarget error = %v, want ErrInvalidSurfaceTarget", err)
	}
}

func TestCreateSurfaceFromTargetPropagatesProviderError(t *testing.T) {
	instance := newInstance(t)
	defer instance.Release()

	wantErr := errors.New("window unavailable")
	provider := &testSurfaceTargetProvider{err: wantErr}
	surface, err := instance.CreateSurfaceFromTarget(provider)
	if surface != nil {
		surface.Release()
		t.Fatal("CreateSurfaceFromTarget returned a surface after provider failure")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateSurfaceFromTarget error = %v, want provider error", err)
	}
	if provider.calls != 1 {
		t.Fatalf("SurfaceTarget calls = %d, want 1", provider.calls)
	}
}

func TestCreateSurfaceFromTargetDoesNotCallProviderAfterInstanceRelease(t *testing.T) {
	instance := newInstance(t)
	instance.Release()

	provider := &testSurfaceTargetProvider{}
	surface, err := instance.CreateSurfaceFromTarget(provider)
	if surface != nil {
		surface.Release()
		t.Fatal("CreateSurfaceFromTarget returned a surface after instance release")
	}
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("CreateSurfaceFromTarget error = %v, want ErrReleased", err)
	}
	if provider.calls != 0 {
		t.Fatalf("SurfaceTarget calls = %d, want 0 after instance release", provider.calls)
	}
}

func TestCreateSurfaceUnsafeChecksInstanceBeforeTarget(t *testing.T) {
	instance := newInstance(t)
	instance.Release()

	surface, err := instance.CreateSurfaceUnsafe(wgpu.SurfaceTargetUnsafe{})
	if surface != nil {
		surface.Release()
		t.Fatal("CreateSurfaceUnsafe returned a surface after instance release")
	}
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("CreateSurfaceUnsafe error = %v, want ErrReleased", err)
	}
}
