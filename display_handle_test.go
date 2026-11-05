package wgpu

import (
	"errors"
	"testing"
)

// The descriptor field exists on every build (native, -tags rust, js/wasm):
// this file has no build tag, so `go vet -tags rust ./...` and
// `GOOS=js GOARCH=wasm go vet ./...` compile it too.
var _ = InstanceDescriptor{Display: DisplayHandleFromWayland(0x1000)}

func TestDisplayHandleConstructors(t *testing.T) {
	var zero DisplayHandle
	if !zero.IsZero() {
		t.Fatal("zero DisplayHandle is not IsZero")
	}
	if err := zero.validate(); err != nil {
		t.Fatalf("zero DisplayHandle validate = %v, want nil (no display)", err)
	}

	x := DisplayHandleFromXlib(0x1000)
	w := DisplayHandleFromWayland(0x1000)
	if x.IsZero() || w.IsZero() {
		t.Fatal("a constructed DisplayHandle reports IsZero")
	}
	if x == w {
		t.Fatal("Xlib and Wayland handles on the same pointer compare equal")
	}
	if x != DisplayHandleFromXlib(0x1000) {
		t.Fatal("equal Xlib handles compare different")
	}
	if x == DisplayHandleFromXlib(0x2000) {
		t.Fatal("Xlib handles of different displays compare equal")
	}
	for _, h := range []DisplayHandle{x, w} {
		if err := h.validate(); err != nil {
			t.Fatalf("validate(%+v) = %v, want nil", h, err)
		}
	}
}

func TestDisplayHandleNilPointerIsInvalid(t *testing.T) {
	for name, h := range map[string]DisplayHandle{
		"xlib":    DisplayHandleFromXlib(0),
		"wayland": DisplayHandleFromWayland(0),
	} {
		t.Run(name, func(t *testing.T) {
			if h.IsZero() {
				t.Fatal("a handle built from a nil pointer reports IsZero; it must be rejected, not ignored")
			}
			if err := h.validate(); !errors.Is(err, ErrInvalidDisplayHandle) {
				t.Fatalf("validate = %v, want ErrInvalidDisplayHandle", err)
			}
		})
	}
}

func TestCheckSurfaceDisplayMatrix(t *testing.T) {
	const dpy, other = 0x1000, 0x2000
	targets := map[string]SurfaceTargetUnsafe{
		"xlib same":         SurfaceTargetFromXlibWindow(dpy, 1),
		"xlib other":        SurfaceTargetFromXlibWindow(other, 1),
		"wayland same":      SurfaceTargetFromWaylandSurface(dpy, 1),
		"wayland other":     SurfaceTargetFromWaylandSurface(other, 1),
		"hwnd":              SurfaceTargetFromWindowsHWND(dpy, 1),
		"hwnd zero":         SurfaceTargetFromWindowsHWND(0, 1),
		"metal":             SurfaceTargetFromMetalLayer(1),
		"android":           SurfaceTargetFromAndroidNativeWindow(1),
		"headless":          {kind: surfaceTargetHeadless},
		"canvas":            SurfaceTargetFromWebCanvasID(0),
		"xlib nil display":  SurfaceTargetFromXlibWindow(0, 1),
		"wayland nil disp.": SurfaceTargetFromWaylandSurface(0, 1),
	}

	// want lists the targets that must fail with ErrMismatchingDisplayHandle
	// for each instance display; every other target passes the check.
	cases := []struct {
		name     string
		instance DisplayHandle
		want     map[string]bool
	}{
		{"no display", DisplayHandle{}, map[string]bool{}},
		{"xlib display", DisplayHandleFromXlib(dpy), map[string]bool{
			"xlib other": true, "wayland same": true, "wayland other": true,
			"xlib nil display": true, "wayland nil disp.": true,
		}},
		{"wayland display", DisplayHandleFromWayland(dpy), map[string]bool{
			"xlib same": true, "xlib other": true, "wayland other": true,
			"xlib nil display": true, "wayland nil disp.": true,
		}},
	}
	for _, c := range cases {
		for targetName, target := range targets {
			t.Run(c.name+"/"+targetName, func(t *testing.T) {
				err := checkSurfaceDisplay(c.instance, target)
				if c.want[targetName] {
					if !errors.Is(err, ErrMismatchingDisplayHandle) {
						t.Fatalf("checkSurfaceDisplay = %v, want ErrMismatchingDisplayHandle", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("checkSurfaceDisplay = %v, want nil", err)
				}
			})
		}
	}
}

func TestDisplayHandleLegacySurfaceTarget(t *testing.T) {
	const dpy, win = 0x1000, 0x42

	if _, ok := (DisplayHandle{}).legacySurfaceTarget(dpy, win); ok {
		t.Fatal("a zero display routed a legacy surface")
	}

	got, ok := DisplayHandleFromWayland(dpy).legacySurfaceTarget(dpy, win)
	if !ok || got != SurfaceTargetFromWaylandSurface(dpy, win) {
		t.Fatalf("Wayland display routed to %+v (ok=%v), want a Wayland surface target", got, ok)
	}
	got, ok = DisplayHandleFromXlib(dpy).legacySurfaceTarget(dpy, win)
	if !ok || got != SurfaceTargetFromXlibWindow(dpy, win) {
		t.Fatalf("Xlib display routed to %+v (ok=%v), want an Xlib window target", got, ok)
	}
}
