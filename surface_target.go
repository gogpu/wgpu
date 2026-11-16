package wgpu

import (
	"errors"
	"fmt"

	// Aliased because SurfaceTargetFromWaylandSurface names its wl_surface
	// parameter "surface", as its public signature always has; the alias keeps
	// the imported package visible without shadowing.
	surfacepkg "github.com/gogpu/wgpu/internal/surface"
)

// ErrInvalidSurfaceTarget is returned when a surface target is empty or has
// missing platform handles. The value is internal/surface's, the package that
// runs the validation, so errors.Is keeps working through the public API.
var ErrInvalidSurfaceTarget = surfacepkg.ErrInvalidSurfaceTarget

// ErrUnsupportedSurfaceTarget is returned when the selected implementation
// cannot create a surface for the supplied platform target kind.
var ErrUnsupportedSurfaceTarget = errors.New("wgpu: unsupported surface target")

// ErrSurfaceDeviceMismatch is returned by Surface.Configure when the Device
// cannot present to the surface. On GLES a Device opened through a surface's
// adapter is bound to that surface's native display: a surface of another
// display needs a Device opened through its own adapter. On Vulkan the Device
// of an adapter requested with no CompatibleSurface is opened on the first
// graphics queue family, which may not support presenting to the surface.
// The backend's detail is wrapped in the error.
var ErrSurfaceDeviceMismatch = errors.New("wgpu: device cannot present to this surface")

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
//
// On GLES a Device opened through the surface's adapter is bound to the
// surface's native display (Xlib Display* or wl_display*), which must outlive
// that Device too. Release surfaces and devices, then close the display, then
// release the Instance; or release the Instance before closing the display.
// Adapters are not part of this rule. When the Instance has a Display
// (InstanceDescriptor.Display), an Xlib or Wayland target must use it, or
// surface creation fails with ErrMismatchingDisplayHandle.
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

// targetMirror builds the surface domain's view of the public raw target: the
// destination cannot name this type, so the root declaration layer converts at
// the call boundary, field by field (ADR-070 §15.6, R9).
func targetMirror(t SurfaceTargetUnsafe) surfacepkg.Target {
	return surfacepkg.Target{Kind: surfacepkg.TargetKind(t.kind), Display: t.displayHandle, Window: t.windowHandle}
}

// validate rejects a target that is empty or misses the platform handles its
// kind needs. The rules live in internal/surface; this is the public
// declaration layer.
func (t SurfaceTargetUnsafe) validate() error {
	return targetMirror(t).Validate()
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

// isNilSurfaceTarget reports whether target names no object: a nil interface,
// or a typed nil of a nilable kind. The reflect semantics live in
// internal/surface (which field kinds count as nil); this is the public
// declaration layer.
func isNilSurfaceTarget(target SurfaceTarget) bool {
	return surfacepkg.IsNilTarget(target)
}
