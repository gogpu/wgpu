package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// NewDeviceFromHAL creates a Device wrapping existing HAL device and queue objects.
// This constructor is used by backends that manage their own HAL lifecycle
// (e.g., the Rust FFI backend via wgpu-native) and need to expose their
// HAL objects through the wgpu public API.
//
// Ownership of halDevice and halQueue is transferred to the returned Device.
// The caller must not destroy them directly after this call.
func NewDeviceFromHAL(
	halDevice hal.Device,
	halQueue hal.Queue,
	features Features,
	limits Limits,
	label string,
) (*Device, error) {
	if halDevice == nil {
		return nil, fmt.Errorf("wgpu: halDevice is nil")
	}
	if halQueue == nil {
		return nil, fmt.Errorf("wgpu: halQueue is nil")
	}

	// Create a core.Adapter stub for the core.Device constructor.
	// The Rust backend doesn't go through the adapter registry, so we
	// create a minimal adapter that satisfies the core.Device requirements.
	coreAdapter := &core.Adapter{
		Features: features,
		Limits:   limits,
	}

	coreDevice := core.NewDevice(halDevice, coreAdapter, features, limits, label)

	// Single shared encoder pool for both user command encoders and PendingWrites.
	pool := newEncoderPool(halDevice)

	queue := &Queue{
		hal:       halQueue,
		halDevice: halDevice,
		pending:   newPendingWrites(halDevice, halQueue, pool),
	}

	coreDevice.SetAssociatedQueue(&core.Queue{Label: label + " Queue"})

	device := &Device{
		core:           coreDevice,
		queue:          queue,
		cmdEncoderPool: pool,
	}
	queue.device = device

	return device, nil
}

// NewSurfaceFromHAL creates a Surface wrapping an existing HAL surface.
// This constructor is used by backends that create surfaces externally
// (e.g., the Rust FFI backend).
//
// Ownership of halSurface is transferred to the returned Surface.
func NewSurfaceFromHAL(halSurface hal.Surface, label string) *Surface {
	coreSurface := core.NewSurface(halSurface, label)
	return &Surface{
		core: coreSurface,
	}
}

// NewTextureFromHAL creates a Texture wrapping an existing HAL texture.
// Used for backward compatibility and testing.
func NewTextureFromHAL(halTexture hal.Texture, device *Device, format TextureFormat) *Texture {
	return &Texture{hal: halTexture, device: device, format: format}
}

// NewTextureViewFromHAL creates a TextureView wrapping an existing HAL texture view.
func NewTextureViewFromHAL(halView hal.TextureView, device *Device) *TextureView {
	return &TextureView{hal: halView, device: device}
}

// NewSamplerFromHAL creates a Sampler wrapping an existing HAL sampler.
func NewSamplerFromHAL(halSampler hal.Sampler, device *Device) *Sampler {
	return &Sampler{hal: halSampler, device: device}
}

// HalDevice returns the underlying HAL device for advanced use cases.
// This enables interop with code that needs direct HAL access (e.g., gg
// GPU accelerator, DeviceProvider interfaces).
//
// Returns nil if the device has been released or has no HAL backend.
func (d *Device) HalDevice() hal.Device {
	return d.halDevice()
}

// HalQueue returns the underlying HAL queue.
// Returns nil if the device has been released or has no HAL backend.
func (d *Device) HalQueue() hal.Queue {
	if d.queue == nil {
		return nil
	}
	return d.queue.hal
}
