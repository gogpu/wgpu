//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
)

// Surface lifecycle errors.
var (
	// ErrSurfaceNotConfigured is returned when attempting to acquire or present
	// on a surface that has not been configured.
	ErrSurfaceNotConfigured = errors.New("core: surface is not configured")

	// ErrSurfaceAlreadyAcquired is returned when attempting to acquire a texture
	// while one is already acquired.
	ErrSurfaceAlreadyAcquired = errors.New("core: surface texture already acquired")

	// ErrSurfaceNoTextureAcquired is returned when attempting to present or discard
	// without an acquired texture.
	ErrSurfaceNoTextureAcquired = errors.New("core: no surface texture acquired")

	// ErrSurfaceConfigureWhileAcquired is returned when attempting to configure
	// a surface while a texture is still acquired.
	ErrSurfaceConfigureWhileAcquired = errors.New("core: cannot configure surface while texture is acquired")

	// ErrSurfaceNilDevice is returned when a nil device is passed to Configure.
	ErrSurfaceNilDevice = errors.New("core: device must not be nil")

	// ErrSurfaceNilConfig is returned when a nil config is passed to Configure.
	ErrSurfaceNilConfig = errors.New("core: surface configuration must not be nil")
)

// SetPrepareFrame registers a platform hook that is called before acquiring a texture.
//
// The hook returns the current surface dimensions and whether they changed.
// If changed is true, the surface is automatically reconfigured before acquiring.
//
// Pass nil to remove the hook.
func (s *Surface) SetPrepareFrame(fn PrepareFrameFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prepareFrame = fn
}

// Configure configures the surface with the given device and settings.
//
// The surface must not have an acquired texture. If the surface is already
// configured, it will be reconfigured with the new settings.
//
// After Configure, the surface enters the Configured state and is ready
// to acquire textures.
func (s *Surface) Configure(device *Device, config *hal.SurfaceConfiguration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if device == nil {
		return ErrSurfaceNilDevice
	}
	if config == nil {
		return ErrSurfaceNilConfig
	}
	if s.state == SurfaceStateAcquired {
		return ErrSurfaceConfigureWhileAcquired
	}

	halDevice := s.getHALDevice(device)
	if halDevice == nil {
		return ErrDeviceDestroyed
	}

	if err := s.raw.Configure(halDevice, config); err != nil {
		return err
	}

	s.invalidateAcquisitionLocked()
	s.device = device
	s.config = config
	s.state = SurfaceStateConfigured
	return nil
}

// Unconfigure removes the surface configuration and returns to the Unconfigured state.
//
// If a texture is currently acquired, it is discarded first.
// If the surface is already unconfigured, this is a no-op.
func (s *Surface) Unconfigure() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == SurfaceStateUnconfigured {
		return
	}

	// Discard acquired texture if any
	if s.state == SurfaceStateAcquired && s.acquiredTex != nil {
		s.raw.DiscardTexture(s.acquiredTex)
		s.acquiredTex = nil
	}
	s.invalidateAcquisitionLocked()

	halDevice := s.getHALDevice(s.device)
	if halDevice != nil {
		s.raw.Unconfigure(halDevice)
	}

	s.device = nil
	s.config = nil
	s.state = SurfaceStateUnconfigured
}

// AcquireTexture acquires the next surface texture for rendering.
//
// The surface must be in the Configured state. If a PrepareFrame hook is
// registered and reports that dimensions changed, the surface is automatically
// reconfigured before acquiring.
//
// After a successful acquire, the surface enters the Acquired state.
// The caller must either Present or DiscardTexture before acquiring again.
func (s *Surface) AcquireTexture(fence hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	result, _, err := s.AcquireTextureWithLease(fence)
	return result, err
}

