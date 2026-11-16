//go:build js && wasm

package wgpu

// This file is the public declaration layer over internal/instance on the
// browser build (ADR-070 S12). The implementation moved to internal/instance;
// what stays here is the frozen public Instance type, the creation function and
// the canvas creation glue, which names the public target types and js.Value.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/instance"
)

// InstanceDescriptor configures instance creation.
// On browser, Backends and Flags are accepted for API compatibility but
// ignored — the browser has exactly one WebGPU backend.
type InstanceDescriptor struct {
	Backends gputypes.Backends
	Flags    gputypes.InstanceFlags
	// Display is the native display connection to build the instance on, or
	// zero (Rust wgpu InstanceDescriptor.display). On Linux GLES (EGL) the
	// context is built on it at instance creation, and Xlib or Wayland
	// surfaces must use the same display. The browser has no native display:
	// it is ignored here.
	Display DisplayHandle
}

// Instance is the entry point for GPU operations.
// On browser, this wraps navigator.gpu via internal/browser.Instance.
//
// The implementation lives in internal/instance (ADR-070 S12); this type is the
// declaration layer the public API is frozen on.
//
// released is the public zero-value contract: a wrapper without an
// implementation cannot record its release in it, and the browser build's zero
// value answers its HAL kind until it is released.
type Instance struct {
	impl     *instance.Instance
	released bool
}

// CreateInstance creates a new GPU instance.
// On browser, this accesses navigator.gpu. The desc parameter is accepted
// for API compatibility but Backends/Flags are ignored (browser has one backend).
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	impl, err := instance.Create()
	if err != nil {
		return nil, err
	}
	return &Instance{impl: impl}, nil
}

// RequestAdapter requests a GPU adapter matching the options.
// If opts is nil, the best available adapter is returned.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	if i.released {
		return nil, ErrReleased
	}

	// The descriptor boundary (shape 1): the public options become the core
	// request type. A nil pointer stays nil so the browser path builds
	// js.Undefined exactly as it did pre-move.
	var coreOpts *gputypes.RequestAdapterOptions
	if opts != nil {
		coreOpts = &gputypes.RequestAdapterOptions{
			PowerPreference:      opts.PowerPreference,
			ForceFallbackAdapter: opts.ForceFallbackAdapter,
		}
	}

	impl, err := i.impl.RequestAdapter(coreOpts)
	if err != nil {
		return nil, err
	}
	return &Adapter{impl: impl}, nil
}

// isReleased reports whether the instance is released. A nil wrapper is
// released; a wrapper without an implementation (the zero value) is not, which
// is the zero-value answer the build always had.
func (i *Instance) isReleased() bool {
	if i == nil || i.released {
		return true
	}
	return i.impl != nil && i.impl.IsReleased()
}

// Release releases the instance. Surfaces must be released explicitly.
func (i *Instance) Release() {
	i.released = true
	i.impl.Release()
}
