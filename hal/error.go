package hal

import "errors"

// Common HAL errors representing unrecoverable GPU states.
var (
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
)
