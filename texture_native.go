//go:build !rust && !(js && wasm)

package wgpu

import (
	"slices"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
)

// Texture represents a GPU texture.
type Texture struct {
	hal    hal.Texture
	device core.DeviceView
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

// resolveHAL is the single boundary from a public texture wrapper to HAL for
// new work: a released texture cannot be recorded into new commands.
// Surface textures are borrowed and are usable only for their acquisition.
// Encoder validation and HAL conversion may call this more than once during
// one operation; acquisition and presentation are serialized by the render
// loop, so the lease remains stable across those calls.
func (t *Texture) resolveHAL() hal.Texture {
	if t == nil || t.released {
		return nil
	}
	return t.recordedHAL()
}

// recordedHAL returns the HAL texture for work already recorded against t —
// a command buffer or a pending queue write being submitted — or nil when
// that work can no longer run. Release() is Rust's drop, not destroy()
// (ADR-056): recorded work holds a reference, so a released texture with a
// reference count stays alive until the work completes, as an Arc<Texture>
// held by Rust wgpu-core's trackers does, and Submit must accept it
// (P1-CORE-020). The work cannot run when the device is gone, when a surface
// texture's acquisition has ended, or when a texture without a reference
// count (wrapped from HAL) was released, since that queued its destruction.
func (t *Texture) recordedHAL() hal.Texture {
	if t == nil || t.hal == nil || t.device == nil || t.device.IsReleased() {
		return nil
	}
	if t.surface != nil {
		if !t.surface.AcquisitionValid(t.surfaceLease) {
			return nil
		}
		return t.hal
	}
	if t.released && t.ref() == nil {
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

// newDetachedTexture returns a texture that carries the descriptor's metadata
// (its format and view formats) and nothing else: no device and no HAL object.
// It backs the public wgputest.NewDetachedTexture; see newDetachedTextureFactory.
// desc may be nil: the handle then carries zero metadata.
func newDetachedTexture(desc *TextureDescriptor) *Texture {
	t := &Texture{}
	if desc != nil {
		t.format = desc.Format
		t.viewFormats = copyViewFormats(desc.ViewFormats)
	}
	return t
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
	// A detached texture (wgputest.NewDetachedTexture) has no device and no
	// HAL object: Release only marks it released. Reaching
	// t.device.destroyTextureDeferred would dereference the nil device.
	if t.device == nil {
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
	textureDestroyDeferred(t.device, t.hal, td)
}

// ref returns the texture's reference count, or nil for a texture that has
// none: a borrowed surface texture or a wrapped HAL texture.
func (t *Texture) ref() *core.ResourceRef {
	if t == nil || t.coreTexture == nil {
		return nil
	}
	return t.coreTexture.Ref
}

// needsStencilBlit reports whether an upload into aspect of t must be lowered
// to the stencil blit (core.Device.StencilUploadNeedsBlit).
func (t *Texture) needsStencilBlit(aspect gputypes.TextureAspect) bool {
	return t != nil && t.coreTexture != nil && t.coreTexture.Device().StencilUploadNeedsBlit(t.format, aspect)
}

// textureDestroyDeferred destroys a HAL texture once the GPU has completed
// the last submission so far, and frees its tracker index with it.
//
// It takes the device view rather than the device: this type moves to an
// extraction package, whose code cannot name the frontend device type
// (ADR-070 §15.3). (*Device).destroyTextureDeferred in the device file
// delegates here for the call sites that still hold a device.
//
// ADR-060: releasing the TrackerIndex prevents monotonic growth (#307);
// surface textures handle this in Surface.destroySwapchainTexture. The index
// is freed together with the HAL texture, after the GPU is done with it, as
// Rust frees it in Drop for TrackingData once the last Arc (including
// in-flight submissions) is gone (wgpu-core resource.rs).
func textureDestroyDeferred(d core.DeviceView, halTex hal.Texture, td *track.TrackingData) {
	releaseIndex := func() {
		if td != nil {
			td.Release()
		}
	}

	halDevice := d.HALDevice()
	if halDevice == nil {
		releaseIndex()
		return
	}

	dq := d.DestroyQueue()
	if dq == nil {
		halDevice.DestroyTexture(halTex)
		releaseIndex()
		return
	}

	dq.Defer(d.LastSubmissionIndex(), "Texture", func() {
		halDevice.DestroyTexture(halTex)
		releaseIndex()
	})
}

// TextureView represents a view into a texture.
type TextureView struct {
	hal          hal.TextureView
	device       core.DeviceView
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
	// mipLevelCount, format and aspect are the view's resolved mip count
	// (0 when not known), its descriptor format (Undefined: the texture's)
	// and aspect, copied into core.TextureView for the render pass
	// renderable-view and attachment format checks.
	mipLevelCount uint32
	format        gputypes.TextureFormat
	aspect        gputypes.TextureAspect
	// internalUsage is core.TextureView.InternalUsage: usages only the
	// frontend's own commands use the view with (the stencil upload blit).
	internalUsage gputypes.TextureUsage
}

// resolveViewMipCount returns the number of mip levels a texture view
// covers: the requested count, or the levels from baseMipLevel to the end
// of the texture ("resolving GPUTextureViewDescriptor defaults"). 0 when
// the texture is not known.
func resolveViewMipCount(texture *Texture, base, requested uint32) uint32 {
	if requested != 0 {
		return requested
	}
	if texture == nil || texture.coreTexture == nil {
		return 0
	}
	if mips := texture.coreTexture.MipLevelCount(); mips > base {
		return mips - base
	}
	return 0
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
	if v == nil || v.released || v.hal == nil || v.device == nil || v.device.IsReleased() {
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
		textureViewDestroyDeferred(v.device, v.hal)
	}
}

// textureViewDestroyDeferred destroys a HAL texture view once the GPU has
// completed the last submission so far. It takes the device view for the
// reason textureDestroyDeferred does.
func textureViewDestroyDeferred(d core.DeviceView, halView hal.TextureView) {
	halDevice := d.HALDevice()
	if halDevice == nil {
		return
	}

	dq := d.DestroyQueue()
	if dq == nil {
		halDevice.DestroyTextureView(halView)
		return
	}

	dq.Defer(d.LastSubmissionIndex(), "TextureView", func() {
		halDevice.DestroyTextureView(halView)
	})
}
