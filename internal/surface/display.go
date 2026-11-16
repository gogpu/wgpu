// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

// The display domain of surface creation (ADR-070 S13). The public
// DisplayHandle is a defined public type, so this package names the same shape
// as a package-local mirror (R9): the root declaration layer builds it field by
// field at the call boundary and converts a target back the same way. Every
// rule below is the one the public methods ran before the move (Rust wgpu-core
// create_surface, MismatchingDisplayHandle), unchanged.

import (
	"errors"
	"fmt"
)

// ErrMismatchingDisplayHandle is returned by surface creation when the
// instance has a Display and an Xlib or Wayland target uses another display.
// The root package binds its public sentinel to this value, so errors.Is keeps
// working through the public API.
var ErrMismatchingDisplayHandle = errors.New("wgpu: surface display differs from the instance display")

// DisplayKind names the window-system connection a display handle refers to.
type DisplayKind uint8

const (
	// DisplayKindNone is the zero value: no display.
	DisplayKindNone DisplayKind = iota
	// DisplayKindXlib is an Xlib Display*.
	DisplayKindXlib
	// DisplayKindWayland is a wl_display*.
	DisplayKindWayland
)

// DisplayHandle is the surface domain's mirror of the public display handle:
// the connection kind and the native pointer. Comparable with ==.
type DisplayHandle struct {
	Kind DisplayKind
	Ptr  uintptr
}

// Describe returns a diagnostic form of the handle for error messages. It is
// not a String method: the public surface of the handle is IsZero only
// (ADR-077).
func (h DisplayHandle) Describe() string {
	switch h.Kind {
	case DisplayKindXlib:
		return fmt.Sprintf("Xlib display 0x%x", h.Ptr)
	case DisplayKindWayland:
		return fmt.Sprintf("Wayland display 0x%x", h.Ptr)
	default:
		return "no display"
	}
}

// LegacyTarget maps the legacy CreateSurface(display, window) handles by the
// display's kind, so a set Display, not the WAYLAND_DISPLAY environment,
// decides between Xlib and Wayland. ok is false without one.
func (h DisplayHandle) LegacyTarget(display, window uintptr) (Target, bool) {
	switch h.Kind {
	case DisplayKindXlib:
		return Target{Kind: TargetXlibWindow, Display: display, Window: window}, true
	case DisplayKindWayland:
		return Target{Kind: TargetWaylandSurface, Display: display, Window: window}, true
	default:
		return Target{}, false
	}
}

// CheckSurfaceDisplay applies the instance display to a surface target, the
// same way on every build (Rust wgpu-core create_surface,
// MismatchingDisplayHandle). With a display set, an Xlib or Wayland target
// must use the same kind and pointer; our targets carry no screen, so the
// screen Rust also compares is not part of the check. Other target kinds
// (HWND, whose display handle is the hinstance, Metal, Android, headless,
// canvas) have no display to compare. Without a display, nothing is checked.
func CheckSurfaceDisplay(instance DisplayHandle, target Target) error {
	if instance.Kind == DisplayKindNone {
		return nil
	}
	var kind DisplayKind
	switch target.Kind {
	case TargetXlibWindow:
		kind = DisplayKindXlib
	case TargetWaylandSurface:
		kind = DisplayKindWayland
	default:
		return nil
	}
	mirror := DisplayHandle{Kind: kind, Ptr: target.Display}
	if mirror != instance {
		return fmt.Errorf("%w: instance %s, surface %s", ErrMismatchingDisplayHandle,
			instance.Describe(), mirror.Describe())
	}
	return nil
}
