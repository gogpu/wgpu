//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software"
)

func TestCreateInstanceRejectsNilDisplay(t *testing.T) {
	for _, h := range []DisplayHandle{DisplayHandleFromXlib(0), DisplayHandleFromWayland(0)} {
		instance, err := CreateInstance(&InstanceDescriptor{Display: h})
		if instance != nil {
			instance.Release()
		}
		if !errors.Is(err, ErrInvalidDisplayHandle) {
			t.Fatalf("CreateInstance(%+v) error = %v, want ErrInvalidDisplayHandle", h, err)
		}
	}
}

func newDisplaySurfaceTestInstance(t *testing.T, display DisplayHandle) *Instance {
	t.Helper()
	hal.RegisterBackend(software.NewBackend())
	instance, err := CreateInstance(&InstanceDescriptor{Backends: BackendsAll, Display: display})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	return instance
}

// TestCreateSurfaceRejectsForeignDisplay checks every surface creation path
// against the instance display before any backend sees the target.
func TestCreateSurfaceRejectsForeignDisplay(t *testing.T) {
	const dpy = 0x1000
	instance := newDisplaySurfaceTestInstance(t, DisplayHandleFromWayland(dpy))

	foreign := []SurfaceTargetUnsafe{
		SurfaceTargetFromWaylandSurface(dpy+0x10, 2),
		SurfaceTargetFromXlibWindow(dpy, 2),
	}
	for _, target := range foreign {
		surface, err := instance.CreateSurfaceUnsafe(target)
		if surface != nil {
			surface.Release()
		}
		if !errors.Is(err, ErrMismatchingDisplayHandle) {
			t.Fatalf("CreateSurfaceUnsafe(%+v) error = %v, want ErrMismatchingDisplayHandle", target, err)
		}

		provider := &surfaceTargetTestProvider{target: target}
		surface, err = instance.CreateSurfaceFromTarget(provider)
		if surface != nil {
			surface.Release()
		}
		if !errors.Is(err, ErrMismatchingDisplayHandle) {
			t.Fatalf("CreateSurfaceFromTarget(%+v) error = %v, want ErrMismatchingDisplayHandle", target, err)
		}
	}
}

func TestLegacySurfaceTargetRoutesByInstanceDisplay(t *testing.T) {
	const dpy, win = 0x1000, 0x42
	tests := []struct {
		name           string
		waylandDisplay string
		display        DisplayHandle
		want           SurfaceTargetUnsafe
	}{
		{"Wayland display without WAYLAND_DISPLAY", "", DisplayHandleFromWayland(dpy), SurfaceTargetFromWaylandSurface(dpy, win)},
		{"Xlib display under WAYLAND_DISPLAY", "wayland-0", DisplayHandleFromXlib(dpy), SurfaceTargetFromXlibWindow(dpy, win)},
		{"no display follows WAYLAND_DISPLAY", "wayland-0", DisplayHandle{}, SurfaceTargetFromWaylandSurface(dpy, win)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := surfaceTargetFromLegacyHandlesForPlatform(platformLinux, test.waylandDisplay, test.display, dpy, win)
			if got != test.want {
				t.Fatalf("legacy target = %+v, want %+v", got, test.want)
			}
		})
	}

	// The instance display is a Linux window-system handle: other platforms
	// keep their own legacy mapping.
	got := surfaceTargetFromLegacyHandlesForPlatform(platformWindows, "", DisplayHandleFromWayland(dpy), dpy, win)
	if got != SurfaceTargetFromWindowsHWND(dpy, win) {
		t.Fatalf("Windows legacy target = %+v, want an HWND target", got)
	}
}

func TestDisplayHandleHALConversion(t *testing.T) {
	tests := []struct {
		in   DisplayHandle
		want hal.DisplayHandle
	}{
		{DisplayHandle{}, hal.DisplayHandle{}},
		{DisplayHandleFromXlib(0x10), hal.DisplayHandle{Kind: hal.DisplayKindXlib, Display: 0x10}},
		{DisplayHandleFromWayland(0x20), hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: 0x20}},
	}
	for _, test := range tests {
		if got := test.in.halDisplay(); got != test.want {
			t.Fatalf("halDisplay(%+v) = %+v, want %+v", test.in, got, test.want)
		}
	}
}

// TestSurfaceConfigureMapsDeviceMismatch checks that the backend's
// device-mismatch error reaches the caller as the public sentinel, with the
// backend detail kept.
func TestSurfaceConfigureMapsDeviceMismatch(t *testing.T) {
	backendErr := fmt.Errorf("gles: %w: device context 0x1, surface context 0x2", hal.ErrSurfaceDeviceMismatch)
	err := mapSurfaceConfigureError(backendErr)
	if !errors.Is(err, ErrSurfaceDeviceMismatch) {
		t.Fatalf("mapped error %v is not ErrSurfaceDeviceMismatch", err)
	}
	if !errors.Is(err, hal.ErrSurfaceDeviceMismatch) {
		t.Fatalf("mapped error %v lost the backend error", err)
	}

	other := errors.New("other configure failure")
	if got := mapSurfaceConfigureError(other); !errors.Is(got, other) || errors.Is(got, ErrSurfaceDeviceMismatch) {
		t.Fatalf("an unrelated error was rewritten: %v", got)
	}
	if mapSurfaceConfigureError(nil) != nil {
		t.Fatal("nil error was rewritten")
	}
}
