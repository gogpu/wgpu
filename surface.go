package wgpu

import (
	"fmt"

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

	coreSurface := core.NewSurface(halSurface, "")
	return &Surface{
		core:     coreSurface,
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
	if device == nil {
		return fmt.Errorf("wgpu: device is nil")
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

	acquired, err := s.core.AcquireTexture(nil)
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

	if texture == nil {
		return fmt.Errorf("wgpu: surface texture is nil")
	}

	return s.core.Present(s.device.queue.hal)
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

// DiscardTexture discards the acquired surface texture without presenting it.
// Use this if rendering failed or was canceled. If no texture is currently
// acquired, this is a no-op.
func (s *Surface) DiscardTexture() {
	if s.released {
		return
	}
	s.core.DiscardTexture()
}

// HAL returns the underlying HAL surface for backward compatibility.
// Prefer using Surface methods instead of direct HAL access.
func (s *Surface) HAL() hal.Surface {
	return s.core.RawSurface()
}

// Release releases the surface.
func (s *Surface) Release() {
	if s.released {
		return
	}
	s.released = true
	s.core.RawSurface().Destroy()
	s.core = nil
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

	return &TextureView{hal: halView, device: st.device}, nil
}
