//go:build rust

package wgpu

// This file is the public declaration layer over internal/instance on the rust
// build (ADR-070 S12). The implementation moved to internal/instance; what stays
// here is the frozen public Instance type, the creation function and the display
// handle the surface creation glue converts.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/instance"
	"github.com/gogpu/wgpu/internal/surface"
)

// InstanceDescriptor configures instance creation.
// On Rust backend, Backends and Flags are accepted for API compatibility
// but the Rust wgpu-native handles backend selection internally.
type InstanceDescriptor struct {
	Backends gputypes.Backends
	Flags    gputypes.InstanceFlags
	// Display is the native display connection to build the instance on, or
	// zero (Rust wgpu InstanceDescriptor.display).
	//
	// On Linux GLES (EGL) the context is built on it at instance creation.
	// The caller keeps the display open until Instance.Release returns: wgpu
	// cannot keep it alive. With it set, every Xlib or Wayland surface must
	// use the same display, or surface creation fails with
	// ErrMismatchingDisplayHandle. A handle of a nil pointer makes
	// CreateInstance fail with ErrInvalidDisplayHandle.
	//
	// With -tags rust it is not passed to wgpu-native until go-webgpu maps
	// it (WGPUInstanceExtras.displayHandle); the surface check above applies.
	Display DisplayHandle
}

// Instance is the entry point for GPU operations.
// On Rust backend, this wraps go-webgpu/webgpu Instance.
//
// The implementation lives in internal/instance (ADR-070 S12); this type is the
// declaration layer the public API is frozen on. It holds the implementation
// handle and the display handle the surface creation glue converts, both
// written once at construction.
type Instance struct {
	impl    *instance.Instance
	display DisplayHandle // InstanceDescriptor.Display, immutable
}

// CreateInstance creates a new GPU instance.
// If desc is nil, all available backends are used.
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	var display DisplayHandle
	if desc != nil {
		if err := desc.Display.validate(); err != nil {
			return nil, err
		}
		display = desc.Display
	}

	impl, err := instance.Create()
	if err != nil {
		return nil, err
	}
	return &Instance{impl: impl, display: display}, nil
}

// RequestAdapter requests a GPU adapter matching the options.
// If opts is nil, the best available adapter is returned.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	// The descriptor boundary (shape 1): the public options become the core
	// request type and the compatible surface's implementation handle. A nil
	// pointer stays nil so wgpu-native resolves the same defaults it did
	// pre-move.
	var coreOpts *gputypes.RequestAdapterOptions
	var compatible *surface.Surface
	if opts != nil {
		coreOpts = &gputypes.RequestAdapterOptions{
			PowerPreference:      opts.PowerPreference,
			ForceFallbackAdapter: opts.ForceFallbackAdapter,
		}
		if opts.CompatibleSurface != nil {
			compatible = opts.CompatibleSurface.impl
		}
	}

	impl, err := i.impl.RequestAdapter(coreOpts, compatible)
	if err != nil {
		return nil, err
	}
	return &Adapter{impl: impl, instance: i}, nil
}

// isReleased reports whether the instance is released. A nil wrapper is
// released; a wrapper without an implementation (the zero value) is not, which
// is the zero-value answer the build always had.
func (i *Instance) isReleased() bool {
	if i == nil {
		return true
	}
	return i.impl != nil && i.impl.IsReleased()
}

// Release releases the instance. Surfaces must be released explicitly.
func (i *Instance) Release() {
	i.impl.Release()
}
