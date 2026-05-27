//go:build rust

package wgpu

import (
	"errors"
	"fmt"
)

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

	// ErrSubmitCommandBufferInvalid is returned when a command buffer is submitted twice.
	ErrSubmitCommandBufferInvalid = errors.New("wgpu: command buffer already submitted")

	// ErrSubmitBufferDestroyed is returned when a submitted command buffer references a destroyed buffer.
	ErrSubmitBufferDestroyed = errors.New("wgpu: submitted command buffer references destroyed buffer")

	// ErrSubmitBufferMapped is returned when a submitted command buffer references a mapped buffer.
	ErrSubmitBufferMapped = errors.New("wgpu: submitted command buffer references mapped buffer")

	// ErrSubmitTextureDestroyed is returned when a submitted command buffer references a destroyed texture.
	ErrSubmitTextureDestroyed = errors.New("wgpu: submitted command buffer references destroyed texture")

	// ErrSubmitBindGroupDestroyed is returned when a submitted command buffer references a destroyed bind group.
	ErrSubmitBindGroupDestroyed = errors.New("wgpu: submitted command buffer references destroyed bind group")
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

// GPUError represents a captured GPU error from an error scope.
// Matches the core.GPUError structure used by the native backend.
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

// ErrorFilter selects which errors an error scope captures.
type ErrorFilter int

const (
	// ErrorFilterValidation captures validation errors.
	ErrorFilterValidation ErrorFilter = 0
	// ErrorFilterOutOfMemory captures out-of-memory errors.
	ErrorFilterOutOfMemory ErrorFilter = 1
	// ErrorFilterInternal captures internal errors.
	ErrorFilterInternal ErrorFilter = 2
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
