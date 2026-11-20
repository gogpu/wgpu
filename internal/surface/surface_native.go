//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package surface holds the surface domain's implementation (ADR-070 S10).
//
// This context is the native one: the implementation configures a HAL surface
// through a core.Surface, acquires swapchain images through it and presents
// them through the HAL queue. The device is reached through the DeviceHost
// seam (seams_native.go); the instance lives in internal/instance since S12,
// and the root keeps the surface's instance identity pointer and passes the one
// HAL value the implementation needs — the hal.Instance a backend's raw surface
// is created on — in as a parameter.
package surface

import (
	"errors"
	"fmt"
	"image"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// Surface is the implementation of a platform rendering surface: the HAL
// surface per enabled backend, the core.Surface that owns the active one and
// its lifecycle state machine, the cached swapchain texture, and the seam view
// of the device it is configured against.
//
// The root package keeps the declaration layer over it, including the public
// type, the instance pointer and the target ownership source. This type holds
// only what the surface lifecycle needs.
type Surface struct {
	// core is the core.Surface that owns the active HAL surface and enforces
	// the state machine Unconfigured -> Configured -> Acquired -> Configured.
	core *core.Surface

	// device is the seam view of the device the surface is configured against;
	// nil until Configure succeeds.
	device DeviceHost

	released bool

	// target is the platform target, converted once by the root declaration
	// layer from the public SurfaceTargetUnsafe. targetSet is false for a
	// surface wrapped around an already-created HAL surface, which owns no
	// target.
	target    hal.SurfaceTarget
	targetSet bool

	// currentBackend is the backend of the active HAL surface, and
	// surfaceCreated reports whether core points at a HAL surface.
	currentBackend gputypes.Backend
	surfaceCreated bool

	// halSurfaces owns one successfully created surface per enabled backend,
	// matching Rust wgpu's surface_per_backend representation. core points at
	// exactly one of these at a time to keep its lifecycle state machine small.
	halSurfaces map[gputypes.Backend]hal.Surface

	// swapchainTexture is the cached core.Texture for surface texture tracking.
	// A swapchain has a bounded number of images (typically 2-3). All of them
	// share a single TrackerIndex because surface texture usage is always the
	// same (RenderAttachment) and the swapchain serializes image access.
	// Created lazily on first GetCurrentTexture(), destroyed on Unconfigure/
	// Release/reconfigure. This prevents TrackerIndex leak: without caching,
	// a new core.Texture (and TrackerIndex) was allocated every frame, causing
	// monotonic index growth and unbounded memory usage (#307).
	swapchainTexture *core.Texture
}

// Init is the construction value for New: everything an Instance-created
// surface carries before it is configured. The root declaration layer fills it
// after it has created the HAL surfaces and the core.Surface.
type Init struct {
	// Core is the core.Surface that owns the initial HAL surface.
	Core *core.Surface

	// Target is the already-converted platform target; TargetSet is false when
	// the surface owns none (a wrapped HAL surface).
	Target    hal.SurfaceTarget
	TargetSet bool

	// Backend is the backend of the initial HAL surface.
	Backend gputypes.Backend

	// HALSurfaces is the created surface per backend, and SurfaceCreated
	// reports that core already points at one.
	HALSurfaces    map[gputypes.Backend]hal.Surface
	SurfaceCreated bool
}

// New returns the implementation of a surface created from a platform target.
func New(init Init) *Surface {
	return &Surface{
		core:           init.Core,
		target:         init.Target,
		targetSet:      init.TargetSet,
		currentBackend: init.Backend,
		halSurfaces:    init.HALSurfaces,
		surfaceCreated: init.SurfaceCreated,
	}
}

// NewFromHAL returns the implementation of a surface wrapped around an
// already-created HAL surface. Such a surface owns no platform target, so a
// backend switch cannot re-create it.
func NewFromHAL(coreSurface *core.Surface) *Surface {
	return &Surface{core: coreSurface}
}

// Core returns the core surface state the surface records against, or nil
// after Release.
func (s *Surface) Core() *core.Surface {
	if s == nil {
		return nil
	}
	return s.core
}

// Released reports whether the surface has been released. A nil receiver is
// the root declaration layer's zero value, which is not released.
func (s *Surface) Released() bool {
	return s != nil && s.released
}

// SameDevice reports whether the surface is configured against coreDevice.
// The seam exposes the device's core state, not a root pointer, so identity is
// compared there.
func (s *Surface) SameDevice(coreDevice *core.Device) bool {
	if s == nil || s.device == nil || coreDevice == nil {
		return false
	}
	coreDeviceView := s.device.Core()
	return coreDeviceView != nil && coreDeviceView == coreDevice
}

// HALSurfaceForBackend returns the retained surface that belongs to an
// adapter's backend. Legacy surfaces wrapped from one HAL handle have no
// backend map, so they retain the historical active-surface fallback.
func (s *Surface) HALSurfaceForBackend(backend gputypes.Backend) hal.Surface {
	if s == nil || s.released || s.core == nil {
		return nil
	}
	if len(s.halSurfaces) != 0 {
		return s.halSurfaces[backend]
	}
	return s.core.RawSurface()
}

// HALSurfacesForAdapterRequest returns a copy of the surface per backend for
// an adapter request: the map is the surface's own, so a caller mutating the
// result cannot orphan a retained HAL surface.
func (s *Surface) HALSurfacesForAdapterRequest() map[gputypes.Backend]hal.Surface {
	if s == nil || s.released || s.core == nil {
		return nil
	}
	result := make(map[gputypes.Backend]hal.Surface, len(s.halSurfaces))
	for backend, halSurface := range s.halSurfaces {
		result[backend] = halSurface
	}
	if len(result) == 0 {
		if raw := s.core.RawSurface(); raw != nil {
			result[s.currentBackend] = raw
		}
	}
	return result
}

// Configure configures the surface for presentation.
//
// host is the seam view of the device and config its mirror of the public
// configuration. sameInstance is the root declaration layer's cross-instance
// decision (the instance is a root object), and halInstance is the HAL
// instance the backend's raw surface is created on, resolved by the root for
// the device's backend; it may be nil when the surface keeps its existing
// HAL surface.
//
// Must be called before GetCurrentTexture().
func (s *Surface) Configure(host DeviceHost, config *SurfaceConfiguration, sameInstance bool, halInstance hal.Instance) error {
	// A nil receiver is the root declaration layer's zero value: released is
	// false, so the pre-move answers are the nil-argument ones and ErrReleased
	// after them.
	if s == nil {
		if config == nil {
			return fmt.Errorf("wgpu: surface configuration is nil")
		}
		if host == nil {
			return fmt.Errorf("wgpu: device is nil")
		}
		return core.ErrReleased
	}
	if s.released {
		return core.ErrReleased
	}
	if config == nil {
		return fmt.Errorf("wgpu: surface configuration is nil")
	}
	if host == nil {
		return fmt.Errorf("wgpu: device is nil")
	}
	if host.IsReleased() {
		return core.ErrReleased
	}
	// Standard API objects must share an Instance so device teardown can find
	// and retire every configured surface. Explicit HAL wrappers remain usable
	// when both objects are intentionally unowned (instance == nil).
	if !sameInstance {
		return fmt.Errorf("wgpu: surface and device belong to different instances")
	}
	// Reject reconfiguration while an image is acquired before touching the
	// backend surface. ensureHALSurface may destroy and recreate the raw
	// surface; doing that first would leave the active SurfaceTexture pointing
	// at a destroyed image when core.Configure rejects the state transition.
	if s.core == nil {
		return core.ErrReleased
	}
	if s.core.State() == core.SurfaceStateAcquired {
		return core.ErrSurfaceConfigureWhileAcquired
	}

	halConfig := &hal.SurfaceConfiguration{
		Width:       config.Width,
		Height:      config.Height,
		Format:      config.Format,
		Usage:       config.Usage,
		PresentMode: config.PresentMode,
		AlphaMode:   config.AlphaMode,
	}

	// Create or re-create the HAL surface on the correct backend's HAL
	// instance. Surface creation is deferred from CreateSurface() to here
	// because we need to know the device's backend. Creating a Vulkan surface
	// then destroying it (when device is software) corrupts GDI state on some
	// drivers.
	if err := s.ensureHALSurface(host.Core().Backend(), halInstance); err != nil {
		return err
	}

	// Destroy the cached swapchain texture on reconfigure. The new swapchain
	// may have different format, usage, or dimensions. A fresh core.Texture
	// will be lazily created on the next GetCurrentTexture().
	s.destroySwapchainTexture()

	s.device = host
	return s.core.Configure(host.Core(), halConfig)
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	if s == nil || s.released {
		return
	}
	s.destroySwapchainTexture()
	s.core.Unconfigure()
}

// GetCurrentTexture acquires the next texture for rendering.
// Returns the surface texture and whether the surface is suboptimal.
//
// The texture is returned by value: the root declaration layer embeds it, so an
// acquisition costs the one allocation the public wrapper is, like before the
// move (ADR-070 §15.7).
func (s *Surface) GetCurrentTexture() (SurfaceTexture, bool, error) {
	if s == nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: surface not configured")
	}
	if s.released {
		return SurfaceTexture{}, false, core.ErrReleased
	}
	if s.device == nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: surface not configured")
	}

	acquired, lease, err := s.core.AcquireTextureWithLease(nil)
	if err != nil {
		s.reportOperationError(err)
		return SurfaceTexture{}, false, err
	}

	// Reuse the cached core.Texture for TrackerIndex stability. A swapchain
	// has a bounded number of images (typically 2-3), and they all share a
	// single TrackerIndex because the swapchain serializes image access.
	// Without caching, a new core.Texture (with a new TrackerIndex) was
	// created every frame, causing monotonic index growth and unbounded
	// memory consumption (#307).
	//
	// The coreTexture is shared with any Texture/TextureView wrappers
	// created from this SurfaceTexture. At submit time, the TrackerIndex
	// is used by populateTextureScope to record surface texture usage in
	// the command buffer's TextureUsageScope for barrier generation.
	if s.swapchainTexture == nil && s.device != nil && s.device.Core() != nil {
		cfg := s.core.Config()
		if cfg != nil {
			s.swapchainTexture = core.NewTexture(
				acquired.Texture, s.device.Core(),
				cfg.Format,
				gputypes.TextureDimension2D,
				cfg.Usage,
				gputypes.Extent3D{
					Width:              cfg.Width,
					Height:             cfg.Height,
					DepthOrArrayLayers: 1,
				},
				1, 1, "surface",
			)
		}
	}

	return SurfaceTexture{
		hal:         acquired.Texture,
		surface:     s,
		device:      s.device,
		lease:       lease,
		coreTexture: s.swapchainTexture,
	}, acquired.Suboptimal, nil
}

