//go:build !rust && !(js && wasm)

package wgpu

// This file is the public declaration layer over internal/instance on the
// native build (ADR-070 S12). The implementation moved to internal/instance;
// what stays here is the frozen public Instance type, the creation function, the
// descriptor boundary and the display handle the surface creation glue converts.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/instance"
	"github.com/gogpu/wgpu/internal/surface"
)

// InstanceDescriptor configures instance creation.
type InstanceDescriptor struct {
	Backends gputypes.Backends
	// Flags controls instance features like debug layers and validation.
	// Use gputypes.InstanceFlagsDebug to enable GPU debug layer.
	Flags gputypes.InstanceFlags
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
	// Ignored by Vulkan, DX12, Metal, the software backend, WGL, the browser,
	// and -tags rust until go-webgpu maps it.
	Display DisplayHandle
}

// Instance is the entry point for GPU operations.
//
// Instance methods are safe for concurrent use, except Release() which
// must not be called concurrently with other methods.
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
	var gpuDesc *gputypes.InstanceDescriptor
	var display DisplayHandle
	if desc != nil {
		if err := desc.Display.validate(); err != nil {
			return nil, err
		}
		display = desc.Display
		d := gputypes.DefaultInstanceDescriptor()
		d.Backends = desc.Backends
		d.Flags = desc.Flags
		gpuDesc = &d
	}

	return &Instance{impl: instance.Create(gpuDesc, display.halDisplay()), display: display}, nil
}

// RequestAdapter requests a GPU adapter matching the options.
// If opts is nil, the best available adapter is returned.
//
// When opts.CompatibleSurface is set, backends that require a surface for
// adapter enumeration (GLES/OpenGL) will perform deferred enumeration using
// the surface's GL context. This follows the WebGPU spec pattern where
// requestAdapter accepts a compatible surface hint.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	if i.isReleased() {
		return nil, ErrReleased
	}

	// The descriptor boundary (shape 1): the public options become the core
	// request type and the compatible surface's implementation handle. A nil
	// pointer stays nil so core resolves the same defaults it did pre-move.
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

// isReleased reports whether the instance is released. A nil wrapper, or one
// without an implementation (the zero value), answers released.
func (i *Instance) isReleased() bool {
	if i == nil || i.impl == nil {
		return true
	}
	return i.impl.IsReleased()
}

// Release releases the instance and all associated resources.
func (i *Instance) Release() {
	if i == nil {
		return
	}
	i.impl.Release()
}
