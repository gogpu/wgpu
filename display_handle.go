package wgpu

import (
	"errors"
	"fmt"
)

// ErrInvalidDisplayHandle is returned by CreateInstance for a DisplayHandle
// built from a nil display pointer.
var ErrInvalidDisplayHandle = errors.New("wgpu: invalid display handle")

// ErrMismatchingDisplayHandle is returned by surface creation when the
// instance has a Display and an Xlib or Wayland target uses another display.
var ErrMismatchingDisplayHandle = errors.New("wgpu: surface display differs from the instance display")

type displayKind uint8

const (
	displayKindNone displayKind = iota
	displayKindXlib
	displayKindWayland
)

// DisplayHandle is a native display connection for InstanceDescriptor.Display.
// The zero value means no display. Comparable with ==.
//
// The handle only names the connection: wgpu cannot keep it alive. The caller
// keeps the display open until Instance.Release returns.
type DisplayHandle struct {
	kind displayKind
	ptr  uintptr
}

// DisplayHandleFromXlib returns the handle of an Xlib Display*.
func DisplayHandleFromXlib(display uintptr) DisplayHandle {
	return DisplayHandle{kind: displayKindXlib, ptr: display}
}

// DisplayHandleFromWayland returns the handle of a wl_display*.
func DisplayHandleFromWayland(display uintptr) DisplayHandle {
	return DisplayHandle{kind: displayKindWayland, ptr: display}
}

// IsZero reports whether h names no display.
func (h DisplayHandle) IsZero() bool {
	return h.kind == displayKindNone
}

// validate rejects a handle built from a nil display pointer. The zero
// handle is valid: no display.
func (h DisplayHandle) validate() error {
	if !h.IsZero() && h.ptr == 0 {
		return fmt.Errorf("%w: nil display pointer", ErrInvalidDisplayHandle)
	}
	return nil
}

// checkSurfaceDisplay applies the instance display to a surface target, the
// same way on every build (Rust wgpu-core create_surface,
// MismatchingDisplayHandle). With a display set, an Xlib or Wayland target
// must use the same kind and pointer; our targets carry no screen, so the
// screen Rust also compares is not part of the check. Other target kinds
// (HWND, whose display handle is the hinstance, Metal, Android, headless,
// canvas) have no display to compare. Without a display, nothing is checked.
func checkSurfaceDisplay(instance DisplayHandle, target SurfaceTargetUnsafe) error {
	if instance.IsZero() {
		return nil
	}
	var kind displayKind
	switch target.kind {
	case surfaceTargetXlibWindow:
		kind = displayKindXlib
	case surfaceTargetWaylandSurface:
		kind = displayKindWayland
	default:
		return nil
	}
	if (DisplayHandle{kind: kind, ptr: target.displayHandle}) != instance {
		return fmt.Errorf("%w: instance %s, surface %s", ErrMismatchingDisplayHandle,
			instance.describe(), DisplayHandle{kind: kind, ptr: target.displayHandle}.describe())
	}
	return nil
}

// legacySurfaceTarget maps the legacy CreateSurface(display, window) handles
// by the instance display's kind, so a set Display, not the WAYLAND_DISPLAY
// environment, decides between Xlib and Wayland. ok is false without one.
func (h DisplayHandle) legacySurfaceTarget(displayHandle, windowHandle uintptr) (target SurfaceTargetUnsafe, ok bool) {
	switch h.kind {
	case displayKindXlib:
		return SurfaceTargetFromXlibWindow(displayHandle, windowHandle), true
	case displayKindWayland:
		return SurfaceTargetFromWaylandSurface(displayHandle, windowHandle), true
	default:
		return SurfaceTargetUnsafe{}, false
	}
}

// describe returns a diagnostic form of the handle for error messages. It is
// not String: the public surface of DisplayHandle is IsZero only (ADR-077).
func (h DisplayHandle) describe() string {
	switch h.kind {
	case displayKindXlib:
		return fmt.Sprintf("Xlib display 0x%x", h.ptr)
	case displayKindWayland:
		return fmt.Sprintf("Wayland display 0x%x", h.ptr)
	default:
		return "no display"
	}
}
