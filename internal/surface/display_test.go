// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"testing"
)

func TestDescribeDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   DisplayHandle
		want string
	}{
		{"zero", DisplayHandle{}, "no display"},
		{"xlib", DisplayHandle{Kind: DisplayKindXlib, Ptr: 0x1000}, "Xlib display 0x1000"},
		{"wayland", DisplayHandle{Kind: DisplayKindWayland, Ptr: 0xab}, "Wayland display 0xab"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.in.Describe(); got != test.want {
				t.Fatalf("Describe() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLegacyTargetByDisplayKind(t *testing.T) {
	const dpy, win = 0x1000, 0x42

	if got, ok := (DisplayHandle{}).LegacyTarget(dpy, win); ok || got != (Target{}) {
		t.Fatalf("zero display routed to %+v (ok=%v), want no target", got, ok)
	}

	wantWayland := Target{Kind: TargetWaylandSurface, Display: dpy, Window: win}
	if got, ok := (DisplayHandle{Kind: DisplayKindWayland}).LegacyTarget(dpy, win); !ok || got != wantWayland {
		t.Fatalf("Wayland display routed to %+v (ok=%v), want %+v", got, ok, wantWayland)
	}

	wantXlib := Target{Kind: TargetXlibWindow, Display: dpy, Window: win}
	if got, ok := (DisplayHandle{Kind: DisplayKindXlib}).LegacyTarget(dpy, win); !ok || got != wantXlib {
		t.Fatalf("Xlib display routed to %+v (ok=%v), want %+v", got, ok, wantXlib)
	}
}

func TestCheckSurfaceDisplayMatrix(t *testing.T) {
	const dpy, other = 0x1000, 0x2000
	targets := map[string]Target{
		"xlib same":         {Kind: TargetXlibWindow, Display: dpy, Window: 1},
		"xlib other":        {Kind: TargetXlibWindow, Display: other, Window: 1},
		"wayland same":      {Kind: TargetWaylandSurface, Display: dpy, Window: 1},
		"wayland other":     {Kind: TargetWaylandSurface, Display: other, Window: 1},
		"hwnd":              {Kind: TargetWindowsHWND, Display: dpy, Window: 1},
		"hwnd zero":         {Kind: TargetWindowsHWND, Window: 1},
		"metal":             {Kind: TargetMetalLayer, Window: 1},
		"android":           {Kind: TargetAndroidNativeWindow, Window: 1},
		"headless":          {Kind: TargetHeadless},
		"canvas":            {Kind: TargetWebCanvasID},
		"xlib nil display":  {Kind: TargetXlibWindow, Window: 1},
		"wayland nil disp.": {Kind: TargetWaylandSurface, Window: 1},
	}

	// want lists the targets that must fail with ErrMismatchingDisplayHandle
	// for each instance display; every other target passes the check.
	cases := []struct {
		name     string
		instance DisplayHandle
		want     map[string]bool
	}{
		{"no display", DisplayHandle{}, map[string]bool{}},
		{"xlib display", DisplayHandle{Kind: DisplayKindXlib, Ptr: dpy}, map[string]bool{
			"xlib other": true, "wayland same": true, "wayland other": true,
			"xlib nil display": true, "wayland nil disp.": true,
		}},
		{"wayland display", DisplayHandle{Kind: DisplayKindWayland, Ptr: dpy}, map[string]bool{
			"xlib same": true, "xlib other": true, "wayland other": true,
			"xlib nil display": true, "wayland nil disp.": true,
		}},
	}
	for _, c := range cases {
		for targetName, target := range targets {
			t.Run(c.name+"/"+targetName, func(t *testing.T) {
				err := CheckSurfaceDisplay(c.instance, target)
				if c.want[targetName] {
					if !errors.Is(err, ErrMismatchingDisplayHandle) {
						t.Fatalf("CheckSurfaceDisplay = %v, want ErrMismatchingDisplayHandle", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("CheckSurfaceDisplay = %v, want nil", err)
				}
			})
		}
	}
}

// TestCheckSurfaceDisplayMessage pins the diagnostic text the public API has
// always returned; the root wrapper must stay byte-identical.
func TestCheckSurfaceDisplayMessage(t *testing.T) {
	err := CheckSurfaceDisplay(
		DisplayHandle{Kind: DisplayKindXlib, Ptr: 0x1000},
		Target{Kind: TargetWaylandSurface, Display: 0x1000, Window: 1},
	)
	want := "wgpu: surface display differs from the instance display: instance Xlib display 0x1000, surface Wayland display 0x1000"
	if err == nil || err.Error() != want {
		t.Fatalf("CheckSurfaceDisplay error = %v, want %q", err, want)
	}
}
