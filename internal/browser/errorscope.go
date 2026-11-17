//go:build js && wasm

package browser

import (
	"errors"
	"fmt"
)

// The browser build's error scopes (error-scope reshape, stage A4): the JS
// device's own pushErrorScope/popErrorScope, with the JS error constructor
// names mapped onto our category vocabulary. The root declaration layer
// converts GPUErrorInfo into the public GPUError.

// ErrorFilter mirrors the WebGPU error-scope filters on the browser build.
// core's enum is excluded by its own build tag, so the browser carries its own
// values with the same meaning; the root layer converts.
type ErrorFilter int

const (
	// ErrorFilterValidation captures validation errors.
	ErrorFilterValidation ErrorFilter = iota
	// ErrorFilterOutOfMemory captures out-of-memory errors.
	ErrorFilterOutOfMemory
	// ErrorFilterInternal captures internal errors.
	ErrorFilterInternal
)

// String returns a human-readable name for the error filter.
func (f ErrorFilter) String() string {
	switch f {
	case ErrorFilterValidation:
		return "Validation"
	case ErrorFilterOutOfMemory:
		return "OutOfMemory"
	case ErrorFilterInternal:
		return "Internal"
	}
	return fmt.Sprintf("ErrorFilter(%d)", int(f))
}

// ErrErrorScopeEmpty is the browser build's empty-stack answer: the JS
// popErrorScope promise rejects with an OperationError, which maps here. The
// root package binds this value as wgpu.ErrErrorScopeEmpty (per-build identity).
var ErrErrorScopeEmpty = errors.New("wgpu: PopErrorScope: error scope stack is empty (no matching PushErrorScope)")

// Error-scope category sentinels: GPUError.Unwrap answers with these on the
// js/wasm build, so errors.Is classifies a captured error. They live here so
// the public Unwrap is one delegation with no logic (the public-layer gate's
// rule); the root names bind these values.
var (
	// ErrValidation is the category identity of a captured validation error.
	ErrValidation = errors.New("wgpu: validation error")
	// ErrInternal is the category identity of a captured internal error.
	ErrInternal = errors.New("wgpu: internal error")
)

// ErrOutOfMemory is the category identity for a captured out-of-memory error;
// the root package binds wgpu.ErrOutOfMemory to this value (per-build identity).
var ErrOutOfMemory = errors.New("wgpu: out of memory")

// SentinelForFilter maps a filter to its category sentinel. The js/wasm
// declaration layer has no HAL sentinel for OutOfMemory, so that category
// answers with this package's value, which the root binds.
func SentinelForFilter(f ErrorFilter) error {
	if f == ErrorFilterValidation {
		return ErrValidation
	}
	if f == ErrorFilterOutOfMemory {
		return ErrOutOfMemory
	}
	if f == ErrorFilterInternal {
		return ErrInternal
	}
	return nil
}

// FilterToJSName maps an ErrorFilter to the JS pushErrorScope filter string.
func FilterToJSName(f ErrorFilter) (string, bool) {
	switch f {
	case ErrorFilterValidation:
		return "validation", true
	case ErrorFilterOutOfMemory:
		return "out-of-memory", true
	case ErrorFilterInternal:
		return "internal", true
	}
	return "", false
}

// ErrorNameToFilter maps a JS GPU error constructor name to our category.
func ErrorNameToFilter(name string) (ErrorFilter, bool) {
	switch name {
	case "GPUValidationError":
		return ErrorFilterValidation, true
	case "GPUOutOfMemoryError":
		return ErrorFilterOutOfMemory, true
	case "GPUInternalError":
		return ErrorFilterInternal, true
	}
	return 0, false
}

// GPUErrorInfo is the browser build's captured-error payload; the root
// declaration layer converts it into its public GPUError.
type GPUErrorInfo struct {
	Type    ErrorFilter
	Message string
}

// PushErrorScope pushes a scope on the JS device.
func (d *Device) PushErrorScope(filter ErrorFilter) error {
	jsName, ok := FilterToJSName(filter)
	if !ok {
		return fmt.Errorf("wgpu: PushErrorScope: unknown error filter %d", int(filter))
	}
	d.ref_.Call("pushErrorScope", jsName)
	return nil
}

// PopErrorScope pops a scope on the JS device. Contract: (nil, nil) when the
// scope captured nothing; (info, nil) for a captured error; (nil,
// ErrErrorScopeEmpty) for the spec's OperationError (the JS promise rejects
// with an OperationError on an empty stack). Any other rejection is returned
// wrapped — the documented per-build limit never invents a category.
func (d *Device) PopErrorScope() (*GPUErrorInfo, error) {
	v, err := AwaitPromise(d.ref_.Call("popErrorScope"))
	if err != nil {
		var rej *PromiseRejection
		if errors.As(err, &rej) {
			if jsErr := NewJSError(rej.Value); jsErr != nil &&
				(jsErr.Name == "OperationError" || jsErr.Name == "GPUOperationError") {
				return nil, ErrErrorScopeEmpty
			}
		}
		return nil, err
	}
	if v.IsUndefined() || v.IsNull() {
		return nil, nil
	}

	name := ""
	if ctor := v.Get("constructor"); !ctor.IsUndefined() && !ctor.IsNull() {
		if nv := ctor.Get("name"); !nv.IsUndefined() && !nv.IsNull() {
			name = nv.String()
		}
	}
	filter, ok := ErrorNameToFilter(name)
	if !ok {
		return nil, fmt.Errorf("wgpu: PopErrorScope: unrecognised GPU error %q: %s", name, extractErrorMessage(v))
	}
	return &GPUErrorInfo{Type: filter, Message: extractErrorMessage(v)}, nil
}
