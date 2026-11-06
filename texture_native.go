//go:build !rust && !(js && wasm)

package wgpu

import (
	"slices"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
)

// Texture represents a GPU texture.
type Texture struct {
	hal    hal.Texture
	device *Device
	format gputypes.TextureFormat
	// viewFormats is a copy of the TextureDescriptor.ViewFormats the texture
	// was created with: the formats a view may reinterpret it as
	// (CreateTextureView). It is a pointer so Texture stays a comparable type.
	viewFormats  *[]gputypes.TextureFormat
	released     bool
	surface      *core.Surface
	surfaceLease uint64

	// coreTexture holds the core.Texture for dense TrackerIndex allocation.
	// This enables submit-time barrier injection: the TrackerIndex is used
	// by populateTextureScope (via core.TextureView.Parent) to record per-
	// texture usage in the command buffer's TextureUsageScope. At submit
	// time the scope is merged into the device-level DeviceTracker which
	// produces the barrier list.
	//
	// nil for textures created without HAL (e.g., test stubs or browser).
	// Swapchain textures also get a coreTexture so barriers work for
	// surface textures used as render targets.
	//
	// Reference: Rust wgpu-core resource.rs Texture (holds TrackingData)
	coreTexture *core.Texture
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
func (t *Texture) Format() gputypes.TextureFormat { return t.format }

// viewFormatList returns the view formats the texture was created with.
func (t *Texture) viewFormatList() []gputypes.TextureFormat {
	if t.viewFormats == nil {
		return nil
	}
	return *t.viewFormats
}

// copyViewFormats copies TextureDescriptor.ViewFormats, so a later change to
// the caller's slice does not change what views are allowed.
func copyViewFormats(formats []gputypes.TextureFormat) *[]gputypes.TextureFormat {
	if len(formats) == 0 {
		return nil
	}
	c := slices.Clone(formats)
	return &c
}

// Release drops the application's reference to the texture. The HAL texture
// is destroyed once nothing references it any more — no bind group,
// unfinished command encoder, unsubmitted command buffer or pending queue
// write — and the GPU has completed every submission that may use it.
// This prevents use-after-free on DX12/Vulkan, as Rust wgpu-core's
// Arc<Texture> does.
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

	if ref := t.ref(); ref != nil {
		ref.Drop()
		return
	}
	// No reference count (a wrapped HAL texture): destroy it after the last
	// submission.
	var td *track.TrackingData
	if t.coreTexture != nil {
		td = t.coreTexture.TrackingData()
	}
	t.device.destroyTextureDeferred(t.hal, td)
}

// ref returns the texture's reference count, or nil for a texture that has
// none: a borrowed surface texture or a wrapped HAL texture.
func (t *Texture) ref() *core.ResourceRef {
	if t == nil || t.coreTexture == nil {
		return nil
	}
	return t.coreTexture.Ref
}

