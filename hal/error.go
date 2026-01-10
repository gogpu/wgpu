package hal

import "errors"

// Common HAL errors representing unrecoverable GPU states.
var (
	// ErrBackendNotFound indicates the requested backend is not registered.
	ErrBackendNotFound = errors.New("hal: backend not found")
	// ErrDeviceOutOfMemory indicates the GPU has exhausted its memory.
	// This is unrecoverable - the application should reduce resource usage
	// or gracefully terminate.
	ErrDeviceOutOfMemory = errors.New("hal: device out of memory")

	// ErrDeviceLost indicates the GPU device has been lost.
	// This can happen due to:
	//   - GPU driver crash or reset
	//   - GPU hardware disconnection
	//   - Driver timeout (TDR on Windows)
	// The device cannot be recovered and must be recreated.
	ErrDeviceLost = errors.New("hal: device lost")

	// ErrSurfaceLost indicates the rendering surface has been destroyed.
	// This typically happens when the window is closed.
	// The surface cannot be recovered - create a new one if needed.
	ErrSurfaceLost = errors.New("hal: surface lost")

	// ErrSurfaceOutdated indicates the surface configuration is stale.
	// This happens when:
	//   - Window was resized
	//   - Display mode changed
	//   - Surface pixel format changed
	// Call Surface.Configure again with updated parameters.
	ErrSurfaceOutdated = errors.New("hal: surface outdated")

	// ErrTimeout indicates an operation timed out.
	// This is typically returned by Wait operations.
	ErrTimeout = errors.New("hal: timeout")

	// ErrZeroArea indicates that both surface width and height must be non-zero.
	// This error is returned by Surface.Configure when the window has zero area.
	// Wait to recreate the surface until the window has non-zero area.
	// This commonly happens when:
	//   - Window is minimized
	//   - Window is not yet fully visible (timing issue on macOS)
	//   - Invalid dimensions passed to Configure
	ErrZeroArea = errors.New("hal: surface width and height must be non-zero")

	// ErrDriverBug indicates the GPU driver returned an invalid or unexpected result
	// that violates the graphics API specification. This typically indicates a
	// driver bug rather than an application error.
	//
	// Known cases:
	//   - Intel Iris Xe: vkCreateGraphicsPipelines returns VK_SUCCESS but writes
	//     VK_NULL_HANDLE to pipeline output (Vulkan spec violation)
	//
	// The operation cannot be completed. Possible workarounds:
	//   - Update GPU driver to latest version
	//   - Use a different backend (e.g., DX12 instead of Vulkan)
	//   - Use software rendering backend
	//
	// See: https://github.com/gogpu/wgpu/issues/24
	ErrDriverBug = errors.New("hal: driver bug detected (API spec violation)")
)
