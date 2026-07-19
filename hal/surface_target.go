//go:build !(js && wasm)

package hal

import (
	"errors"
	"fmt"
)

// ErrUnsupportedSurfaceTarget indicates that a HAL backend cannot create a
// surface for the supplied platform target kind.
var ErrUnsupportedSurfaceTarget = errors.New("hal: unsupported surface target")

// SurfaceTargetKind identifies the native window-system representation carried
// by a SurfaceTarget.
type SurfaceTargetKind uint8

const (
	SurfaceTargetInvalid SurfaceTargetKind = iota
	SurfaceTargetHeadless
	SurfaceTargetWindowsHWND
	SurfaceTargetXlibWindow
	SurfaceTargetWaylandSurface
	SurfaceTargetAndroidNativeWindow
	SurfaceTargetMetalLayer
)

// SurfaceTarget is the typed raw-window contract passed from core to HAL.
// DisplayHandle is unused for Android and Metal. WindowHandle is HWND, Xlib
// Window, wl_surface*, ANativeWindow*, or CAMetalLayer* according to Kind.
// HAL never owns these raw handles; they must outlive the created Surface.
// Headless is a Go software/noop extension and carries no handles.
type SurfaceTarget struct {
	Kind          SurfaceTargetKind
	DisplayHandle uintptr
	WindowHandle  uintptr
}

// RequireKind rejects a target intended for a different window system.
func (t SurfaceTarget) RequireKind(expected SurfaceTargetKind) error {
	if t.Kind != expected {
		return fmt.Errorf("%w: got %s, backend requires %s", ErrUnsupportedSurfaceTarget, t.Kind, expected)
	}
	return nil
}

// String returns a stable diagnostic name for the target kind.
func (k SurfaceTargetKind) String() string {
	switch k {
	case SurfaceTargetHeadless:
		return "headless"
	case SurfaceTargetWindowsHWND:
		return "Win32 HWND"
	case SurfaceTargetXlibWindow:
		return "Xlib window"
	case SurfaceTargetWaylandSurface:
		return "Wayland surface"
	case SurfaceTargetAndroidNativeWindow:
		return "Android native window"
	case SurfaceTargetMetalLayer:
		return "Metal layer"
	case SurfaceTargetInvalid:
		return "invalid"
	default:
		return fmt.Sprintf("SurfaceTargetKind(%d)", uint8(k))
	}
}
