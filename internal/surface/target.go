// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

// The surface-target domain (ADR-070 S13). The public SurfaceTargetUnsafe is a
// defined public type, so this package names the same shape as a
// package-local mirror (R9): the root declaration layer builds it field by
// field at the call boundary and converts a target back the same way. The
// validation rules, the messages and the nil-provider check are the ones the
// public methods ran before the move, unchanged.

import (
	"errors"
	"fmt"
	"reflect"
)

// ErrInvalidSurfaceTarget is returned when a surface target is empty or has
// missing platform handles. The root package binds its public sentinel to this
// value, so errors.Is keeps working through the public API.
var ErrInvalidSurfaceTarget = errors.New("wgpu: invalid surface target")

// TargetKind names the platform resource a raw surface target refers to.
type TargetKind uint8

const (
	// TargetInvalid is the zero value: an empty target.
	TargetInvalid TargetKind = iota
	// TargetHeadless is a surface without a native window.
	TargetHeadless
	// TargetWindowsHWND is a Win32 HWND.
	TargetWindowsHWND
	// TargetXlibWindow is an Xlib Display*/Window pair.
	TargetXlibWindow
	// TargetWaylandSurface is a wl_display*/wl_surface* pair.
	TargetWaylandSurface
	// TargetAndroidNativeWindow is an ANativeWindow*.
	TargetAndroidNativeWindow
	// TargetMetalLayer is a CAMetalLayer*.
	TargetMetalLayer
	// TargetWebCanvasID is a browser canvas identified by its raw handle.
	TargetWebCanvasID
)

// Target is the surface domain's mirror of the public raw target: the target
// kind and the two native handles the kind uses.
type Target struct {
	Kind    TargetKind
	Display uintptr
	Window  uintptr
}

// Validate rejects a target that is empty or misses the platform handles its
// kind needs. A headless target carries no handles, and a zero canvas id
// intentionally selects the first canvas for compatibility.
func (t Target) Validate() error {
	switch t.Kind {
	case TargetHeadless:
		// A headless target carries no platform handles.
	case TargetWindowsHWND:
		if t.Window == 0 {
			return invalidTarget("Win32 HWND is zero")
		}
	case TargetXlibWindow:
		if t.Display == 0 || t.Window == 0 {
			return invalidTarget("Xlib Display or Window is zero")
		}
	case TargetWaylandSurface:
		if t.Display == 0 || t.Window == 0 {
			return invalidTarget("Wayland display or surface is zero")
		}
	case TargetAndroidNativeWindow:
		if t.Window == 0 {
			return invalidTarget("Android ANativeWindow is zero")
		}
	case TargetMetalLayer:
		if t.Window == 0 {
			return invalidTarget("Metal layer is zero")
		}
	case TargetWebCanvasID:
		// Zero intentionally selects the first canvas for compatibility.
	case TargetInvalid:
		return invalidTarget("target is empty")
	default:
		return invalidTarget("target kind is unknown")
	}
	return nil
}

// invalidTarget wraps ErrInvalidSurfaceTarget with the reason.
func invalidTarget(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSurfaceTarget, message)
}

// IsNilTarget reports whether target names no object: a nil interface, or a
// typed nil of a nilable kind. It preserves the public check's reflect
// semantics exactly — pointer, chan, func, interface, map and slice count as
// nil, every other kind does not — because it decides nil-target detection for
// every caller.
func IsNilTarget(target any) bool {
	if target == nil {
		return true
	}
	value := reflect.ValueOf(target)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// TargetFromLegacyHandles maps legacy handles to a typed target. On Linux an
// instance Display decides between Xlib and Wayland; without one, the
// WAYLAND_DISPLAY environment does.
func TargetFromLegacyHandles(goos, waylandDisplay string, display DisplayHandle, displayHandle, windowHandle uintptr) Target {
	switch goos {
	case platformWindows:
		return Target{Kind: TargetWindowsHWND, Display: displayHandle, Window: windowHandle}
	case platformDarwin:
		return Target{Kind: TargetMetalLayer, Window: windowHandle}
	case platformLinux:
		if target, ok := display.LegacyTarget(displayHandle, windowHandle); ok {
			return target
		}
		if waylandDisplay != "" {
			return Target{Kind: TargetWaylandSurface, Display: displayHandle, Window: windowHandle}
		}
		return Target{Kind: TargetXlibWindow, Display: displayHandle, Window: windowHandle}
	case platformAndroid:
		return Target{Kind: TargetAndroidNativeWindow, Window: windowHandle}
	default:
		return Target{Kind: TargetInvalid, Display: displayHandle, Window: windowHandle}
	}
}

// The platform names the legacy mapper switches on.
const (
	platformWindows = "windows"
	platformDarwin  = "darwin"
	platformLinux   = "linux"
	platformAndroid = "android"
)
