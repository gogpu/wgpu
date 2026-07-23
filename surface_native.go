//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Surface represents a platform rendering surface (e.g., a window).
//
// Surface delegates lifecycle management to core.Surface, which enforces
// the state machine: Unconfigured -> Configured -> Acquired -> Configured.
type Surface struct {
	core     *core.Surface
	instance *Instance
	device   *Device
	released bool

	// targetSource is retained only for CreateSurfaceFromTarget. Raw handles
	// passed through CreateSurfaceUnsafe or the compatibility CreateSurface
	// method remain entirely caller-owned.
	targetSource SurfaceTarget

	// target is stored for a backend whose initial surface creation failed but
	// whose device is later supplied explicitly.
	target         SurfaceTargetUnsafe
	currentBackend gputypes.Backend // backend type of the current HAL surface
	surfaceCreated bool             // true while core points at a HAL surface
	// halSurfaces owns one successfully created surface per enabled backend,
	// matching Rust wgpu's surface_per_backend representation. core points at
	// exactly one of these at a time to keep its lifecycle state machine small.
	halSurfaces map[gputypes.Backend]hal.Surface
}

// CreateSurface creates a rendering surface from legacy platform-specific
// handles. New code should prefer CreateSurfaceFromTarget or
// CreateSurfaceUnsafe so the target kind and ownership contract are explicit.
// displayHandle and windowHandle are platform-specific:
//   - Windows: displayHandle=0, windowHandle=HWND
//   - macOS: displayHandle=0, windowHandle=CAMetalLayer*
//   - Linux/X11: displayHandle=Display*, windowHandle=Window
//   - Linux/Wayland: displayHandle=wl_display*, windowHandle=wl_surface*
//   - Android: displayHandle ignored, windowHandle=ANativeWindow*
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (*Surface, error) {
	return i.createSurface(surfaceTargetFromLegacyHandles(displayHandle, windowHandle), nil)
}

// CreateSurfaceFromTarget samples a provider once and retains it until the
// surface is released. The provider's native objects must remain valid for the
// same lifetime.
func (i *Instance) CreateSurfaceFromTarget(target SurfaceTarget) (*Surface, error) {
	if i.isReleased() {
		return nil, ErrReleased
	}
	rawTarget, err := resolveSurfaceTarget(target)
	if err != nil {
		return nil, err
	}
	return i.createSurface(rawTarget, target)
}

// CreateSurfaceUnsafe creates a surface from raw platform handles without
// retaining an ownership source. Every referenced native object must remain
// valid until the returned Surface is released.
func (i *Instance) CreateSurfaceUnsafe(target SurfaceTargetUnsafe) (*Surface, error) {
	if i.isReleased() {
		return nil, ErrReleased
	}
	if err := target.validate(); err != nil {
		return nil, err
	}
	return i.createSurface(target, nil)
}

func (i *Instance) createSurface(target SurfaceTargetUnsafe, targetSource SurfaceTarget) (*Surface, error) {
	if i.isReleased() {
		return nil, ErrReleased
	}

	entries := i.core.HALInstanceEntries()
	if len(entries) == 0 {
		return nil, fmt.Errorf("wgpu: no HAL instance available for surface creation")
	}

	halTarget, err := target.halTarget()
	if err != nil {
		return nil, err
	}
	halSurfaces, initialBackend, err := createHALSurfaces(entries, halTarget)
	if err != nil {
		return nil, err
	}
	return i.adoptCreatedSurface(target, targetSource, halSurfaces, initialBackend)
}

func (i *Instance) adoptCreatedSurface(
	target SurfaceTargetUnsafe,
	targetSource SurfaceTarget,
	halSurfaces map[gputypes.Backend]hal.Surface,
	initialBackend gputypes.Backend,
) (*Surface, error) {
	halSurface := halSurfaces[initialBackend]
	coreSurface := core.NewSurface(halSurface, "")
	surface := &Surface{
		core:           coreSurface,
		target:         target,
		targetSource:   targetSource,
		currentBackend: initialBackend,
		surfaceCreated: true,
		halSurfaces:    halSurfaces,
	}
	if err := i.adoptSurface(surface); err != nil {
		destroyHALSurfaces(coreSurface, halSurfaces, initialBackend, true)
		return nil, err
	}
	return surface, nil
}

