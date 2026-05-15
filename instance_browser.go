//go:build js && wasm

package wgpu

import (
	"syscall/js"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
)

// InstanceDescriptor configures instance creation.
// On browser, Backends and Flags are accepted for API compatibility but
// ignored — the browser has exactly one WebGPU backend.
type InstanceDescriptor struct {
	Backends Backends
	Flags    gputypes.InstanceFlags
}

// Instance is the entry point for GPU operations.
// On browser, this wraps navigator.gpu via internal/browser.Instance.
type Instance struct {
	browser  *browser.Instance
	released bool
}

// CreateInstance creates a new GPU instance.
// On browser, this accesses navigator.gpu. The desc parameter is accepted
// for API compatibility but Backends/Flags are ignored (browser has one backend).
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	bi, err := browser.NewInstance()
	if err != nil {
		return nil, err
	}
	return &Instance{browser: bi}, nil
}

// RequestAdapter requests a GPU adapter matching the options.
// If opts is nil, the best available adapter is returned.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	if i.released {
		return nil, ErrReleased
	}

	// Build JS options object from Go types.
	var jsOpts js.Value
	if opts != nil {
		jsOpts = browser.BuildRequestAdapterOptions(opts.PowerPreference, opts.ForceFallbackAdapter)
	} else {
		jsOpts = js.Undefined()
	}

	ba, err := i.browser.RequestAdapter(jsOpts)
	if err != nil {
		return nil, err
	}

	// Extract features and limits from the JS adapter.
	features := browser.ExtractFeatures(ba.Features())
	limits := browser.ExtractLimits(ba.Limits())

	// WebGPU browser API does not expose detailed adapter info.
	// Return minimal info matching Rust wgpu's WebAdapter::get_info().
	info := AdapterInfo{
		Name: "WebGPU Adapter",
	}

	return &Adapter{
		browser:  ba,
		info:     info,
		features: features,
		limits:   limits,
	}, nil
}

// CreateSurface and CreateSurfaceFromCanvas are defined in surface_browser.go.

// Release releases the instance and all associated resources.
func (i *Instance) Release() {
	if i.released {
		return
	}
	i.released = true
}