// AcquireTextureWithLease acquires a texture and returns its opaque lifetime
// lease. Call AcquisitionValid before converting a retained public wrapper to
// HAL; the lease expires on present, discard, unconfigure, or destruction.
func (s *Surface) AcquireTextureWithLease(fence hal.Fence) (*hal.AcquiredSurfaceTexture, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == SurfaceStateAcquired {
		return nil, 0, ErrSurfaceAlreadyAcquired
	}
	if s.state != SurfaceStateConfigured {
		return nil, 0, ErrSurfaceNotConfigured
	}

	// Call PrepareFrame hook if registered
	if err := s.applyPrepareFrame(); err != nil {
		return nil, 0, err
	}

	result, err := s.raw.AcquireTexture(fence)
	if err != nil {
		return nil, 0, err
	}

	s.acquiredTex = result.Texture
	s.nextAcquisition++
	if s.nextAcquisition == 0 {
		s.nextAcquisition++
	}
	s.acquisition = s.nextAcquisition
	s.state = SurfaceStateAcquired
	return result, s.acquisition, nil
}

// AcquisitionValid reports whether lease still identifies this surface's
// current acquired texture.
func (s *Surface) AcquisitionValid(lease uint64) bool {
	if s == nil || lease == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == SurfaceStateAcquired && s.acquisition == lease
}

// Present presents the acquired surface texture to the screen.
//
// The surface must be in the Acquired state. After presenting, the surface
// returns to the Configured state and is ready to acquire again.
func (s *Surface) Present(queue hal.Queue) error {
	return s.PresentWithDamage(queue, nil)
}

// PresentWithDamage presents the acquired surface texture to the screen,
// passing optional damage rectangles to the compositor.
//
// damageRects specifies which regions of the surface changed this frame
// (physical pixels, top-left origin). When nil or empty, the entire surface
// is presented — identical code path to Present(). Backends that support
// damage rects use them as compositor hints; others accept and ignore them.
//
// The surface must be in the Acquired state. After presenting, the surface
// returns to the Configured state and is ready to acquire again.
func (s *Surface) PresentWithDamage(queue hal.Queue, damageRects []image.Rectangle) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SurfaceStateAcquired {
		return ErrSurfaceNoTextureAcquired
	}

	err := queue.Present(s.raw, s.acquiredTex, damageRects)
	s.acquiredTex = nil
	s.invalidateAcquisitionLocked()
	s.state = SurfaceStateConfigured
	return err
}

// PresentPixels writes RGBA pixel data directly to the surface and presents it
// in a single operation. This bypasses the WebGPU render pass pipeline entirely
// (no AcquireTexture, no render pass, no Present needed).
//
// On the software backend, this performs RGBA→BGRA swizzle into the DIB section
// framebuffer and immediately BitBlt's to the window — the fastest path for
// CPU-rendered content (1 copy vs 3 in the standard pipeline).
//
// The surface must be in Configured or Acquired state. If a texture is currently
// acquired, it is discarded before writing pixels (stale texture cleanup).
// The surface remains in Configured state after this call.
//
// Returns an error if the backend does not support PresentPixels (only the
// software backend implements this extension).
func (s *Surface) PresentPixels(data []byte, width, height uint32, damageRects []image.Rectangle) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == SurfaceStateUnconfigured {
		return ErrSurfaceNotConfigured
	}

	// Check backend support BEFORE discarding acquired texture.
	// GPU backends don't implement PixelPresenter — early return preserves
	// the acquired texture so the normal render→Present path still works.
	pp, ok := s.raw.(hal.PixelPresenter)
	if !ok {
		return fmt.Errorf("core: PresentPixels not supported on this backend")
	}

	// Discard stale acquired texture if any — PresentPixels replaces the
	// entire AcquireTexture→render→Present flow.
	if s.state == SurfaceStateAcquired && s.acquiredTex != nil {
		s.raw.DiscardTexture(s.acquiredTex)
		s.acquiredTex = nil
		s.invalidateAcquisitionLocked()
		s.state = SurfaceStateConfigured
	}

	return pp.PresentPixels(data, width, height, damageRects)
}