// destroyTextureDeferred destroys a HAL texture once the GPU has completed
// the last submission so far, and frees its tracker index with it.
//
// ADR-060: releasing the TrackerIndex prevents monotonic growth (#307);
// surface textures handle this in Surface.destroySwapchainTexture. The index
// is freed together with the HAL texture, after the GPU is done with it, as
// Rust frees it in Drop for TrackingData once the last Arc (including
// in-flight submissions) is gone (wgpu-core resource.rs).
func (d *Device) destroyTextureDeferred(halTex hal.Texture, td *track.TrackingData) {
	releaseIndex := func() {
		if td != nil {
			td.Release()
		}
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		releaseIndex()
		return
	}

	dq := d.destroyQueue()
	if dq == nil {
		halDevice.DestroyTexture(halTex)
		releaseIndex()
		return
	}

	dq.Defer(d.lastSubmissionIndex(), "Texture", func() {
		halDevice.DestroyTexture(halTex)
		releaseIndex()
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
	// ref counts the view's owners: the application, bind groups and command
	// encoders. nil for a view without one (surface views from
	// SurfaceTexture.CreateView, wrapped HAL views).
	ref *core.ResourceRef
	// dimension and baseMipLevel are the view descriptor's resolved dimension
	// and base mip level, copied into core.TextureView for the render pass
	// depth-slice validation. dimension is undefined for a wrapped HAL view,
	// whose descriptor is not known.
	dimension    gputypes.TextureViewDimension
	baseMipLevel uint32
	// baseArrayLayer and arrayLayerCount are the view's resolved array layer
	// range, copied into core.TextureView for the render pass attachment
	// overlap check. arrayLayerCount is 0 when the range is not known.
	baseArrayLayer  uint32
	arrayLayerCount uint32
}

// resolveViewLayerCount returns the number of array layers a texture view
// covers: the requested count, or the default WebGPU derives ("resolving
// GPUTextureViewDescriptor defaults"): 1 for a 1d, 2d or 3d view, 6 for a
// cube, and the layers from baseArrayLayer to the end of the texture for a
// 2d-array or cube-array view. 0 when the texture is not known.
func resolveViewLayerCount(texture *Texture, dim gputypes.TextureViewDimension, base, requested uint32) uint32 {
	if requested != 0 {
		return requested
	}
	switch dim {
	case gputypes.TextureViewDimension1D, gputypes.TextureViewDimension2D, gputypes.TextureViewDimension3D:
		return 1
	case gputypes.TextureViewDimensionCube:
		return 6
	}
	if texture == nil || texture.coreTexture == nil {
		return 0
	}
	if layers := texture.coreTexture.Size().DepthOrArrayLayers; layers > base {
		return layers - base
	}
	return 0
}

// resolveViewDimension returns the dimension a texture view has: the
// requested one, or the default WebGPU derives from the texture ("resolving
// GPUTextureViewDescriptor defaults"): 1d, 2d for a single-layer 2D texture,
// 2d-array for a layered one, 3d. Undefined when the texture's own dimension
// is not known (a texture wrapped from a HAL handle).
func resolveViewDimension(texture *Texture, requested gputypes.TextureViewDimension) gputypes.TextureViewDimension {
	if requested != gputypes.TextureViewDimensionUndefined {
		return requested
	}
	if texture == nil || texture.coreTexture == nil {
		return gputypes.TextureViewDimensionUndefined
	}
	switch texture.coreTexture.Dimension() {
	case gputypes.TextureDimension1D:
		return gputypes.TextureViewDimension1D
	case gputypes.TextureDimension3D:
		return gputypes.TextureViewDimension3D
	default:
		if texture.coreTexture.Size().DepthOrArrayLayers > 1 {
			return gputypes.TextureViewDimension2DArray
		}
		return gputypes.TextureViewDimension2D
	}
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

// Release drops the application's reference to the texture view. The HAL
// TextureView (and its descriptor heap slots) is destroyed once no bind
// group, unfinished command encoder or unsubmitted command buffer references
// it, and the GPU has completed every submission that may use it. This
// prevents descriptor use-after-free on DX12 with maxFramesInFlight=2
// (BUG-DX12-007).
func (v *TextureView) Release() {
	if v.released {
		return
	}
	v.released = true

	if v.ref != nil {
		v.ref.Drop()
		return
	}
	if v.device != nil {
		v.device.destroyTextureViewDeferred(v.hal)
	}
}

// destroyTextureViewDeferred destroys a HAL texture view once the GPU has
// completed the last submission so far.
func (d *Device) destroyTextureViewDeferred(halView hal.TextureView) {
	halDevice := d.halDevice()
	if halDevice == nil {
		return
	}

	dq := d.destroyQueue()
	if dq == nil {
		halDevice.DestroyTextureView(halView)
		return
	}

	dq.Defer(d.lastSubmissionIndex(), "TextureView", func() {
		halDevice.DestroyTextureView(halView)
	})
}
