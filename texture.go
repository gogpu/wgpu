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

// Release destroys the texture. The underlying HAL texture is not freed
// immediately — destruction is deferred until the GPU completes any submission
// that may reference it. This prevents use-after-free on DX12/Vulkan.
func (t *Texture) Release() {
	if t.released {
		return
	}
	t.released = true

	halDevice := t.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := t.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyTexture(t.hal)
		return
	}

	subIdx := t.device.lastSubmissionIndex()
	halTex := t.hal
	dq.Defer(subIdx, "Texture", func() {
		halDevice.DestroyTexture(halTex)
	})
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
// (and its descriptor heap slots) is not freed immediately — it is deferred via
// DestroyQueue until the GPU completes any submission that may reference it.
// This prevents descriptor use-after-free on DX12 with maxFramesInFlight=2
// (BUG-DX12-007).
func (v *TextureView) Release() {
	if v.released {
		return
	}
	v.released = true

	if v.device == nil {
		return
	}

	halDevice := v.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := v.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyTextureView(v.hal)
		return
	}

	subIdx := v.device.lastSubmissionIndex()
	halTV := v.hal
	dq.Defer(subIdx, "TextureView", func() {
		halDevice.DestroyTextureView(halTV)
	})
}
