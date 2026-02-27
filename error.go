package wgpu

import (
	"errors"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Sentinel errors re-exported from HAL.
var (
	ErrDeviceLost      = hal.ErrDeviceLost
	ErrOutOfMemory     = hal.ErrDeviceOutOfMemory
	ErrSurfaceLost     = hal.ErrSurfaceLost
	ErrSurfaceOutdated = hal.ErrSurfaceOutdated
	ErrTimeout         = hal.ErrTimeout
)

// Public API sentinel errors.
var (
	// ErrReleased is returned when operating on a released resource.
	ErrReleased = errors.New("wgpu: resource already released")

	// ErrNoAdapters is returned when no GPU adapters are found.
	ErrNoAdapters = errors.New("wgpu: no GPU adapters available")

	// ErrNoBackends is returned when no backends are registered.
	ErrNoBackends = errors.New("wgpu: no backends registered (import a backend package)")
)

// Re-export error types from core.
type GPUError = core.GPUError
type ErrorFilter = core.ErrorFilter

const (
	ErrorFilterValidation  = core.ErrorFilterValidation
	ErrorFilterOutOfMemory = core.ErrorFilterOutOfMemory
	ErrorFilterInternal    = core.ErrorFilterInternal
)
