// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"testing"
)

func TestTargetValidate(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		want   string
	}{
		{"headless", Target{Kind: TargetHeadless}, ""},
		{"hwnd", Target{Kind: TargetWindowsHWND, Window: 1}, ""},
		{"hwnd zero", Target{Kind: TargetWindowsHWND}, "wgpu: invalid surface target: Win32 HWND is zero"},
		{"xlib", Target{Kind: TargetXlibWindow, Display: 1, Window: 2}, ""},
		{"xlib nil display", Target{Kind: TargetXlibWindow, Window: 2}, "wgpu: invalid surface target: Xlib Display or Window is zero"},
		{"xlib nil window", Target{Kind: TargetXlibWindow, Display: 1}, "wgpu: invalid surface target: Xlib Display or Window is zero"},
		{"wayland", Target{Kind: TargetWaylandSurface, Display: 3, Window: 4}, ""},
		{"wayland nil display", Target{Kind: TargetWaylandSurface, Window: 4}, "wgpu: invalid surface target: Wayland display or surface is zero"},
		{"wayland nil surface", Target{Kind: TargetWaylandSurface, Display: 3}, "wgpu: invalid surface target: Wayland display or surface is zero"},
		{"android", Target{Kind: TargetAndroidNativeWindow, Window: 5}, ""},
		{"android zero", Target{Kind: TargetAndroidNativeWindow}, "wgpu: invalid surface target: Android ANativeWindow is zero"},
		{"metal", Target{Kind: TargetMetalLayer, Window: 6}, ""},
		{"metal zero", Target{Kind: TargetMetalLayer}, "wgpu: invalid surface target: Metal layer is zero"},
		{"canvas", Target{Kind: TargetWebCanvasID}, ""},
		{"empty", Target{}, "wgpu: invalid surface target: target is empty"},
		{"unknown", Target{Kind: TargetKind(255), Window: 1}, "wgpu: invalid surface target: target kind is unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.target.Validate()
			if test.want == "" {
				if err != nil {
					t.Fatalf("Validate(%+v) = %v, want nil", test.target, err)
				}
				return
			}
			if err == nil || err.Error() != test.want {
				t.Fatalf("Validate(%+v) = %v, want %q", test.target, err, test.want)
			}
			if !errors.Is(err, ErrInvalidSurfaceTarget) {
				t.Fatalf("Validate(%+v) = %v, want ErrInvalidSurfaceTarget", test.target, err)
			}
		})
	}
}

// TestIsNilTarget pins the reflect semantics the public nil-provider check has
// always had: a nil interface and a typed nil of a nilable kind count as nil,
// every other value does not.
func TestIsNilTarget(t *testing.T) {
	type provider struct{ display uintptr }
	var typedNil *provider
	var nilMap map[string]int
	var nilSlice []int
	var nilChan chan int
	var nilFunc func()

	tests := []struct {
		name string
		in   any
		want bool
	}{
		{"nil interface", nil, true},
		{"typed nil pointer", typedNil, true},
		{"non-nil pointer", &provider{display: 1}, false},
		{"empty struct value", provider{}, false},
		{"nil map", nilMap, true},
		{"non-nil map", map[string]int{}, false},
		{"nil slice", nilSlice, true},
		{"non-nil slice", []int{}, false},
		{"nil chan", nilChan, true},
		{"nil func", nilFunc, true},
		{"int", 42, false},
		{"string", "x", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNilTarget(test.in); got != test.want {
				t.Fatalf("IsNilTarget(%v) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestTargetFromLegacyHandles(t *testing.T) {
	const dpy, win = 0x1000, 0x42
	xlib := DisplayHandle{Kind: DisplayKindXlib, Ptr: dpy}
	wayland := DisplayHandle{Kind: DisplayKindWayland, Ptr: dpy}

	tests := []struct {
		name           string
		goos           string
		waylandDisplay string
		display        DisplayHandle
		want           Target
	}{
		{"windows", platformWindows, "", wayland, Target{Kind: TargetWindowsHWND, Display: dpy, Window: win}},
		{"darwin", platformDarwin, "", xlib, Target{Kind: TargetMetalLayer, Window: win}},
		{"linux wayland display", platformLinux, "", wayland, Target{Kind: TargetWaylandSurface, Display: dpy, Window: win}},
		{"linux xlib display under WAYLAND_DISPLAY", platformLinux, "wayland-0", xlib, Target{Kind: TargetXlibWindow, Display: dpy, Window: win}},
		{"linux no display follows WAYLAND_DISPLAY", platformLinux, "wayland-0", DisplayHandle{}, Target{Kind: TargetWaylandSurface, Display: dpy, Window: win}},
		{"linux no display without WAYLAND_DISPLAY", platformLinux, "", DisplayHandle{}, Target{Kind: TargetXlibWindow, Display: dpy, Window: win}},
		{"android", platformAndroid, "", xlib, Target{Kind: TargetAndroidNativeWindow, Window: win}},
		{"unknown platform", "freebsd", "", wayland, Target{Kind: TargetInvalid, Display: dpy, Window: win}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := TargetFromLegacyHandles(test.goos, test.waylandDisplay, test.display, dpy, win)
			if got != test.want {
				t.Fatalf("TargetFromLegacyHandles = %+v, want %+v", got, test.want)
			}
		})
	}
}