func createHALSurfaces(entries []core.HALInstanceEntry, target hal.SurfaceTarget) (map[gputypes.Backend]hal.Surface, gputypes.Backend, error) {
	surfaces := make(map[gputypes.Backend]hal.Surface, len(entries))
	errs := make([]error, 0, len(entries))
	allUnsupported := true
	var firstBackend gputypes.Backend
	firstSet := false

	for _, entry := range entries {
		raw, err := entry.Instance.CreateSurface(target)
		if err == nil && raw == nil {
			err = fmt.Errorf("backend %v returned a nil surface", entry.Backend)
		}
		if err != nil {
			if !errors.Is(err, hal.ErrUnsupportedSurfaceTarget) {
				allUnsupported = false
			}
			errs = append(errs, fmt.Errorf("backend %v: %w", entry.Backend, err))
			hal.Logger().Debug("wgpu: backend surface creation failed", "backend", entry.Backend, "error", err)
			continue
		}
		surfaces[entry.Backend] = raw
		if !firstSet {
			firstBackend = entry.Backend
			firstSet = true
		}
	}

	if firstSet {
		return surfaces, firstBackend, nil
	}
	joined := errors.Join(errs...)
	if allUnsupported {
		return nil, 0, fmt.Errorf("%w: no enabled backend accepted the target: %w", ErrUnsupportedSurfaceTarget, joined)
	}
	return nil, 0, fmt.Errorf("wgpu: failed to create surface for every enabled backend: %w", joined)
}

func surfaceTargetFromLegacyHandles(displayHandle, windowHandle uintptr) SurfaceTargetUnsafe {
	return surfaceTargetFromLegacyHandlesForPlatform(
		runtime.GOOS,
		os.Getenv("WAYLAND_DISPLAY"),
		displayHandle,
		windowHandle,
	)
}

func surfaceTargetFromLegacyHandlesForPlatform(goos, waylandDisplay string, displayHandle, windowHandle uintptr) SurfaceTargetUnsafe {
	switch goos {
	case "windows":
		return SurfaceTargetFromWindowsHWND(displayHandle, windowHandle)
	case "darwin":
		return SurfaceTargetFromMetalLayer(windowHandle)
	case "linux":
		if waylandDisplay != "" {
			return SurfaceTargetFromWaylandSurface(displayHandle, windowHandle)
		}
		return SurfaceTargetFromXlibWindow(displayHandle, windowHandle)
	case "android":
		return SurfaceTargetFromAndroidNativeWindow(windowHandle)
	default:
		return SurfaceTargetUnsafe{
			kind:          surfaceTargetInvalid,
			displayHandle: displayHandle,
			windowHandle:  windowHandle,
		}
	}
}

func (t SurfaceTargetUnsafe) halTarget() (hal.SurfaceTarget, error) {
	var kind hal.SurfaceTargetKind
	switch t.kind {
	case surfaceTargetWindowsHWND:
		kind = hal.SurfaceTargetWindowsHWND
	case surfaceTargetXlibWindow:
		kind = hal.SurfaceTargetXlibWindow
	case surfaceTargetWaylandSurface:
		kind = hal.SurfaceTargetWaylandSurface
	case surfaceTargetAndroidNativeWindow:
		kind = hal.SurfaceTargetAndroidNativeWindow
	case surfaceTargetMetalLayer:
		kind = hal.SurfaceTargetMetalLayer
	case surfaceTargetWebCanvasID:
		return hal.SurfaceTarget{}, fmt.Errorf("%w: Web canvas target on native backend", ErrUnsupportedSurfaceTarget)
	default:
		return hal.SurfaceTarget{}, invalidSurfaceTarget("target kind is unknown")
	}
	return hal.SurfaceTarget{
		Kind:          kind,
		DisplayHandle: t.displayHandle,
		WindowHandle:  t.windowHandle,
	}, nil
}

