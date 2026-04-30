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

// Draw-time validation sentinel errors.
var (
	ErrDrawMissingPipeline            = errors.New("wgpu: draw called without SetPipeline")
	ErrDrawMissingBindGroup           = errors.New("wgpu: draw called with missing bind group")
	ErrDrawIncompatibleBindGroup      = errors.New("wgpu: draw called with incompatible bind group layout")
	ErrDrawMissingVertexBuffer        = errors.New("wgpu: draw called with insufficient vertex buffers")
	ErrDrawMissingIndexBuffer         = errors.New("wgpu: DrawIndexed called without SetIndexBuffer")
	ErrDrawMissingBlendConstant       = errors.New("wgpu: draw called without SetBlendConstant (pipeline uses constant blend factor)")
	ErrDrawLateBufferTooSmall         = errors.New("wgpu: bound buffer smaller than shader-required minimum")
	ErrDispatchMissingPipeline        = errors.New("wgpu: dispatch called without SetPipeline")
	ErrDispatchMissingBindGroup       = errors.New("wgpu: dispatch called with missing bind group")
	ErrDispatchIncompatibleBindGroup  = errors.New("wgpu: dispatch called with incompatible bind group layout")
	ErrDispatchLateBufferTooSmall     = errors.New("wgpu: dispatch: bound buffer smaller than shader-required minimum")
	ErrDispatchWorkgroupCountExceeded = errors.New("wgpu: dispatch workgroup count exceeds device limit")

	ErrDrawIndexFormatMismatch         = errors.New("wgpu: index buffer format does not match pipeline strip index format")
	ErrDrawIndirectBufferUsage         = errors.New("wgpu: indirect draw buffer missing INDIRECT usage")
	ErrDrawIndirectOffsetAlignment     = errors.New("wgpu: indirect draw buffer offset not 4-byte aligned")
	ErrDispatchIndirectBufferUsage     = errors.New("wgpu: indirect dispatch buffer missing INDIRECT usage")
	ErrDispatchIndirectOffsetAlignment = errors.New("wgpu: indirect dispatch buffer offset not 4-byte aligned")
	ErrDrawIndirectBufferOverrun       = errors.New("wgpu: indirect draw args exceed buffer size")
	ErrDispatchIndirectBufferOverrun   = errors.New("wgpu: indirect dispatch args exceed buffer size")
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
