package wgpu

import (
	"errors"
	"fmt"

	"github.com/gogpu/wgpu/internal/surface"
)

// ErrInvalidDisplayHandle is returned by CreateInstance for a DisplayHandle
// built from a nil display pointer.
var ErrInvalidDisplayHandle = errors.New("wgpu: invalid display handle")

// ErrMismatchingDisplayHandle is returned by surface creation when the
// instance has a Display and an Xlib or Wayland target uses another display.
// The value is internal/surface's, the package that runs the check, so
// errors.Is keeps working through the public API.
var ErrMismatchingDisplayHandle = surface.ErrMismatchingDisplayHandle

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

// displayMirror builds the surface domain's view of the public handle: the
// destination cannot name this type, so the root declaration layer converts at
// the call boundary, field by field (ADR-070 §15.6, R9).
func displayMirror(h DisplayHandle) surface.DisplayHandle {
	return surface.DisplayHandle{Kind: surface.DisplayKind(h.kind), Ptr: h.ptr}
}

// checkSurfaceDisplay applies the instance display to a surface target, the
// same way on every build (Rust wgpu-core create_surface,
// MismatchingDisplayHandle). With a display set, an Xlib or Wayland target
// must use the same kind and pointer; our targets carry no screen, so the
// screen Rust also compares is not part of the check. Other target kinds
// (HWND, whose display handle is the hinstance, Metal, Android, headless,
// canvas) have no display to compare. Without a display, nothing is checked.
// The rule lives in internal/surface; this is the public declaration layer.
func checkSurfaceDisplay(instance DisplayHandle, target SurfaceTargetUnsafe) error {
	return surface.CheckSurfaceDisplay(displayMirror(instance), targetMirror(target))
}

// legacySurfaceTarget maps the legacy CreateSurface(display, window) handles
// by the instance display's kind, so a set Display, not the WAYLAND_DISPLAY
// environment, decides between Xlib and Wayland. ok is false without one. The
// mapping lives in internal/surface; this is the public declaration layer.
func (h DisplayHandle) legacySurfaceTarget(displayHandle, windowHandle uintptr) (target SurfaceTargetUnsafe, ok bool) {
	mirror, ok := displayMirror(h).LegacyTarget(displayHandle, windowHandle)
	return SurfaceTargetUnsafe{
		kind:          surfaceTargetKind(mirror.Kind),
		displayHandle: mirror.Display,
		windowHandle:  mirror.Window,
	}, ok
}