// Configure configures the surface for presentation.
// Must be called before GetCurrentTexture().
func (s *Surface) Configure(device *Device, config *SurfaceConfiguration) error {
	if s.released {
		return ErrReleased
	}
	if config == nil {
		return fmt.Errorf("wgpu: surface configuration is nil")
	}
	if device == nil {
		return fmt.Errorf("wgpu: device is nil")
	}
	if device.released.Load() {
		return ErrReleased
	}
	// Standard API objects must share an Instance so device teardown can find
	// and retire every configured surface. Explicit HAL wrappers remain usable
	// when both objects are intentionally unowned (instance == nil).
	if s.instance != device.instance && (s.instance != nil || device.instance != nil) {
		return fmt.Errorf("wgpu: surface and device belong to different instances")
	}
	// Reject reconfiguration while an image is acquired before touching the
	// backend surface. ensureHALSurface may destroy and recreate the raw surface;
	// doing that first would leave the active SurfaceTexture pointing at a
	// destroyed image when core.Configure rejects the state transition.
	if s.core == nil {
		return ErrReleased
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

	// Create or re-create the HAL surface on the correct backend's HAL instance.
	// Surface creation is deferred from CreateSurface() to here because we need
	// to know the device's backend. Creating a Vulkan surface then destroying it
	// (when device is software) corrupts GDI state on some drivers.
	if err := s.ensureHALSurface(device.core.Backend()); err != nil {
		return err
	}

	s.device = device
	return s.core.Configure(device.core, halConfig)
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	if s.released {
		return
	}
	s.core.Unconfigure()
}

// GetCurrentTexture acquires the next texture for rendering.
// Returns the surface texture and whether the surface is suboptimal.
//
// If a PrepareFrame hook is registered and reports changed dimensions,
// the surface is automatically reconfigured before acquiring.
func (s *Surface) GetCurrentTexture() (*SurfaceTexture, bool, error) {
	if s.released {
		return nil, false, ErrReleased
	}
	if s.device == nil {
		return nil, false, fmt.Errorf("wgpu: surface not configured")
	}

	acquired, lease, err := s.core.AcquireTextureWithLease(nil)
	if err != nil {
		return nil, false, err
	}

	return &SurfaceTexture{
		hal:     acquired.Texture,
		surface: s,
		device:  s.device,
		lease:   lease,
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
	if s.released {
		return ErrReleased
	}
	if s.device == nil {
		return fmt.Errorf("wgpu: surface not configured")
	}
	if s.device.queue == nil || s.device.queue.hal == nil {
		return fmt.Errorf("wgpu: queue not available")
	}

	if texture == nil {
		return fmt.Errorf("wgpu: surface texture is nil")
	}
	if texture.surface != s || !texture.isUsable() {
		return ErrReleased
	}

	return s.core.PresentWithDamage(s.device.queue.hal, damageRects)
}

// SetPrepareFrame registers a platform hook called before each GetCurrentTexture.
// If the hook returns changed=true with new dimensions, the surface is automatically
// reconfigured. This is the integration point for HiDPI/DPI change handling:
//   - macOS Metal: read CAMetalLayer.contentsScale
//   - Windows: handle WM_DPICHANGED
//   - Wayland: read wl_output.scale
//
// Pass nil to remove the hook.
func (s *Surface) SetPrepareFrame(fn core.PrepareFrameFunc) {
	s.core.SetPrepareFrame(fn)
}

// SetPresentsWithTransaction toggles Core Animation transaction-based present
// on Metal surfaces. Enable during macOS live window resize so the drawable
// swap commits atomically with the resize CA transaction (Flutter/wgpu #3756
// pattern); disable for normal rendering. No-op on non-Metal backends.
func (s *Surface) SetPresentsWithTransaction(enabled bool) {
	if s.released || s.core == nil {
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
	if s.released {
		return ErrReleased
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
	if s.released || s.core == nil {
		return ErrReleased
	}
	raw := s.core.RawSurface()
	if raw == nil {
		return ErrReleased
	}
	if pw, ok := raw.(hal.PixelWriter); ok {
		return pw.WritePixels(data, width, height)
	}
	return fmt.Errorf("wgpu: WritePixels not supported on this backend")
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
	if s.released {
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
	if s.released {
		return
	}
	s.core.DiscardTexture()
}

func (s *Surface) discardForDevice(device *Device) {
	if s == nil || s.core == nil || s.device != device {
		return
	}
	s.core.DiscardTexture()
}

func (s *Surface) retireDevice(device *Device) {
	if s == nil || s.core == nil || s.device != device {
		return
	}
	s.core.RetireDevice(device.core)
	s.device = nil
}

func (s *Surface) createHALSurface(backend gputypes.Backend) (hal.Surface, error) {
	targetInstance := s.instance.core.HALInstanceForBackend(backend)
	if targetInstance == nil {
		return nil, fmt.Errorf("wgpu: no HAL instance for backend %v", backend)
	}
	halTarget, err := s.target.halTarget()
	if err != nil {
		return nil, err
	}
	return createHALSurfaceForTarget(targetInstance, backend, halTarget)
}

func createHALSurfaceForTarget(targetInstance hal.Instance, backend gputypes.Backend, target hal.SurfaceTarget) (hal.Surface, error) {
	halSurface, err := targetInstance.CreateSurface(target)
	if errors.Is(err, hal.ErrUnsupportedSurfaceTarget) {
		return nil, fmt.Errorf("%w: backend %v: %w", ErrUnsupportedSurfaceTarget, backend, err)
	}
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface for backend %v: %w", backend, err)
	}
	if halSurface == nil {
		return nil, fmt.Errorf("wgpu: backend %v returned a nil surface", backend)
	}
	return halSurface, nil
}

// ensureHALSurface creates or re-creates the HAL surface for the given backend.
func (s *Surface) ensureHALSurface(backend gputypes.Backend) error {
	if s.surfaceCreated && s.currentBackend == backend {
		return nil
	}
	halSurface := s.halSurfaces[backend]
	if halSurface == nil {
		var err error
		halSurface, err = s.createHALSurface(backend)
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

// HAL returns the underlying HAL surface for backward compatibility.
// Prefer using Surface methods instead of direct HAL access.
func (s *Surface) HAL() hal.Surface {
	if s == nil || s.released || s.core == nil {
		return nil
	}
	return s.core.RawSurface()
}

// halSurfaceForBackend returns the retained surface that belongs to an
// adapter's backend. Legacy surfaces wrapped from one HAL handle have no
// backend map, so they retain the historical active-surface fallback.
func (s *Surface) halSurfaceForBackend(backend gputypes.Backend) hal.Surface {
	if s == nil || s.released || s.core == nil {
		return nil
	}
	if len(s.halSurfaces) != 0 {
		return s.halSurfaces[backend]
	}
	return s.core.RawSurface()
}

func (s *Surface) halSurfacesForAdapterRequest() map[gputypes.Backend]hal.Surface {
	if s == nil || s.released || s.core == nil {
		return nil
	}
	result := make(map[gputypes.Backend]hal.Surface, len(s.halSurfaces))
	for backend, surface := range s.halSurfaces {
		result[backend] = surface
	}
	if len(result) == 0 {
		if raw := s.core.RawSurface(); raw != nil {
			result[s.currentBackend] = raw
		}
	}
	return result
}

// Release releases the surface.
func (s *Surface) Release() {
	if s.released {
		return
	}
	s.released = true
	if s.core != nil {
		destroyHALSurfaces(s.core, s.halSurfaces, s.currentBackend, s.surfaceCreated)
	}
	s.core = nil
	s.halSurfaces = nil
	if s.instance != nil {
		s.instance.unregisterSurface(s)
		s.instance = nil
	}
	s.targetSource = nil
}

func destroyHALSurfaces(coreSurface *core.Surface, surfaces map[gputypes.Backend]hal.Surface, currentBackend gputypes.Backend, currentSet bool) {
	if coreSurface != nil {
		coreSurface.Destroy()
	}
	for backend, surface := range surfaces {
		if currentSet && backend == currentBackend {
			continue
		}
		if surface != nil {
			surface.Destroy()
		}
	}
}

// SurfaceTexture is a texture acquired from a surface for rendering.
type SurfaceTexture struct {
	hal     hal.SurfaceTexture
	surface *Surface
	device  *Device
	lease   uint64
}

func (st *SurfaceTexture) isUsable() bool {
	if st == nil || st.hal == nil || st.surface == nil ||
		st.surface.released || st.surface.core == nil ||
		st.device == nil || st.device.released.Load() {
		return false
	}
	return st.surface.core.AcquisitionValid(st.lease)
}

// AsTexture returns a lightweight Texture wrapper around this surface texture,
// enabling use with Queue.WriteTexture() for direct CPU pixel upload without a
// render pass. The surface must be configured with TextureUsageCopyDst.
//
// The returned Texture shares the underlying HAL resource — do not Release() it
// independently. Its lifetime is tied to this SurfaceTexture.
func (st *SurfaceTexture) AsTexture() *Texture {
	if !st.isUsable() {
		return nil
	}
	return &Texture{
		hal:          st.hal,
		device:       st.device,
		surface:      st.surface.core,
		surfaceLease: st.lease,
	}
}

// CreateView creates a texture view of this surface texture.
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	if !st.isUsable() {
		return nil, ErrReleased
	}
	halDevice := st.device.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
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

	texture := &Texture{hal: st.hal, device: st.device, surface: st.surface.core, surfaceLease: st.lease}
	return &TextureView{hal: halView, device: st.device, texture: texture, surface: st.surface.core, surfaceLease: st.lease}, nil
}
