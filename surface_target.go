package wgpu

import (
	"errors"
	"fmt"
	"reflect"
)

// ErrInvalidSurfaceTarget is returned when a surface target is empty or has
// missing platform handles.
var ErrInvalidSurfaceTarget = errors.New("wgpu: invalid surface target")

// ErrUnsupportedSurfaceTarget is returned when the selected implementation
// cannot create a surface for the supplied platform target kind.
var ErrUnsupportedSurfaceTarget = errors.New("wgpu: unsupported surface target")

// SurfaceTarget provides the raw platform target used to create a Surface.
//
// CreateSurfaceFromTarget samples SurfaceTarget once and retains the provider
// until the returned Surface is released. Retaining the provider keeps its Go
// ownership graph alive; the provider remains responsible for ensuring that
// the underlying native display and window stay valid for the same lifetime.
//
// This is the Go equivalent of Rust wgpu's safe SurfaceTarget path.
type SurfaceTarget interface {
	SurfaceTarget() (SurfaceTargetUnsafe, error)
}

type surfaceTargetKind uint8

const (
	surfaceTargetInvalid surfaceTargetKind = iota
	surfaceTargetHeadless
	surfaceTargetWindowsHWND
	surfaceTargetXlibWindow
	surfaceTargetWaylandSurface
	surfaceTargetAndroidNativeWindow
	surfaceTargetMetalLayer
	surfaceTargetWebCanvasID
)

// SurfaceTargetUnsafe identifies raw platform handles for surface creation.
// Construct values with one of the SurfaceTargetFrom* functions.
//
// No ownership source is retained for an unsafe target. The caller must keep
// every referenced native object valid until the returned Surface is released.
// This is the Go equivalent of Rust wgpu's SurfaceTargetUnsafe::RawHandle.
type SurfaceTargetUnsafe struct {
	kind          surfaceTargetKind
	displayHandle uintptr
	windowHandle  uintptr
}

// HeadlessSurfaceTarget requests a surface without a native window.
//
// The Pure-Go software backend implements this Go-specific extension. It owns
// no native handles, so the zero value is ready for use with
// Instance.CreateSurfaceFromTarget. Rust and browser implementations reject
// this target with ErrUnsupportedSurfaceTarget.
type HeadlessSurfaceTarget struct{}

// SurfaceTarget returns the raw target used by the Pure-Go software backend.
func (HeadlessSurfaceTarget) SurfaceTarget() (SurfaceTargetUnsafe, error) {
	return SurfaceTargetUnsafe{kind: surfaceTargetHeadless}, nil
}

// SurfaceTargetFromWindowsHWND returns a raw Win32 surface target.
// hinstance may be zero when the backend can resolve the current module.
func SurfaceTargetFromWindowsHWND(hinstance, hwnd uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetUnsafe{
		kind:          surfaceTargetWindowsHWND,
		displayHandle: hinstance,
		windowHandle:  hwnd,
	}
}

// SurfaceTargetFromXlibWindow returns a raw Xlib Display*/Window target.
func SurfaceTargetFromXlibWindow(display, window uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetUnsafe{
		kind:          surfaceTargetXlibWindow,
		displayHandle: display,
		windowHandle:  window,
	}
}

// SurfaceTargetFromWaylandSurface returns a raw wl_display*/wl_surface* target.
func SurfaceTargetFromWaylandSurface(display, surface uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetUnsafe{
		kind:          surfaceTargetWaylandSurface,
		displayHandle: display,
		windowHandle:  surface,
	}
}

// SurfaceTargetFromAndroidNativeWindow returns a raw ANativeWindow* target.
// Android has no meaningful display handle for this operation.
func SurfaceTargetFromAndroidNativeWindow(window uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetUnsafe{
		kind:         surfaceTargetAndroidNativeWindow,
		windowHandle: window,
	}
}

// SurfaceTargetFromMetalLayer returns a raw CAMetalLayer* target.
func SurfaceTargetFromMetalLayer(layer uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetUnsafe{
		kind:         surfaceTargetMetalLayer,
		windowHandle: layer,
	}
}

// SurfaceTargetFromWebCanvasID returns a browser canvas target identified by
// its data-raw-handle attribute. ID zero retains the legacy behavior of using
// the first canvas element in the document.
func SurfaceTargetFromWebCanvasID(id uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetUnsafe{
		kind:         surfaceTargetWebCanvasID,
		windowHandle: id,
	}
}

func (t SurfaceTargetUnsafe) validate() error {
	switch t.kind {
	case surfaceTargetHeadless:
		// A headless target carries no platform handles.
	case surfaceTargetWindowsHWND:
		if t.windowHandle == 0 {
			return invalidSurfaceTarget("Win32 HWND is zero")
		}
	case surfaceTargetXlibWindow:
		if t.displayHandle == 0 || t.windowHandle == 0 {
			return invalidSurfaceTarget("Xlib Display or Window is zero")
		}
	case surfaceTargetWaylandSurface:
		if t.displayHandle == 0 || t.windowHandle == 0 {
			return invalidSurfaceTarget("Wayland display or surface is zero")
		}
	case surfaceTargetAndroidNativeWindow:
		if t.windowHandle == 0 {
			return invalidSurfaceTarget("Android ANativeWindow is zero")
		}
	case surfaceTargetMetalLayer:
		if t.windowHandle == 0 {
			return invalidSurfaceTarget("Metal layer is zero")
		}
	case surfaceTargetWebCanvasID:
		// Zero intentionally selects the first canvas for compatibility.
	case surfaceTargetInvalid:
		return invalidSurfaceTarget("target is empty")
	default:
		return invalidSurfaceTarget("target kind is unknown")
	}
	return nil
}

func invalidSurfaceTarget(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSurfaceTarget, message)
}

func resolveSurfaceTarget(target SurfaceTarget) (SurfaceTargetUnsafe, error) {
	if isNilSurfaceTarget(target) {
		return SurfaceTargetUnsafe{}, invalidSurfaceTarget("provider is nil")
	}

	rawTarget, err := target.SurfaceTarget()
	if err != nil {
		return SurfaceTargetUnsafe{}, fmt.Errorf("wgpu: get surface target: %w", err)
	}
	if err := rawTarget.validate(); err != nil {
		return SurfaceTargetUnsafe{}, err
	}
	return rawTarget, nil
}

func isNilSurfaceTarget(target SurfaceTarget) bool {
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