// Present presents a surface texture to the screen.
func (s *Surface) Present(texture *SurfaceTexture) error {
	return s.PresentWithDamage(texture, nil)
}

// PresentWithDamage presents a surface texture to the screen, passing optional
// damage rectangles to the compositor.
//
// damageRects specifies which regions of the surface changed this frame
// (physical pixels, top-left origin). When nil or empty, the entire surface
// is presented — identical to Present(). Backends that support damage rects
// (software partial blit, and in future: DX12 Present1, Vulkan
// VK_KHR_incremental_present, GLES eglSwapBuffersWithDamageKHR) use them
// as compositor hints; others accept and ignore them.
func (s *Surface) PresentWithDamage(texture *SurfaceTexture, damageRects []image.Rectangle) error {
	if s == nil {
		return fmt.Errorf("wgpu: surface not configured")
	}
	if s.released {
		return core.ErrReleased
	}
	if s.device == nil {
		return fmt.Errorf("wgpu: surface not configured")
	}
	if s.device.HALQueue() == nil {
		return fmt.Errorf("wgpu: queue not available")
	}

	if texture == nil {
		return fmt.Errorf("wgpu: surface texture is nil")
	}
	if texture.surface != s || !texture.isUsable() {
		return core.ErrReleased
	}

	err := s.core.PresentWithDamage(s.device.HALQueue(), damageRects)
	if err != nil {
		s.reportOperationError(err)
	}
	return err
}