// DiscardTexture discards the acquired surface texture without presenting it.
//
// Use this if rendering failed or was canceled. If no texture is acquired,
// this is a no-op.
func (s *Surface) DiscardTexture() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SurfaceStateAcquired {
		return
	}

	if s.acquiredTex != nil {
		s.raw.DiscardTexture(s.acquiredTex)
	}

	s.acquiredTex = nil
	s.invalidateAcquisitionLocked()
	s.state = SurfaceStateConfigured
}

func (s *Surface) invalidateAcquisitionLocked() {
	s.acquisition = 0
}

// RetireDevice clears the logical configuration after its HAL device has been
// destroyed. The public API calls this after backend device teardown so a
// retained surface can be released or configured with another device without
// retaining a valid acquisition from the old device.
func (s *Surface) RetireDevice(device *Device) {
	if s == nil || device == nil {
		return
	}
	s.mu.Lock()
	if s.device != device {
		s.mu.Unlock()
		return
	}
	s.acquiredTex = nil
	s.invalidateAcquisitionLocked()
	s.device = nil
	s.config = nil
	s.state = SurfaceStateUnconfigured
	s.mu.Unlock()
}

// Destroy invalidates the active acquisition and releases the owned HAL
// surface. It deliberately does not call DiscardTexture: device-first instance
// teardown may already have retired the backend swapchain.
func (s *Surface) Destroy() {
	if s == nil {
		return
	}
	s.mu.Lock()
	raw := s.raw
	if raw == nil {
		s.mu.Unlock()
		return
	}
	acquiredTex := s.acquiredTex
	halDevice := s.getHALDevice(s.device)
	s.raw = nil
	s.acquiredTex = nil
	s.invalidateAcquisitionLocked()
	s.device = nil
	s.config = nil
	s.state = SurfaceStateUnconfigured
	s.mu.Unlock()

	// When the configured device is still alive, retire the current acquisition
	// and configuration before destroying the platform surface. Device-first
	// teardown leaves no HAL device here, so the backend has already retired (or
	// abandoned) its device-owned children and only the platform handle remains.
	if acquiredTex != nil {
		raw.DiscardTexture(acquiredTex)
	}
	if halDevice != nil {
		raw.Unconfigure(halDevice)
	}
	raw.Destroy()
	untrackResource(uintptr(unsafe.Pointer(s))) //nolint:gosec // debug tracking uses pointer as unique ID
}

// State returns the current lifecycle state of the surface.
func (s *Surface) State() SurfaceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Config returns the current surface configuration.
// Returns nil if the surface is unconfigured.
func (s *Surface) Config() *hal.SurfaceConfiguration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config
}

// applyPrepareFrame calls the PrepareFrame hook and reconfigures if dimensions changed.
// Must be called with s.mu held.
func (s *Surface) applyPrepareFrame() error {
	if s.prepareFrame == nil {
		return nil
	}

	width, height, changed := s.prepareFrame()
	if !changed || s.config == nil {
		return nil
	}

	newConfig := *s.config
	newConfig.Width = width
	newConfig.Height = height

	halDevice := s.getHALDevice(s.device)
	if halDevice == nil {
		return ErrDeviceDestroyed
	}

	if err := s.raw.Configure(halDevice, &newConfig); err != nil {
		return err
	}
	s.config = &newConfig
	return nil
}

// getHALDevice extracts the hal.Device from a core.Device using the snatch lock.
// Returns nil if the device has been destroyed or has no HAL integration.
// Must NOT be called with s.mu held if the device's snatch lock could deadlock;
// in practice the snatch lock is independent so this is safe.
func (s *Surface) getHALDevice(device *Device) hal.Device {
	if device == nil || device.SnatchLock() == nil {
		return nil
	}
	guard := device.SnatchLock().Read()
	defer guard.Release()
	return device.Raw(guard)
}
