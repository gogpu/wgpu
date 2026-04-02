package wgpu

import (
	"github.com/gogpu/wgpu/hal"
)

// Texture represents a GPU texture.
type Texture struct {
	hal      hal.Texture
	device   *Device
	format   TextureFormat
	released bool
}

// Format returns the texture format.
func (t *Texture) Format() TextureFormat { return t.format }

// HalTexture returns the underlying HAL texture for advanced use cases.
// This enables interop with code that needs direct HAL access (e.g., gg
// GPU accelerator texture barriers and copy operations).
//
// Returns nil if the texture has been released.
func (t *Texture) HalTexture() hal.Texture {
	if t.released {
		return nil
	}
	return t.hal
}

// Release destroys the texture.
func (t *Texture) Release() {
	if t.released {
		return
	}
	t.released = true
	halDevice := t.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyTexture(t.hal)
	}
}

// TextureView represents a view into a texture.
type TextureView struct {
	hal      hal.TextureView
	device   *Device
	texture  *Texture
	released bool
}

// HalTextureView returns the underlying HAL texture view for advanced use cases.
// This enables interop with code that needs direct HAL access (e.g., gg
// GPU accelerator surface rendering).
//
// Returns nil if the view has been released.
func (v *TextureView) HalTextureView() hal.TextureView {
	if v.released {
		return nil
	}
	return v.hal
}

// Release marks the texture view for destruction. The underlying HAL TextureView
// (and its descriptor heap slots) is not freed immediately — it is deferred
// until the GPU completes any submission that may reference it. This prevents
// descriptor use-after-free on DX12 with maxFramesInFlight=2 (BUG-DX12-007).
func (v *TextureView) Release() {
	if v.released {
		return
	}
	v.released = true
	if v.device != nil && v.device.queue != nil && v.device.queue.pending != nil {
		v.device.queue.pending.deferTextureViewDestroy(v.hal)
	} else if v.device != nil {
		// Fallback: queue not available (device shutting down) — destroy immediately.
		halDevice := v.device.halDevice()
		if halDevice != nil {
			halDevice.DestroyTextureView(v.hal)
		}
	}
}
