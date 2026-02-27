package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
)

// Surface represents a platform rendering surface (e.g., a window).
type Surface struct {
	hal      hal.Surface
	instance *Instance
	device   *Device
	released bool
}

// CreateSurface creates a rendering surface from platform-specific handles.
// displayHandle and windowHandle are platform-specific:
//   - Windows: displayHandle=0, windowHandle=HWND
//   - macOS: displayHandle=0, windowHandle=NSView*
//   - Linux/X11: displayHandle=Display*, windowHandle=Window
//   - Linux/Wayland: displayHandle=wl_display*, windowHandle=wl_surface*
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (*Surface, error) {
	if i.released {
		return nil, ErrReleased
	}

	halInstance := i.core.HALInstance()
	if halInstance == nil {
		return nil, fmt.Errorf("wgpu: no HAL instance available for surface creation")
	}

	halSurface, err := halInstance.CreateSurface(displayHandle, windowHandle)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface: %w", err)
	}

	return &Surface{
		hal:      halSurface,
		instance: i,
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

	halDevice := device.halDevice()
	if halDevice == nil {
		return ErrReleased
	}

	halConfig := &hal.SurfaceConfiguration{
		Width:       config.Width,
		Height:      config.Height,
		Format:      config.Format,
		Usage:       config.Usage,
		PresentMode: config.PresentMode,
		AlphaMode:   config.AlphaMode,
	}

	s.device = device
	return s.hal.Configure(halDevice, halConfig)
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	if s.released || s.device == nil {
		return
	}
	halDevice := s.device.halDevice()
	if halDevice == nil {
		return
	}
	s.hal.Unconfigure(halDevice)
}

// GetCurrentTexture acquires the next texture for rendering.
// Returns the surface texture and whether the surface is suboptimal.
func (s *Surface) GetCurrentTexture() (*SurfaceTexture, bool, error) {
	if s.released {
		return nil, false, ErrReleased
	}
	if s.device == nil {
		return nil, false, fmt.Errorf("wgpu: surface not configured")
	}

	halDevice := s.device.halDevice()
	if halDevice == nil {
		return nil, false, ErrReleased
	}

	fence, err := halDevice.CreateFence()
	if err != nil {
		return nil, false, fmt.Errorf("wgpu: failed to create acquire fence: %w", err)
	}
	defer halDevice.DestroyFence(fence)

	acquired, err := s.hal.AcquireTexture(fence)
	if err != nil {
		return nil, false, err
	}

	return &SurfaceTexture{
		hal:     acquired.Texture,
		surface: s,
		device:  s.device,
	}, acquired.Suboptimal, nil
}

// Present presents a surface texture to the screen.
func (s *Surface) Present(texture *SurfaceTexture) error {
	if s.released {
		return ErrReleased
	}
	if s.device == nil {
		return fmt.Errorf("wgpu: surface not configured")
	}
	if s.device.queue == nil || s.device.queue.hal == nil {
		return fmt.Errorf("wgpu: queue not available")
	}

	return s.device.queue.hal.Present(s.hal, texture.hal)
}

// Release releases the surface.
func (s *Surface) Release() {
	if s.released {
		return
	}
	s.released = true
	s.hal.Destroy()
}

// SurfaceTexture is a texture acquired from a surface for rendering.
type SurfaceTexture struct {
	hal     hal.SurfaceTexture
	surface *Surface
	device  *Device
}

// CreateView creates a texture view of this surface texture.
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	halDevice := st.device.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.TextureViewDescriptor{}
	if desc != nil {
		halDesc.Label = desc.Label
		halDesc.Format = desc.Format
		halDesc.Dimension = desc.Dimension
		halDesc.Aspect = desc.Aspect
		halDesc.BaseMipLevel = desc.BaseMipLevel
		halDesc.MipLevelCount = desc.MipLevelCount
		halDesc.BaseArrayLayer = desc.BaseArrayLayer
		halDesc.ArrayLayerCount = desc.ArrayLayerCount
	}

	halView, err := halDevice.CreateTextureView(st.hal, halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface texture view: %w", err)
	}

	return &TextureView{hal: halView, device: st.device}, nil
}