// reportOperationError records a driver device loss observed on the acquire or
// present path and delivers the callbacks the observation queued
// (P1-API-042 / #417, C3 and C4): the core surface's own lock is already
// released when this runs, so no callback executes under it.
//
// Only hal.ErrDeviceLost is classified as device loss. ErrSurfaceLost and
// ErrSurfaceOutdated keep their own identity and reach the caller unchanged —
// a lost surface says nothing about the device, and escalating it would report
// a loss that never happened (the shape's explicit "any escalation is
// documented, never implied").
func (s *Surface) reportOperationError(err error) {
	if s == nil || err == nil || s.device == nil {
		return
	}
	if errors.Is(err, hal.ErrDeviceLost) {
		s.device.MarkDeviceLost(err.Error())
	}
	s.device.DispatchDeviceCallbacks()
}

// SetPresentsWithTransaction toggles Core Animation transaction-based present
// on Metal surfaces. Enable during macOS live window resize so the drawable
// swap commits atomically with the resize CA transaction (Flutter/wgpu #3756
// pattern); disable for normal rendering. No-op on non-Metal backends.
func (s *Surface) SetPresentsWithTransaction(enabled bool) {
	if s == nil || s.released || s.core == nil {
		return
	}
	raw := s.core.RawSurface()
	if raw == nil {
		return
	}
	if tp, ok := raw.(interface{ SetPresentsWithTransaction(bool) }); ok {
		tp.SetPresentsWithTransaction(enabled)
	}
}

