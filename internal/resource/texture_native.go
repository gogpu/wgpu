//go:build !rust && !(js && wasm)

package resource

import (
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

	// Pub caches the public wrapper (wgpu.Texture) built over this texture.
	// TextureView.Texture() returns it, so the view hands back the same
	// wrapper the device did and the hot accessor allocates nothing
	// (ADR-070 S1 F1, the MapPending.Pub pattern). It is written once, when
	// the frontend creates the wrapper, before the texture is shared.
	Pub any
}

// NewTexture builds a texture over a HAL object and the shared device view.
// The device that creates a texture is the only caller.
func NewTexture(halTexture hal.Texture, device core.DeviceView, format gputypes.TextureFormat, viewFormats *[]gputypes.TextureFormat, coreTexture *core.Texture) *Texture {
	return &Texture{hal: halTexture, device: device, format: format, viewFormats: viewFormats, coreTexture: coreTexture}
}

// NewSurfaceTexture builds a texture that borrows a surface image: it carries
// the surface and lease so the HAL handle is usable only for that acquisition.
func NewSurfaceTexture(halTexture hal.Texture, device core.DeviceView, format gputypes.TextureFormat, surface *core.Surface, surfaceLease uint64, coreTexture *core.Texture) *Texture {
	return &Texture{hal: halTexture, device: device, format: format, surface: surface, surfaceLease: surfaceLease, coreTexture: coreTexture}
}

// NewDetachedTexture builds a texture that carries only the descriptor's
// metadata: no device and no HAL object. It backs wgpu's detached-texture
// factory; Release on it is a no-op.
func NewDetachedTexture(format gputypes.TextureFormat, viewFormats *[]gputypes.TextureFormat) *Texture {
	return &Texture{format: format, viewFormats: viewFormats}
}

// HAL returns the HAL texture, or nil for a texture without one.
func (t *Texture) HAL() hal.Texture {
	if t == nil {
		return nil
	}
	return t.hal
}

// CoreTexture returns the core.Texture the encoder and queue track usage
// against, or nil for a texture without one.
func (t *Texture) CoreTexture() *core.Texture {
	if t == nil {
		return nil
	}
	return t.coreTexture
}

// Surface returns the surface a surface texture borrows from, or nil.
func (t *Texture) Surface() *core.Surface {
	if t == nil {
		return nil
	}
	return t.surface
}

// SurfaceLease returns the acquisition lease of a surface texture.
func (t *Texture) SurfaceLease() uint64 {
	if t == nil {
		return 0
	}
	return t.surfaceLease
}

// ResolveHAL is the single boundary from a public texture wrapper to HAL for
// new work: a released texture cannot be recorded into new commands.
// Surface textures are borrowed and are usable only for their acquisition.
// Encoder validation and HAL conversion may call this more than once during
// one operation; acquisition and presentation are serialized by the render
// loop, so the lease remains stable across those calls.
func (t *Texture) ResolveHAL() hal.Texture {
	if t == nil || t.released {
		return nil
	}
	return t.RecordedHAL()
}

// RecordedHAL returns the HAL texture for work already recorded against t —
// a command buffer or a pending queue write being submitted — or nil when
// that work can no longer run. Release() is Rust's drop, not destroy()
// (ADR-056): recorded work holds a reference, so a released texture with a
// reference count stays alive until the work completes, as an Arc<Texture>
// held by Rust wgpu-core's trackers does, and Submit must accept it
// (P1-CORE-020). The work cannot run when the device is gone, when a surface
// texture's acquisition has ended, or when a texture without a reference
// count (wrapped from HAL) was released, since that queued its destruction.
func (t *Texture) RecordedHAL() hal.Texture {
	if t == nil || t.hal == nil || t.device == nil || t.device.IsReleased() {
		return nil
	}
	if t.surface != nil {
		if !t.surface.AcquisitionValid(t.surfaceLease) {
			return nil
		}
		return t.hal
	}
	if t.released && t.Ref() == nil {
		return nil
	}
	return t.hal
}

// Format returns the texture format.
func (t *Texture) Format() gputypes.TextureFormat { return t.format }

// ViewFormatList returns the view formats the texture was created with.
func (t *Texture) ViewFormatList() []gputypes.TextureFormat {
	if t.viewFormats == nil {
		return nil
	}
	return *t.viewFormats
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
	// HAL object: Release only marks it released. Reaching the deferred
	// destroy would dereference the nil device view.
	if t.device == nil {
		t.released = true
		return
	}
	t.released = true

	if ref := t.Ref(); ref != nil {
		ref.Drop()
		return
	}
	// No reference count (a wrapped HAL texture): destroy it after the last
	// submission.
	var td *track.TrackingData
	if t.coreTexture != nil {
		td = t.coreTexture.TrackingData()
	}
	core.DestroyTextureDeferred(t.device, t.hal, td)
}

// Ref returns the texture's reference count, or nil for a texture that has
// none: a borrowed surface texture or a wrapped HAL texture.
func (t *Texture) Ref() *core.ResourceRef {
	if t == nil || t.coreTexture == nil {
		return nil
	}
	return t.coreTexture.Ref
}

