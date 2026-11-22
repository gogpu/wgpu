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

// EnumerateAdapters returns every adapter the instance knows, in registration
// order: the backends' order, then each backend's enumeration order.
// RequestAdapter ranks these candidates by power preference and may pick a
// different one, so take a specific element when a specific adapter is wanted.
// Each element is a live *Adapter: read Info, Features, Limits and
// DownlevelCapabilities, or call RequestDevice on the chosen one (gogpu #360).
//
// The instance's backend mask decides what exists: software adapters appear
// only when BackendsSoftware was requested explicitly (ADR-083 §2.3), exactly
// as for RequestAdapter, and a ForceFallbackAdapter request does not add them.
//
// The result is a snapshot: the set is fixed at instance creation, plus the
// first backend enumeration that runs lazily inside the first RequestAdapter
// call. GPUs added to the system later do not appear. An adapter that
// disappears afterwards keeps its wrapper; RequestDevice on it fails with an
// error, never panics.
//
// The adapters are owned by the instance: Release on a returned handle marks
// that wrapper only, and the adapter itself lives until Instance.Release.
//
// The result is empty, never an error: a system without an adapter is a
// normal state.
//
// GLES enumerates lazily and once per instance: its adapter enters the list
// when the first RequestAdapter call runs, and only that call can hand the
// backend a surface — pass the compatible surface on it when the GLES path
// must open. Without one, a machine whose EGL needs a window yields the
// non-openable placeholder, which is never enumerated here and consumes the
// backend's one-shot enumeration for later surface requests.
func (i *Instance) EnumerateAdapters() []*Adapter {
	if i.isReleased() {
		return nil
	}
	return instance.ProjectAdapters(i.impl.EnumerateAdapters(), i.wrapAdapter)
}

// wrapAdapter is the projection EnumerateAdapters builds its list with: a
// method value, not a closure, so the public method stays a declaration-layer
// delegation (ADR-070 §15.1).
func (i *Instance) wrapAdapter(impl *instance.Adapter) *Adapter {
	return &Adapter{impl: impl, instance: i}
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