// PresentPixels writes RGBA pixel data directly to the surface and presents it
// in a single operation, bypassing the WebGPU render pass pipeline entirely.
//
// On the software backend this performs RGBA→BGRA swizzle into the DIB section
// framebuffer and immediately BitBlt's to the window — the fastest path for
// CPU-rendered content (1 copy vs 3 in the standard pipeline). No AcquireTexture,
// render pass, or Present call is needed.
//
// damageRects restricts the window blit to specific regions (nil = full surface).
// The surface must be configured. If a texture is currently acquired, it is
// discarded automatically (stale texture cleanup).
//
// Returns an error if the backend does not support PresentPixels (only the
// software backend implements this extension).
func (s *Surface) PresentPixels(data []byte, width, height uint32, damageRects []image.Rectangle) error {
	if s == nil {
		return fmt.Errorf("wgpu: surface not configured")
	}
	if s.released {
		return core.ErrReleased
	}
	if s.device == nil {
		return fmt.Errorf("wgpu: surface not configured")
	}
	return s.core.PresentPixels(data, width, height, damageRects)
}

// WritePixels copies RGBA pixel data directly into the surface framebuffer,
// bypassing render pass and texture upload. On software backend this is a
// single-pass RGBA→BGRA swizzle+copy into the DIB section (1 copy vs 3).
// On GPU backends, falls back to WriteTexture + present (no fast path).
//
// Call between GetCurrentTexture() and Present(). No render pass needed.
// Returns ErrReleased if the surface is not configured.
func (s *Surface) WritePixels(data []byte, width, height uint32) error {
	if s == nil || s.released || s.core == nil {
		return core.ErrReleased
	}
	raw := s.core.RawSurface()
	if raw == nil {
		return core.ErrReleased
	}
	if pw, ok := raw.(hal.PixelWriter); ok {
		return pw.WritePixels(data, width, height)
	}
	return fmt.Errorf("wgpu: WritePixels not supported on this backend")
}

// ReadPixels captures the current surface framebuffer as an owned, tightly
// packed RGBA8 snapshot in top-left, row-major order.
//
// This is a non-WebGPU extension implemented by the Pure-Go software backend.
// The surface must be configured, and any acquired texture must be presented
// or discarded before capture.
func (s *Surface) ReadPixels() ([]byte, error) {
	if s == nil || s.released || s.core == nil {
		return nil, core.ErrReleased
	}

	switch s.core.State() {
	case core.SurfaceStateUnconfigured:
		return nil, fmt.Errorf("wgpu: surface not configured")
	case core.SurfaceStateAcquired:
		return nil, fmt.Errorf("wgpu: surface texture is still acquired; present or discard it before ReadPixels")
	}

	reader, ok := s.core.RawSurface().(hal.PixelReader)
	if !ok {
		return nil, fmt.Errorf("wgpu: ReadPixels not supported on this backend")
	}
	pixels := reader.ReadPixels()
	if pixels == nil {
		return nil, fmt.Errorf("wgpu: ReadPixels returned no pixel data")
	}
	return pixels, nil
}

// ActualExtent returns the actual swapchain dimensions after driver clamping.
//
// On Vulkan, the driver may clamp the requested extent to its supported range
// (e.g., on X11 HiDPI where the compositor reports physical pixels that differ
// from the application's logical pixels). The returned values reflect what the
// swapchain was actually created with, which may differ from the configured
// SurfaceConfiguration.Width/Height.
//
// On non-Vulkan backends (DX12, Metal, GLES, Software), the returned values
// match the configured dimensions since those backends do not clamp the extent.
// Returns (0, 0) if the surface is not configured.
//
// Use this to size MSAA resolve textures, offscreen targets, and any other
// resources that must match the true swapchain size.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s == nil || s.released {
		return 0, 0
	}
	raw := s.core.RawSurface()
	if raw == nil {
		return 0, 0
	}
	return raw.ActualExtent()
}

