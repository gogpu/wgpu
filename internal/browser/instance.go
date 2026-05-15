//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"
)

// Instance is the browser WebGPU entry point, wrapping navigator.gpu.
//
// Matches Rust wgpu ContextWebGpu which holds an Option<Gpu>.
type Instance struct {
	// gpu is the navigator.gpu JavaScript object (GPUInstance).
	gpu js.Value
}

// NewInstance creates a new browser WebGPU instance by accessing navigator.gpu.
//
// Returns ErrNavigatorUnavailable if the navigator object is missing, or
// ErrWebGPUNotSupported if navigator.gpu is undefined (browser does not
// support WebGPU or the page is not in a secure context).
func NewInstance() (*Instance, error) {
	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.IsNull() {
		return nil, ErrNavigatorUnavailable
	}

	gpu := navigator.Get("gpu")
	if gpu.IsUndefined() || gpu.IsNull() {
		return nil, ErrWebGPUNotSupported
	}

	return &Instance{gpu: gpu}, nil
}

// RequestAdapter requests a GPU adapter from the browser.
//
// The options parameter is a JS object matching GPURequestAdapterOptions
// (built by convert.go helpers). Pass js.Undefined() for default options.
//
// Returns ErrAdapterNotFound if the browser cannot find a suitable adapter
// (same as navigator.gpu.requestAdapter() returning null).
//
// Matches Rust wgpu ContextWebGpu::request_adapter which calls
// gpu.request_adapter_with_options and awaits the promise.
func (inst *Instance) RequestAdapter(options js.Value) (*Adapter, error) {
	var promise js.Value
	if options.IsUndefined() || options.IsNull() {
		promise = inst.gpu.Call("requestAdapter")
	} else {
		promise = inst.gpu.Call("requestAdapter", options)
	}

	result, err := AwaitPromise(promise)
	if err != nil {
		return nil, fmt.Errorf("requestAdapter: %w", err)
	}

	// requestAdapter returns null when no suitable adapter is found.
	if result.IsNull() || result.IsUndefined() {
		return nil, ErrAdapterNotFound
	}

	return newAdapter(result), nil
}

// GPU returns the underlying navigator.gpu js.Value.
// Exposed for testing and advanced interop scenarios.
func (inst *Instance) GPU() js.Value {
	return inst.gpu
}
