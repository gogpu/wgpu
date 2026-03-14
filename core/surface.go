package core

import (
	"errors"

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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == SurfaceStateAcquired {
		return nil, ErrSurfaceAlreadyAcquired
	}
	if s.state != SurfaceStateConfigured {
		return nil, ErrSurfaceNotConfigured
	}

	// Call PrepareFrame hook if registered
	if err := s.applyPrepareFrame(); err != nil {
		return nil, err
	}

	result, err := s.raw.AcquireTexture(fence)
	if err != nil {
		return nil, err
	}

	s.acquiredTex = result.Texture
	s.state = SurfaceStateAcquired
	return result, nil
}

// Present presents the acquired surface texture to the screen.
//
// The surface must be in the Acquired state. After presenting, the surface
// returns to the Configured state and is ready to acquire again.
func (s *Surface) Present(queue hal.Queue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SurfaceStateAcquired {
		return ErrSurfaceNoTextureAcquired
	}

	err := queue.Present(s.raw, s.acquiredTex)
	s.acquiredTex = nil
	s.state = SurfaceStateConfigured
	return err
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
	s.state = SurfaceStateConfigured
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