// DiscardTexture discards the acquired surface texture without presenting it.
// Use this if rendering failed or was canceled. If no texture is currently
// acquired, this is a no-op.
func (s *Surface) DiscardTexture() {
	if s == nil || s.released {
		return
	}
	s.core.DiscardTexture()
}

// DiscardForDevice discards the acquired texture while the given core device is
// still alive, for the device-teardown scan: the surface must be configured
// against coreDevice, or the call is a no-op.
func (s *Surface) DiscardForDevice(coreDevice *core.Device) {
	if s == nil || s.core == nil || !s.SameDevice(coreDevice) {
		return
	}
	s.core.DiscardTexture()
}

// RetireDevice retires the surface's view of a device being destroyed: the
// swapchain cache is released so its TrackerIndex goes back to the device's
// allocator, the core surface is told the device is gone, and the seam is
// cleared.
func (s *Surface) RetireDevice(coreDevice *core.Device) {
	if s == nil || s.core == nil || !s.SameDevice(coreDevice) {
		return
	}
	// The swapchainTexture holds a reference to this device's TrackerIndex
	// allocator. Destroy it before retiring the device to prevent leaks
	// and dangling references.
	s.destroySwapchainTexture()
	s.core.RetireDevice(coreDevice)
	s.device = nil
}

func (s *Surface) createHALSurface(backend gputypes.Backend, halInstance hal.Instance) (hal.Surface, error) {
	if halInstance == nil {
		return nil, fmt.Errorf("wgpu: no HAL instance for backend %v", backend)
	}
	if !s.targetSet {
		return nil, errNoPlatformTarget
	}
	return createHALSurfaceForTarget(halInstance, backend, s.target)
}

func createHALSurfaceForTarget(targetInstance hal.Instance, backend gputypes.Backend, target hal.SurfaceTarget) (hal.Surface, error) {
	halSurface, err := targetInstance.CreateSurface(target)
	if errors.Is(err, hal.ErrUnsupportedSurfaceTarget) {
		// The public sentinel is the root package's; the declaration layer
		// wraps this detail with it after the call (mapSurfaceConfigureError).
		return nil, fmt.Errorf("backend %v: %w", backend, err)
	}
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface for backend %v: %w", backend, err)
	}
	if halSurface == nil {
		return nil, fmt.Errorf("wgpu: backend %v returned a nil surface", backend)
	}
	return halSurface, nil
}

// errNoPlatformTarget reports a surface that must re-create its HAL surface but
// owns no platform target: a surface wrapped around an already-created HAL
// surface. The pre-move code failed the same configuration at the target
// conversion; the surface cannot name the root package's sentinel, so the
// declaration layer reports the root one.
var errNoPlatformTarget = errors.New("wgpu: invalid surface target: target kind is unknown")

// ensureHALSurface creates or re-creates the HAL surface for the given backend.
func (s *Surface) ensureHALSurface(backend gputypes.Backend, halInstance hal.Instance) error {
	if s.surfaceCreated && s.currentBackend == backend {
		return nil
	}
	halSurface := s.halSurfaces[backend]
	if halSurface == nil {
		var err error
		halSurface, err = s.createHALSurface(backend, halInstance)
		if err != nil {
			return err
		}
		if s.halSurfaces == nil {
			s.halSurfaces = make(map[gputypes.Backend]hal.Surface)
		}
		s.halSurfaces[backend] = halSurface
	}
	if s.core.State() != core.SurfaceStateUnconfigured {
		s.core.Unconfigure()
	}
	s.core.SetRawSurface(halSurface)
	s.currentBackend = backend
	s.surfaceCreated = true
	return nil
}

// Release releases the surface. The root declaration layer unregisters it from
// its instance and drops the target ownership source after this returns.
func (s *Surface) Release() {
	if s == nil || s.released {
		return
	}
	s.released = true
	s.destroySwapchainTexture()
	if s.core != nil {
		DestroyHALSurfaces(s.core, s.halSurfaces, s.currentBackend, s.surfaceCreated)
	}
	s.core = nil
	s.halSurfaces = nil
}

