package wgpu

import "github.com/gogpu/wgpu/hal"

// HAL returns the native objects of the backend that serves this instance:
// the objects of the HAL instance it uses for its highest-priority enabled
// backend.
//
// A wgpu Instance can hold one HAL instance per enabled backend (Vulkan, DX12
// and GLES on Windows, for example). The value here is the first in wgpu's
// backend priority order; Adapter.HAL and Device.HAL name the instance that
// actually serves them, which is the one to use when the two differ.
//
// The handles are borrowed and valid only while this instance lives; see
// package hal. HAL returns the zero Handles (Kind KindUnknown) after the
// instance is released.
func (i *Instance) HAL() hal.Handles {
	if i == nil {
		return hal.Handles{}
	}
	return i.halHandles()
}

// HAL returns the native objects of the backend that serves this adapter: its
// physical device (Vulkan), DXGI adapter, MTLDevice or GL context, and the
// instance it was enumerated from. Handles that belong to a device are zero.
//
// The handles are borrowed and valid only while this adapter lives; see
// package hal. HAL returns the zero Handles (Kind KindUnknown) after
// Adapter.Release() on every build, and after the owning instance is released
// on the native and rust builds. That is a contract choice consistent with
// Instance.HAL(), not a lifetime requirement of the objects themselves.
//
// On the native build core destroys the HAL adapter and the instance, so a
// destroyed VkInstance handle must not leak out of a released instance's
// adapter. On the rust build wgpu-native's WGPUAdapterImpl owns its own
// Arc<Context> and stays alive after the instance is released, so the zero
// Handles there is the chosen contract only (ADR-070 §15.5). On js/wasm there
// is no instance destroy: the adapter keeps reporting its handles until
// Adapter.Release().
func (a *Adapter) HAL() hal.Handles {
	if a == nil {
		return hal.Handles{}
	}
	return a.halHandles()
}

// HAL returns the native objects of the backend that serves this device: the
// device and its queue, the adapter the device was opened from, and the
// instance they belong to, in the terms of the backend named by the returned
// Kind.
//
// The handles are borrowed and valid only while this device lives; see package
// hal. HAL returns the zero Handles (Kind KindUnknown) after the device is
// released and for a device without a HAL implementation.
//
// HAL is safe for concurrent use, allocates nothing and does not wait for the
// GPU. It takes the guard wgpu's own operations take, so a device released
// concurrently yields either its handles or the zero value, never a dangling
// pointer.
//
// Everything reached through the handles bypasses wgpu's validation, resource
// state tracking and synchronization.
func (d *Device) HAL() hal.Handles {
	if d == nil {
		return hal.Handles{}
	}
	return d.halHandles()
}

// HAL returns the presentation object of the backend this surface is served by:
// the VkSurfaceKHR, the IDXGISwapChain4, the CAMetalLayer or the EGLSurface,
// in the terms of the returned Kind.
//
// The object can be recreated by wgpu when the surface is reconfigured or
// resized; take a fresh Handles after every Configure. Fields that name a
// device or a queue are zero: use Device.HAL for those.
//
// The handle is borrowed and valid only while this surface lives; see package
// hal. HAL returns the zero Handles (Kind KindUnknown) after the surface is
// released.
func (s *Surface) HAL() hal.Handles {
	if s == nil {
		return hal.Handles{}
	}
	return s.halHandles()
}