// NeedsStencilBlit reports whether an upload into aspect of t must be lowered
// to the stencil blit (core.Device.StencilUploadNeedsBlit).
func (t *Texture) NeedsStencilBlit(aspect gputypes.TextureAspect) bool {
	return t != nil && t.coreTexture != nil && t.coreTexture.Device().StencilUploadNeedsBlit(t.format, aspect)
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

// TextureViewInit is the construction value for NewTextureView. It is
// decomposed into the view's fields on entry and never stored, so a view's
// invariants stay with the view; the frontend fills exactly the fields the
// descriptor resolved (a surface view leaves the array-layer range unknown).
type TextureViewInit struct {
	HAL             hal.TextureView
	Device          core.DeviceView
	Parent          *Texture
	Surface         *core.Surface
	SurfaceLease    uint64
	Ref             *core.ResourceRef
	Dimension       gputypes.TextureViewDimension
	BaseMipLevel    uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
	MipLevelCount   uint32
	Format          gputypes.TextureFormat
	Aspect          gputypes.TextureAspect
}

// NewTextureView builds a texture view from a construction value.
func NewTextureView(init *TextureViewInit) *TextureView {
	return &TextureView{
		hal:             init.HAL,
		device:          init.Device,
		texture:         init.Parent,
		surface:         init.Surface,
		surfaceLease:    init.SurfaceLease,
		ref:             init.Ref,
		dimension:       init.Dimension,
		baseMipLevel:    init.BaseMipLevel,
		baseArrayLayer:  init.BaseArrayLayer,
		arrayLayerCount: init.ArrayLayerCount,
		mipLevelCount:   init.MipLevelCount,
		format:          init.Format,
		aspect:          init.Aspect,
	}
}

// HAL returns the HAL texture view.
func (v *TextureView) HAL() hal.TextureView {
	if v == nil {
		return nil
	}
	return v.hal
}

// Ref returns the view's reference count, or nil for a view without one.
func (v *TextureView) Ref() *core.ResourceRef {
	if v == nil {
		return nil
	}
	return v.ref
}

// Parent returns the parent Texture this view was created from, without the
// released guard: recorded command state reads the parent even after Release,
// as it did before the extraction.
func (v *TextureView) Parent() *Texture {
	if v == nil {
		return nil
	}
	return v.texture
}

// Dimension returns the view's resolved dimension.
func (v *TextureView) Dimension() gputypes.TextureViewDimension {
	if v == nil {
		return gputypes.TextureViewDimensionUndefined
	}
	return v.dimension
}

// BaseMipLevel returns the view's base mip level.
func (v *TextureView) BaseMipLevel() uint32 {
	if v == nil {
		return 0
	}
	return v.baseMipLevel
}

// BaseArrayLayer returns the view's base array layer.
func (v *TextureView) BaseArrayLayer() uint32 {
	if v == nil {
		return 0
	}
	return v.baseArrayLayer
}

// ArrayLayerCount returns the view's array layer count, 0 when not known.
func (v *TextureView) ArrayLayerCount() uint32 {
	if v == nil {
		return 0
	}
	return v.arrayLayerCount
}

// MipLevelCount returns the view's mip count, 0 when not known.
func (v *TextureView) MipLevelCount() uint32 {
	if v == nil {
		return 0
	}
	return v.mipLevelCount
}

// ViewFormat returns the view descriptor's format (Undefined: the texture's).
func (v *TextureView) ViewFormat() gputypes.TextureFormat {
	if v == nil {
		return gputypes.TextureFormatUndefined
	}
	return v.format
}

// Aspect returns the view's aspect.
func (v *TextureView) Aspect() gputypes.TextureAspect {
	if v == nil {
		return 0
	}
	return v.aspect
}

// InternalUsage returns the usages only the frontend's own commands use the
// view with.
func (v *TextureView) InternalUsage() gputypes.TextureUsage {
	if v == nil {
		return 0
	}
	return v.internalUsage
}

// SetInternalUsage records the usages only the frontend's own commands use the
// view with (the stencil upload blit), after the view is created.
func (v *TextureView) SetInternalUsage(usage gputypes.TextureUsage) {
	if v == nil {
		return
	}
	v.internalUsage = usage
}

// ResolveHAL is the single boundary from a public texture-view wrapper to HAL.
// Like Texture.ResolveHAL, callers may resolve repeatedly within one serialized
// encoder operation without extending the surface acquisition lifetime.
func (v *TextureView) ResolveHAL() hal.TextureView {
	if v == nil || v.released || v.hal == nil || v.device == nil || v.device.IsReleased() {
		return nil
	}
	if v.surface != nil && !v.surface.AcquisitionValid(v.surfaceLease) {
		return nil
	}
	if v.surface != nil && v.texture != nil && v.texture.ResolveHAL() == nil {
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
		core.DestroyTextureViewDeferred(v.device, v.hal)
	}
}
