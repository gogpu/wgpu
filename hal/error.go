//go:build !(js && wasm)

package hal

import (
	"errors"
	"fmt"
)

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

	// ErrNotReady indicates the resource is not ready yet.
	// For AcquireTexture, this means no image is available right now.
	// The caller should skip this frame and try again next frame.
	// This is NOT an error - it's a normal condition for non-blocking acquire.
	ErrNotReady = errors.New("hal: not ready")

	// ErrZeroArea indicates that both surface width and height must be non-zero.
	// This error is returned by Surface.Configure when the window has zero area.
	// Wait to recreate the surface until the window has non-zero area.
	// This commonly happens when:
	//   - Window is minimized
	//   - Window is not yet fully visible (timing issue on macOS)
	//   - Invalid dimensions passed to Configure
	ErrZeroArea = errors.New("hal: surface width and height must be non-zero")

	// ErrSurfaceDeviceMismatch is returned by Surface.Configure when the
	// Device cannot present to the surface. On GLES a Device belongs to the
	// EGL context it was opened on, which is bound to one native display:
	// GL objects are not shared between contexts, and a Wayland surface of
	// one wl_display cannot use a context of another. The public package maps
	// it to wgpu.ErrSurfaceDeviceMismatch.
	ErrSurfaceDeviceMismatch = errors.New("hal: device cannot present to this surface")

	// ErrContextReleased is returned when a backend's native context is used
	// after the last Surface and Device built on it were released. On GLES
	// that is an EGL context built on an app's Wayland display: releasing the
	// window's Surface before RequestDevice ends the context, and opening a
	// Device on the cached Adapter then returns this error. It wraps
	// ErrDeviceLost, as WebGPU's requestDevice on an expired adapter returns a
	// device that is already lost (spec: "requestDevice", lose the device with
	// reason "unknown"); request a new adapter. The public package re-exports
	// it as wgpu.ErrContextReleased.
	ErrContextReleased = fmt.Errorf("hal: the native context was released with the last surface and device that used it: %w", ErrDeviceLost)

	// ErrTimestampsNotSupported indicates the backend does not support timestamp queries.
	// This is returned by CreateQuerySet when QueryTypeTimestamp is requested
	// on backends that lack GPU timestamp support (e.g., Software, Noop, GLES without
	// GL_EXT_disjoint_timer_query).
	ErrTimestampsNotSupported = errors.New("hal: timestamp queries not supported by this backend")

	// ErrWorkgroupBarrierUnsupported is no longer returned since v0.34.8 and
	// is kept for compatibility. The software backend returned it from
	// CreateComputePipeline in v0.34.7 for a compute shader sharing workgroup
	// memory across a control barrier; since v0.34.8 its interpreter runs a
	// workgroup's invocations together and accepts such shaders (P1-SW-025).
	ErrWorkgroupBarrierUnsupported = errors.New("software backend: compute shaders that share workgroup memory across a control barrier are not supported yet (P1-SW-025)")

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

	// ErrTextureViewNotSampleable is returned by Device.CreateBindGroup when
	// the backend cannot sample a texture view it was given. GLES has no
	// texture views: a texture binds only through the GL target picked at its
	// creation, and a sampler reads its layers from layer 0, so a view of
	// another dimension (a 2d view of a layered texture, a cube view of a
	// 12-layer one) or of a subset of the layers would read the wrong texels.
	// The public package reports it as core.CreateBindGroupError of kind
	// CreateBindGroupErrorTextureViewNotSampleable.
	ErrTextureViewNotSampleable = errors.New("hal: the backend cannot sample this texture view")

	// ErrInvalidMapRange indicates MapBuffer was called with an offset+size
	// range that exceeds the buffer, or the buffer has no host-visible memory
	// so it cannot be mapped on the CPU.
	ErrInvalidMapRange = errors.New("hal: invalid buffer map range or non-mappable buffer")
)
