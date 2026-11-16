//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/surface"
)

// Compile-time assertion: Surface must implement all public API methods (ADR-047).
var _ interface {
	Configure(*Device, *SurfaceConfiguration) error
	Unconfigure()
	GetCurrentTexture() (*SurfaceTexture, bool, error)
	Present(*SurfaceTexture) error
	PresentWithDamage(*SurfaceTexture, []image.Rectangle) error
	PresentPixels([]byte, uint32, uint32, []image.Rectangle) error
	WritePixels([]byte, uint32, uint32) error
	ReadPixels() ([]byte, error)
	ActualExtent() (uint32, uint32)
	DiscardTexture()
	SetPresentsWithTransaction(bool)
	Release()
} = (*Surface)(nil)

const (
	platformWindows = "windows"
	platformDarwin  = "darwin"
	platformLinux   = "linux"
	platformAndroid = "android"
)

// Surface represents a platform rendering surface (e.g., a window).
//
// The lifecycle implementation lives in internal/surface (ADR-070 S10); this
// type is the declaration layer the public API is frozen on. It holds the
// domain's implementation handle, the instance the surface is registered with,
// and the target ownership source, all of which are root concerns: the instance
// map stays in this package until S12, and the ownership provider is a public
// value the destination cannot name.
type Surface struct {
	impl *surface.Surface

	// instance is the root instance the surface is registered with for device
	// teardown. Kept here, not in the destination: the cross-instance rejection
	// and the unregister are root-side facts, and the instance map is root-owned
	// until S12.
	instance *Instance

	// targetSource is retained only for CreateSurfaceFromTarget. Raw handles
	// passed through CreateSurfaceUnsafe or the compatibility CreateSurface
	// method remain entirely caller-owned.
	targetSource SurfaceTarget
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
	if i.isReleased() {
		return nil, ErrReleased
	}
	return i.createSurface(surfaceTargetFromLegacyHandles(i.display, displayHandle, windowHandle), nil)
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
	if err := checkSurfaceDisplay(i.display, target); err != nil {
		return nil, err
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
	return i.adoptCreatedSurface(targetSource, halSurfaces, initialBackend, halTarget)
}

func (i *Instance) adoptCreatedSurface(
	targetSource SurfaceTarget,
	halSurfaces map[gputypes.Backend]hal.Surface,
	initialBackend gputypes.Backend,
	halTarget hal.SurfaceTarget,
) (*Surface, error) {
	halSurface := halSurfaces[initialBackend]
	coreSurface := core.NewSurface(halSurface, "")
	impl := surface.New(surface.Init{
		Core:           coreSurface,
		Target:         halTarget,
		TargetSet:      true,
		Backend:        initialBackend,
		HALSurfaces:    halSurfaces,
		SurfaceCreated: true,
	})
	created := &Surface{impl: impl, targetSource: targetSource}
	if err := i.adoptSurface(created); err != nil {
		surface.DestroyHALSurfaces(coreSurface, halSurfaces, initialBackend, true)
		return nil, err
	}
	return created, nil
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

func surfaceTargetFromLegacyHandles(display DisplayHandle, displayHandle, windowHandle uintptr) SurfaceTargetUnsafe {
	return surfaceTargetFromLegacyHandlesForPlatform(
		runtime.GOOS,
		os.Getenv("WAYLAND_DISPLAY"),
		display,
		displayHandle,
		windowHandle,
	)
}

// surfaceTargetFromLegacyHandlesForPlatform maps legacy handles to a typed
// target. On Linux an instance Display decides between Xlib and Wayland;
// without one, the WAYLAND_DISPLAY environment does.
func surfaceTargetFromLegacyHandlesForPlatform(goos, waylandDisplay string, display DisplayHandle, displayHandle, windowHandle uintptr) SurfaceTargetUnsafe {
	switch goos {
	case platformWindows:
		return SurfaceTargetFromWindowsHWND(displayHandle, windowHandle)
	case platformDarwin:
		return SurfaceTargetFromMetalLayer(windowHandle)
	case platformLinux:
		if target, ok := display.legacySurfaceTarget(displayHandle, windowHandle); ok {
			return target
		}
		if waylandDisplay != "" {
			return SurfaceTargetFromWaylandSurface(displayHandle, windowHandle)
		}
		return SurfaceTargetFromXlibWindow(displayHandle, windowHandle)
	case platformAndroid:
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
	case surfaceTargetHeadless:
		kind = hal.SurfaceTargetHeadless
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
	host := newSurfaceDeviceHost(device)

	// The cross-instance rejection is a root fact: the instance the surface is
	// registered with is this package's type. Standard API objects must share an
	// Instance so device teardown can find and retire every configured surface;
	// explicit HAL wrappers remain usable when both objects are intentionally
	// unowned (instance == nil).
	sameInstance := true
	if device != nil {
		sameInstance = s.instance == device.instance || (s.instance == nil && device.instance == nil)
	}

	// A zero-value public Surface keeps its pre-move answers: an owned device
	// was rejected as a different instance before any surface state was read,
	// while an unowned one proceeded to the released answer.
	if s.impl == nil && !sameInstance {
		return fmt.Errorf("wgpu: surface and device belong to different instances")
	}

	// The HAL instance a backend's raw surface is created on is a root fact too:
	// the instance's HAL entries stay in this package until S12. Resolve it for
	// the device's backend and hand it in as a value; the implementation never
	// names the instance.
	var halInstance hal.Instance
	if device != nil && device.core != nil && s.instance != nil && s.instance.core != nil {
		halInstance = s.instance.core.HALInstanceForBackend(device.core.Backend())
	}

	return mapSurfaceConfigureError(s.impl.Configure(host, surfaceConfiguration(config), sameInstance, halInstance))
}

// surfaceConfiguration converts the public surface configuration into the
// surface domain's mirror, or nil for an absent configuration: the destination
// reports the same nil-configuration error the public method did.
func surfaceConfiguration(config *SurfaceConfiguration) *surface.SurfaceConfiguration {
	if config == nil {
		return nil
	}
	return &surface.SurfaceConfiguration{
		Width:       config.Width,
		Height:      config.Height,
		Format:      config.Format,
		Usage:       config.Usage,
		PresentMode: config.PresentMode,
		AlphaMode:   config.AlphaMode,
	}
}

// mapSurfaceConfigureError gives the backend's device-mismatch error its
// public sentinel, keeping the backend detail: errors.Is matches both. It also
// restores the public unsupported-target sentinel around the implementation's
// re-creation failure, which is the same error the creation path reports.
func mapSurfaceConfigureError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, hal.ErrSurfaceDeviceMismatch) {
		return fmt.Errorf("%w: %w", ErrSurfaceDeviceMismatch, err)
	}
	if errors.Is(err, hal.ErrUnsupportedSurfaceTarget) && !errors.Is(err, ErrUnsupportedSurfaceTarget) {
		return fmt.Errorf("%w: %w", ErrUnsupportedSurfaceTarget, err)
	}
	return err
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	s.impl.Unconfigure()
}

// GetCurrentTexture acquires the next texture for rendering.
// Returns the surface texture and whether the surface is suboptimal.
func (s *Surface) GetCurrentTexture() (*SurfaceTexture, bool, error) {
	impl, suboptimal, err := s.impl.GetCurrentTexture()
	if err != nil {
		return nil, false, err
	}
	return &SurfaceTexture{impl: impl}, suboptimal, nil
}

// Present presents a surface texture to the screen.
func (s *Surface) Present(texture *SurfaceTexture) error {
	// The destination's Present is PresentWithDamage with no damage rects; going
	// to it directly keeps the hot path one call shorter than the pre-move
	// two-step (the same answer either way).
	return s.impl.Present(surfaceTextureHandle(texture))
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
	return s.impl.PresentWithDamage(surfaceTextureHandle(texture), damageRects)
}

// surfaceTextureHandle returns the implementation handle of st. A nil wrapper
// stays nil, so the destination reports "surface texture is nil"; a zero-value
// wrapper embeds the zero implementation, which fails the same
// nil-acquisition check the pre-move wrapper did.
func surfaceTextureHandle(st *SurfaceTexture) *surface.SurfaceTexture {
	if st == nil {
		return nil
	}
	return &st.impl
}

// SetPresentsWithTransaction toggles Core Animation transaction-based present
// on Metal surfaces. Enable during macOS live window resize so the drawable
// swap commits atomically with the resize CA transaction (Flutter/wgpu #3756
// pattern); disable for normal rendering. No-op on non-Metal backends.
func (s *Surface) SetPresentsWithTransaction(enabled bool) {
	s.impl.SetPresentsWithTransaction(enabled)
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
	return s.impl.PresentPixels(data, width, height, damageRects)
}

// WritePixels copies RGBA pixel data directly into the surface framebuffer,
// bypassing render pass and texture upload. On software backend this is a
// single-pass RGBA→BGRA swizzle+copy into the DIB section (1 copy vs 3).
// On GPU backends, falls back to WriteTexture + present (no fast path).
//
// Call between GetCurrentTexture() and Present(). No render pass needed.
// Returns ErrReleased if the surface is not configured.
func (s *Surface) WritePixels(data []byte, width, height uint32) error {
	if s == nil {
		return ErrReleased
	}
	return s.impl.WritePixels(data, width, height)
}

// ReadPixels captures the current surface framebuffer as an owned, tightly
// packed RGBA8 snapshot in top-left, row-major order.
//
// This is a non-WebGPU extension implemented by the Pure-Go software backend.
// The surface must be configured, and any acquired texture must be presented
// or discarded before capture.
func (s *Surface) ReadPixels() ([]byte, error) {
	if s == nil {
		return nil, ErrReleased
	}
	return s.impl.ReadPixels()
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
	return s.impl.ActualExtent()
}

// DiscardTexture discards the acquired surface texture without presenting it.
// Use this if rendering failed or was canceled. If no texture is currently
// acquired, this is a no-op.
func (s *Surface) DiscardTexture() {
	s.impl.DiscardTexture()
}

// sameDevice reports whether the surface is configured against device. The
// destination compares the core device identity, so an adapter value built per
// call never compares equal to the stored one. A nil device, or a nil core on
// either side, is no match.
func (s *Surface) sameDevice(device *Device) bool {
	if s == nil || s.impl == nil || device == nil {
		return false
	}
	return s.impl.SameDevice(device.core)
}

func (s *Surface) discardForDevice(device *Device) {
	if s == nil || s.impl == nil || device == nil {
		return
	}
	s.impl.DiscardForDevice(device.core)
}

func (s *Surface) retireDevice(device *Device) {
	if s == nil || s.impl == nil || device == nil {
		return
	}
	s.impl.RetireDevice(device.core)
}

// Release releases the surface: the implementation destroys the HAL surfaces,
// and this layer then unregisters the surface from its instance and drops the
// target ownership source, in the pre-move order.
func (s *Surface) Release() {
	s.impl.Release()
	if s.instance != nil {
		s.instance.unregisterSurface(s)
		s.instance = nil
	}
	s.targetSource = nil
}

// SurfaceTexture is a texture acquired from a surface for rendering.
//
// The implementation is embedded by value: an acquisition allocates the public
// wrapper once, exactly as before the move (ADR-070 §15.7).
type SurfaceTexture struct {
	impl surface.SurfaceTexture
}

// AsTexture returns a lightweight Texture wrapper around this surface texture,
// enabling use with Queue.WriteTexture() for direct CPU pixel upload without a
// render pass. The surface must be configured with TextureUsageCopyDst.
//
// The returned Texture shares the underlying HAL resource — do not Release() it
// independently. Its lifetime is tied to this SurfaceTexture.
func (st *SurfaceTexture) AsTexture() *Texture {
	if st == nil {
		return nil
	}
	res := st.impl.Resource()
	if res == nil {
		return nil
	}
	tex := &Texture{impl: res}
	res.Pub = tex
	return tex
}

// CreateView creates a texture view of this surface texture.
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	if st == nil {
		return nil, ErrReleased
	}
	rv, err := st.impl.CreateView(surfaceTextureViewDescriptor(desc))
	if err != nil {
		return nil, err
	}
	// Cache the public wrapper of the view's borrowed parent texture: the
	// pre-move view creation built it, and TextureView.Texture reads it.
	if parent := rv.Texture(); parent != nil && parent.Pub == nil {
		parent.Pub = &Texture{impl: parent}
	}
	return &TextureView{impl: rv}, nil
}

// surfaceTextureViewDescriptor converts the public view descriptor into the
// surface domain's mirror, or nil for an absent descriptor: the destination
// derives the defaults exactly as the public method did.
func surfaceTextureViewDescriptor(desc *TextureViewDescriptor) *surface.TextureViewDescriptor {
	if desc == nil {
		return nil
	}
	return &surface.TextureViewDescriptor{
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
