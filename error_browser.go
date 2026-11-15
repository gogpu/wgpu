//go:build js && wasm

package wgpu

import (
	"errors"
	"fmt"
)

// Public API sentinel errors.
var (
	// ErrReleased is declared in map_types_browser.go, next to the buffer
	// mapping sentinels: the browser buffer implementation returns it and the
	// resource package keeps it, so its home travels with the buffers. Root
	// re-exports the same value once the move lands.

	// Adapter-selection sentinels (ADR-083 §2.2). internal/core has no files in
	// the js/wasm build, and the browser's WebGPU reports its own errors, so they
	// are declared here with the messages of the native build. This build never
	// returns them; they exist for source compatibility.
	//
	// ErrNoBackendRegistered: the requested backend mask selects no registered
	// backend (compiled out or excluded).
	ErrNoBackendRegistered = errors.New("wgpu: no backend registered for the requested mask (compiled out or excluded)")

	// ErrNoHardwareAdapter: backends are registered but none produced an adapter.
	ErrNoHardwareAdapter = errors.New("wgpu: no hardware adapter available")

	// ErrNoFallbackAdapter: ForceFallbackAdapter requested, no CPU adapter exists.
	ErrNoFallbackAdapter = errors.New("wgpu: no fallback adapter available")

	// ErrDeviceLost is returned when the GPU device is lost.
	ErrDeviceLost = errors.New("wgpu: device lost")

	// ErrContextReleased is declared for source compatibility with the native
	// build, where GLES returns it when a native context is gone. This build
	// never returns it. It wraps ErrDeviceLost there and here.
	ErrContextReleased = fmt.Errorf("wgpu: the native context was released with the last surface and device that used it: %w", ErrDeviceLost)

	// ErrOutOfMemory is returned when the GPU is out of memory.
	ErrOutOfMemory = errors.New("wgpu: out of memory")

	// ErrSurfaceLost is returned when the surface is lost.
	ErrSurfaceLost = errors.New("wgpu: surface lost")

	// ErrSurfaceOutdated is returned when the surface is outdated.
	ErrSurfaceOutdated = errors.New("wgpu: surface outdated")

	// ErrTimeout is returned when an operation times out.
	ErrTimeout = errors.New("wgpu: timeout")

	// ErrWorkgroupBarrierUnsupported is no longer returned since v0.34.8 and
	// is kept for compatibility. It was the software backend's refusal of a
	// compute shader that shares workgroup memory across a control barrier in
	// v0.34.7 (P1-SW-025). The browser backend never returns it; it exists so
	// that code checking it builds on every target.
	ErrWorkgroupBarrierUnsupported = errors.New("software backend: compute shaders that share workgroup memory across a control barrier are not supported yet (P1-SW-025)")
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
