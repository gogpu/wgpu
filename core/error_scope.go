package core

import (
	"fmt"
	"sync"
)

// ErrorFilter specifies which error types to capture in an error scope.
//
// Error scopes allow programmatic capture of GPU validation errors,
// out-of-memory conditions, and internal errors per the W3C WebGPU spec.
type ErrorFilter int

const (
	// ErrorFilterValidation captures validation errors.
	// These occur when API usage violates the WebGPU specification
	// (e.g., invalid descriptors, missing resources, state errors).
	ErrorFilterValidation ErrorFilter = iota

	// ErrorFilterOutOfMemory captures out-of-memory errors.
	// These occur when GPU memory allocation fails.
	ErrorFilterOutOfMemory

	// ErrorFilterInternal captures internal errors.
	// These occur due to implementation bugs or unexpected GPU behavior.
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
	default:
		return fmt.Sprintf("ErrorFilter(%d)", int(f))
	}
}

// GPUError represents a captured GPU error from an error scope.
//
// GPUError is returned by ErrorScopeManager.PopErrorScope when an error
// was captured within the scope. It implements the error interface for
// convenient integration with Go error handling.
type GPUError struct {
	// Type identifies the category of the error.
	Type ErrorFilter

	// Message provides a human-readable description of the error.
	Message string
}

// Error implements the error interface.
func (e *GPUError) Error() string {
	return fmt.Sprintf("GPU %s error: %s", e.Type, e.Message)
}

// errorScope represents a single entry in the error scope stack.
//
// Each scope captures the first error that matches its filter.
// Subsequent matching errors within the same scope are silently discarded.
type errorScope struct {
	filter ErrorFilter
	err    *GPUError // first captured error, nil if none
}

// ErrorScopeManager manages a stack of error scopes for a device.
//
// Error scopes allow users to programmatically capture GPU errors instead
// of relying on uncaptured error callbacks. Scopes are LIFO (stack-based):
// the most recently pushed scope is checked first when reporting errors.
//
// This follows the W3C WebGPU error scope specification:
// https://www.w3.org/TR/webgpu/#error-scopes
//
// ErrorScopeManager is safe for concurrent use.
type ErrorScopeManager struct {
	mu     sync.Mutex
	scopes []errorScope
}

// NewErrorScopeManager creates a new ErrorScopeManager with an empty scope stack.
func NewErrorScopeManager() *ErrorScopeManager {
	return &ErrorScopeManager{}
}

// PushErrorScope pushes a new error scope onto the stack.
//
// The scope will capture the first error matching the specified filter.
// Multiple scopes can be pushed to capture different error types simultaneously.
//
// Each PushErrorScope must be paired with a corresponding PopErrorScope.
func (m *ErrorScopeManager) PushErrorScope(filter ErrorFilter) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.scopes = append(m.scopes, errorScope{
		filter: filter,
		err:    nil,
	})
}

// PopErrorScope pops the most recently pushed error scope and returns the
// captured error, if any.
//
// Returns the captured GPUError, or nil if no matching error occurred.
// Returns an error (second return value) if the scope stack is empty.
//
// This must be called once for each PushErrorScope call.
func (m *ErrorScopeManager) PopErrorScope() (*GPUError, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.scopes) == 0 {
		return nil, fmt.Errorf("error scope stack is empty: no matching PushErrorScope")
	}

	// Pop the last scope (LIFO)
	last := len(m.scopes) - 1
	scope := m.scopes[last]
	m.scopes = m.scopes[:last]

	return scope.err, nil
}

// ReportError reports a GPU error to the error scope stack.
//
// The error is delivered to the topmost scope whose filter matches the
// error type. Only the first error per scope is captured; subsequent
// matching errors are silently discarded.
//
// If no scope matches (either the stack is empty or no scope has a
// matching filter), the error is considered "uncaptured". The method
// returns false in this case, allowing the caller to handle uncaptured
// errors (e.g., via a device lost callback or logging).
//
// Returns true if the error was captured by a scope, false otherwise.
func (m *ErrorScopeManager) ReportError(filter ErrorFilter, message string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Walk the stack from top to bottom (LIFO order)
	for i := len(m.scopes) - 1; i >= 0; i-- {
		if m.scopes[i].filter == filter {
			// Only capture the first error per scope
			if m.scopes[i].err == nil {
				m.scopes[i].err = &GPUError{
					Type:    filter,
					Message: message,
				}
			}
			return true
		}
	}

	return false
}

// ScopeDepth returns the current number of pushed error scopes.
//
// This is primarily useful for debugging and testing.
func (m *ErrorScopeManager) ScopeDepth() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.scopes)
}

// =============================================================================
// Device integration
// =============================================================================

// PushErrorScope pushes a new error scope onto this device's error scope stack.
//
// The scope will capture the first error matching the specified filter type.
// Error scopes are LIFO (stack-based) -- the last pushed scope is checked first.
//
// Each PushErrorScope must be paired with a corresponding PopErrorScope.
//
// Example usage:
//
//	device.PushErrorScope(core.ErrorFilterValidation)
//	// ... GPU operations that might produce validation errors
//	gpuErr := device.PopErrorScope()
//	if gpuErr != nil {
//	    log.Printf("Validation error: %s", gpuErr.Message)
//	}
func (d *Device) PushErrorScope(filter ErrorFilter) {
	d.errorScopes().PushErrorScope(filter)
}

// PopErrorScope pops the most recently pushed error scope and returns the
// captured error, if any.
//
// Returns the captured GPUError, or nil if no matching error occurred
// within the scope. Panics if the error scope stack is empty (no
// matching PushErrorScope was called).
//
// Note: Error scopes are LIFO -- the last pushed scope is popped first.
func (d *Device) PopErrorScope() *GPUError {
	gpuErr, err := d.errorScopes().PopErrorScope()
	if err != nil {
		panic(fmt.Sprintf("PopErrorScope: %v", err))
	}
	return gpuErr
}

// reportError reports a GPU error to the device's error scope stack.
//
// This is called internally when a GPU error occurs during validation
// or GPU operations. The error is delivered to the topmost matching
// error scope. If no scope matches, the error is considered uncaptured.
//
// Returns true if the error was captured by a scope, false otherwise.
func (d *Device) reportError(filter ErrorFilter, message string) bool {
	return d.errorScopes().ReportError(filter, message)
}

// errorScopes returns the device's ErrorScopeManager, creating it lazily
// if needed. This supports both the HAL-based and ID-based device paths.
//
// Note: This is not thread-safe for the initial creation of the manager
// when called concurrently on a brand-new Device. In practice, this is
// not an issue because PushErrorScope is always called before any GPU
// operations, and GPU operations happen on a single goroutine per device.
// The ErrorScopeManager itself is fully thread-safe once created.
func (d *Device) errorScopes() *ErrorScopeManager {
	if d.errorScopeManager == nil {
		d.errorScopeManager = NewErrorScopeManager()
	}
	return d.errorScopeManager
}
