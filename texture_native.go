//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Texture represents a GPU texture.
type Texture struct {
	hal          hal.Texture
	device       *Device
	format       TextureFormat
	released     bool
	surface      *core.Surface
	surfaceLease uint64
}

// resolveHAL is the single boundary from a public texture wrapper to HAL.
// Surface textures are borrowed and are usable only for their acquisition.
// Encoder validation and HAL conversion may call this more than once during
// one operation; acquisition and presentation are serialized by the render
// loop, so the lease remains stable across those calls.
func (t *Texture) resolveHAL() hal.Texture {
	if t == nil || t.released || t.hal == nil || t.device == nil || t.device.released.Load() {
		return nil
	}
	if t.surface != nil && !t.surface.AcquisitionValid(t.surfaceLease) {
		return nil
	}
	return t.hal
}

// Format returns the texture format.
func (t *Texture) Format() TextureFormat { return t.format }

// Release destroys the texture. The underlying HAL texture is not freed
// immediately — destruction is deferred until the GPU completes any submission
// that may reference it. This prevents use-after-free on DX12/Vulkan.
func (t *Texture) Release() {
	if t.released {
		return
	}
	// Surface textures are borrowed swapchain images. The wrapper never owns
	// their HAL lifetime, even while the acquisition is still active.
	if t.surface != nil {
		t.released = true
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
	hal          hal.TextureView
	device       *Device
	texture      *Texture
	released     bool
	surface      *core.Surface
	surfaceLease uint64
}

// resolveHAL is the single boundary from a public texture-view wrapper to HAL.
// Like Texture.resolveHAL, callers may resolve repeatedly within one serialized
// encoder operation without extending the surface acquisition lifetime.
func (v *TextureView) resolveHAL() hal.TextureView {
	if v == nil || v.released || v.hal == nil || v.device == nil || v.device.released.Load() {
		return nil
	}
	if v.surface != nil && !v.surface.AcquisitionValid(v.surfaceLease) {
		return nil
	}
	if v.surface != nil && v.texture != nil && v.texture.resolveHAL() == nil {
		return nil
	}
	return v.hal
}

// Texture returns the parent Texture that this view was created from.
// Returns nil if the view has been released.
func (v *TextureView) Texture() *Texture {
	if v == nil || v.released {
		return nil
	}
	return v.texture
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
