//go:build js && wasm

package wgpu

import "errors"

// Public API sentinel errors.
var (
	// ErrReleased is returned when operating on a released resource.
	ErrReleased = errors.New("wgpu: resource already released")

	// ErrNoAdapters is returned when no GPU adapters are found.
	ErrNoAdapters = errors.New("wgpu: no GPU adapters available")

	// ErrNoBackends is returned when no backends are registered.
	ErrNoBackends = errors.New("wgpu: no backends registered (import a backend package)")

	// ErrDeviceLost is returned when the GPU device is lost.
	ErrDeviceLost = errors.New("wgpu: device lost")

	// ErrOutOfMemory is returned when the GPU is out of memory.
	ErrOutOfMemory = errors.New("wgpu: out of memory")

	// ErrSurfaceLost is returned when the surface is lost.
	ErrSurfaceLost = errors.New("wgpu: surface lost")

	// ErrSurfaceOutdated is returned when the surface is outdated.
	ErrSurfaceOutdated = errors.New("wgpu: surface outdated")

	// ErrTimeout is returned when an operation times out.
	ErrTimeout = errors.New("wgpu: timeout")
)

// GPUError represents a GPU error.
type GPUError struct {
	Message string
}

func (e *GPUError) Error() string { return e.Message }

// ErrorFilter selects which errors an error scope captures.
type ErrorFilter int

const (
	ErrorFilterValidation  ErrorFilter = 0
	ErrorFilterOutOfMemory ErrorFilter = 1
	ErrorFilterInternal    ErrorFilter = 2
)
