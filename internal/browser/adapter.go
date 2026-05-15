//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"
)

// Adapter wraps a browser GPUAdapter.
//
// Matches Rust wgpu WebAdapter which holds the webgpu_sys::GpuAdapter inner value
// and pre-caches features/limits at construction time.
type Adapter struct {
	// ref_ is the GPUAdapter JavaScript object.
	ref_ js.Value

	// features is the cached GPUSupportedFeatures set.
	features js.Value

	// limits is the cached GPUSupportedLimits object.
	limits js.Value
}

// newAdapter constructs an Adapter from a GPUAdapter js.Value.
// Pre-caches features and limits to avoid repeated property lookups.
func newAdapter(ref js.Value) *Adapter {
	return &Adapter{
		ref_:     ref,
		features: ref.Get("features"),
		limits:   ref.Get("limits"),
	}
}

// RequestDevice requests a logical device from this adapter.
//
// The descriptor parameter is a JS object matching GPUDeviceDescriptor
// (built by convert.go helpers). Pass js.Undefined() for default device.
//
// Matches Rust wgpu WebAdapter::request_device which calls
// inner.request_device_with_descriptor and awaits the promise.
func (a *Adapter) RequestDevice(descriptor js.Value) (*Device, error) {
	var promise js.Value
	if descriptor.IsUndefined() || descriptor.IsNull() {
		promise = a.ref_.Call("requestDevice")
	} else {
		promise = a.ref_.Call("requestDevice", descriptor)
	}

	result, err := AwaitPromise(promise)
	if err != nil {
		return nil, fmt.Errorf("requestDevice: %w", err)
	}

	if result.IsNull() || result.IsUndefined() {
		return nil, ErrDeviceCreationFailed
	}

	return NewDevice(result), nil
}

// Features returns the cached GPUSupportedFeatures js.Value.
// Use convert.ExtractFeatures to convert to gputypes.Features.
func (a *Adapter) Features() js.Value {
	return a.features
}

// Limits returns the cached GPUSupportedLimits js.Value.
// Use convert.ExtractLimits to convert to gputypes.Limits.
func (a *Adapter) Limits() js.Value {
	return a.limits
}

// Ref returns the underlying GPUAdapter js.Value.
func (a *Adapter) Ref() js.Value {
	return a.ref_
}
