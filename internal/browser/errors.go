//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"
)

// Common browser WebGPU errors.
var (
	// ErrWebGPUNotSupported is returned when navigator.gpu is unavailable.
	ErrWebGPUNotSupported = errors.New("wgpu: WebGPU not supported in this browser")

	// ErrNavigatorUnavailable is returned when navigator object is missing
	// (e.g., running in a non-browser WASM environment like Node.js without gpu).
	ErrNavigatorUnavailable = errors.New("wgpu: navigator not available")

	// ErrAdapterNotFound is returned when requestAdapter yields null
	// (no suitable GPU adapter found).
	ErrAdapterNotFound = errors.New("wgpu: no suitable GPU adapter found")

	// ErrDeviceCreationFailed is returned when requestDevice fails.
	ErrDeviceCreationFailed = errors.New("wgpu: device creation failed")
)

// JSError wraps a JavaScript error value for Go error handling.
type JSError struct {
	// Message is the error message extracted from the JS error.
	Message string
	// Name is the JS error constructor name (e.g., "TypeError", "OperationError").
	Name string
}

// Error implements the error interface.
func (e *JSError) Error() string {
	if e.Name != "" {
		return e.Name + ": " + e.Message
	}
	return e.Message
}

// NewJSError creates a JSError from a js.Value.
// Returns nil if the value is null or undefined.
func NewJSError(v js.Value) *JSError {
	if v.IsUndefined() || v.IsNull() {
		return nil
	}

	name := ""
	nameVal := v.Get("name")
	if !nameVal.IsUndefined() && !nameVal.IsNull() {
		name = nameVal.String()
	}

	return &JSError{
		Message: extractErrorMessage(v),
		Name:    name,
	}
}