// destroySwapchainTexture releases the cached core.Texture's TrackerIndex.
// The core.Texture for a swapchain does not own the HAL texture (that is
// owned by the swapchain), so we only release the tracking data to recycle
// the TrackerIndex. We must NOT call core.Texture.Destroy() which would
// attempt to destroy the swapchain's HAL image.
func (s *Surface) destroySwapchainTexture() {
	if s.swapchainTexture != nil {
		td := s.swapchainTexture.TrackingData()
		if td != nil {
			td.Release()
		}
		s.swapchainTexture = nil
	}
}

// DestroyHALSurfaces destroys the core surface's active HAL surface and every
// retained surface except the one core still points at (currentSet names it).
//
// It is exported because both the root declaration layer's construction race
// and the implementation's Release call it, and a second copy would be a
// divergent definition of "which surface is still owned".
func DestroyHALSurfaces(coreSurface *core.Surface, surfaces map[gputypes.Backend]hal.Surface, currentBackend gputypes.Backend, currentSet bool) {
	if coreSurface != nil {
		coreSurface.Destroy()
	}
	for backend, halSurface := range surfaces {
		if currentSet && backend == currentBackend {
			continue
		}
		if halSurface != nil {
			halSurface.Destroy()
		}
	}
}

// SurfaceTexture is a texture acquired from a surface for rendering.
type SurfaceTexture struct {
	hal         hal.SurfaceTexture
	surface     *Surface
	device      DeviceHost
	lease       uint64
	coreTexture *core.Texture // for TrackerIndex-based barrier generation
}

func (st *SurfaceTexture) isUsable() bool {
	if st == nil || st.hal == nil || st.surface == nil ||
		st.surface.released || st.surface.core == nil ||
		st.device == nil || st.device.IsReleased() {
		return false
	}
	return st.surface.core.AcquisitionValid(st.lease)
}

// Resource returns the resource-package texture that borrows the current
// surface image, or nil when the acquisition is no longer usable. It is the
// handle the root declaration layer wraps for AsTexture, and the parent its
// view is created against.
func (st *SurfaceTexture) Resource() *resource.Texture {
	if !st.isUsable() {
		return nil
	}
	return st.borrowedResource()
}

// borrowedResource builds the resource texture that borrows this surface
// texture. The format is the surface format: copy and upload validation reads
// it from the wrapper.
func (st *SurfaceTexture) borrowedResource() *resource.Texture {
	var format gputypes.TextureFormat
	if st.coreTexture != nil {
		format = st.coreTexture.Format()
	} else if cfg := st.surface.core.Config(); cfg != nil {
		format = cfg.Format
	}
	return resource.NewSurfaceTexture(st.hal, st.device, format, st.surface.core, st.lease, st.coreTexture)
}

// CreateView creates a texture view of this surface texture and returns the
// resource-package view; the root declaration layer wraps it.
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*resource.TextureView, error) {
	if !st.isUsable() {
		return nil, core.ErrReleased
	}
	halDevice := st.device.HALDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	var halDesc *hal.TextureViewDescriptor
	if desc != nil {
		halDesc = &hal.TextureViewDescriptor{
			Label:           desc.Label,
			Format:          desc.Format,
			Dimension:       desc.Dimension,
			Aspect:          desc.Aspect,
			BaseMipLevel:    desc.BaseMipLevel,
			MipLevelCount:   desc.MipLevelCount,
			BaseArrayLayer:  desc.BaseArrayLayer,
			ArrayLayerCount: desc.ArrayLayerCount,
		}
	}

	halView, err := halDevice.CreateTextureView(st.hal, halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface texture view: %w", err)
	}

	// A surface texture is a single-layer 2D texture with one mip level.
	parent := st.borrowedResource()
	init := &resource.TextureViewInit{
		HAL:          halView,
		Device:       st.device,
		Parent:       parent,
		Surface:      st.surface.core,
		SurfaceLease: st.lease,
		Dimension:    gputypes.TextureViewDimension2D,
	}
	if halDesc != nil {
		init.Dimension = halDesc.Dimension
		if init.Dimension == gputypes.TextureViewDimensionUndefined {
			init.Dimension = gputypes.TextureViewDimension2D
		}
		init.BaseMipLevel = halDesc.BaseMipLevel
		init.MipLevelCount = halDesc.MipLevelCount
		init.Format = halDesc.Format
		init.Aspect = halDesc.Aspect
	}
	return resource.NewTextureView(init), nil
}
