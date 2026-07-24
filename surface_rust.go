//go:build rust

package wgpu

import (
	"fmt"
	"image"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// Surface represents a platform rendering surface.
// On Rust backend, this wraps go-webgpu/webgpu Surface.
type Surface struct {
	r        *rwgpu.Surface
	device   *Device
	released bool

	// targetSource is retained only for CreateSurfaceFromTarget.
	targetSource SurfaceTarget

	// Cached configuration for texture creation.
	configFormat TextureFormat
	configWidth  uint32
	configHeight uint32
}

// CreateSurface creates a rendering surface from legacy platform-specific
// handles. New code should prefer CreateSurfaceFromTarget or
// CreateSurfaceUnsafe so the target kind and ownership contract are explicit.
// On Rust backend, it dispatches to the platform-appropriate creation method.
// displayHandle and windowHandle are platform-specific:
//   - Windows: displayHandle=HINSTANCE (can be 0), windowHandle=HWND
//   - macOS: displayHandle=0, windowHandle=CAMetalLayer*
//   - Linux/X11: displayHandle=Display*, windowHandle=Window
//   - Linux/Wayland: displayHandle=wl_display*, windowHandle=wl_surface*
//   - Android: displayHandle ignored, windowHandle=ANativeWindow*
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (*Surface, error) {
	return i.createSurface(surfaceTargetFromLegacyHandles(displayHandle, windowHandle), nil)
}

// CreateSurfaceFromTarget samples a provider once and retains it until the
// surface is released.
func (i *Instance) CreateSurfaceFromTarget(target SurfaceTarget) (*Surface, error) {
	if i == nil || i.released {
		return nil, ErrReleased
	}
	rawTarget, err := resolveSurfaceTarget(target)
	if err != nil {
		return nil, err
	}
	return i.createSurface(rawTarget, target)
}

// CreateSurfaceUnsafe creates a surface from raw platform handles without
// retaining an ownership source.
func (i *Instance) CreateSurfaceUnsafe(target SurfaceTargetUnsafe) (*Surface, error) {
	if i == nil || i.released {
		return nil, ErrReleased
	}
	if err := target.validate(); err != nil {
		return nil, err
	}
	return i.createSurface(target, nil)
}

func (i *Instance) createSurface(target SurfaceTargetUnsafe, targetSource SurfaceTarget) (*Surface, error) {
	if i.released {
		return nil, ErrReleased
	}

	rs, err := createPlatformSurfaceTarget(i.r, target)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface: %w", err)
	}

	return &Surface{r: rs, targetSource: targetSource}, nil
}

// Configure configures the surface for presentation.
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

	rConfig := &rwgpu.SurfaceConfiguration{
		Format:      config.Format,
		Usage:       config.Usage,
		Width:       config.Width,
		Height:      config.Height,
		AlphaMode:   config.AlphaMode,
		PresentMode: config.PresentMode,
	}

	if err := s.r.Configure(device.r, rConfig); err != nil {
		return fmt.Errorf("wgpu: failed to configure surface: %w", err)
	}

	s.device = device
	s.configFormat = config.Format
	s.configWidth = config.Width
	s.configHeight = config.Height
	return nil
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	if s.released {
		return
	}
	s.r.Unconfigure()
	s.device = nil
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

	rst, suboptimal, err := s.r.GetCurrentTexture()
	if err != nil {
		return nil, false, fmt.Errorf("wgpu: %w", err)
	}

	return &SurfaceTexture{
		r: rst,
		texture: &Texture{
			r:      rst.Texture,
			device: s.device,
			format: s.configFormat,
		},
		surface: s,
	}, suboptimal, nil
}

// Present presents a surface texture to the screen.
func (s *Surface) Present(texture *SurfaceTexture) error {
	if s.released {
		return ErrReleased
	}
	if texture == nil {
		return fmt.Errorf("wgpu: surface texture is nil")
	}
	// go-webgpu Present takes variadic *SurfaceTexture.
	return s.r.Present(texture.r)
}

// PresentWithDamage presents a surface texture, optionally with damage rects.
// On Rust backend, damage rects are ignored. wgpu-native does not support them.
func (s *Surface) PresentWithDamage(st *SurfaceTexture, _ []image.Rectangle) error {
	return s.Present(st)
}

// ReadPixels is not supported by the Rust FFI backend.
// Headless surface readback is a Pure-Go software-backend extension.
func (s *Surface) ReadPixels() ([]byte, error) {
	if s == nil || s.released {
		return nil, ErrReleased
	}
	if s.device == nil {
		return nil, fmt.Errorf("wgpu: surface not configured")
	}
	return nil, fmt.Errorf("wgpu: ReadPixels not supported on this backend")
}

// ActualExtent returns the configured surface dimensions.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s.released {
		return 0, 0
	}
	return s.configWidth, s.configHeight
}

// SetPrepareFrame registers a platform hook called before each GetCurrentTexture.
// On Rust backend, this is a no-op — wgpu-native handles HiDPI internally.
// The function signature uses any to avoid importing core in the rust build path.
func (s *Surface) SetPrepareFrame(_ any) {}

// DiscardTexture discards the acquired surface texture without presenting it.
// On Rust backend, this is a no-op.
func (s *Surface) DiscardTexture() {
	// No-op: wgpu-native does not support texture discard.
}

// Release releases the surface.
func (s *Surface) Release() {
	if s.released {
		return
	}
	s.released = true
	if s.r != nil {
		s.r.Release()
	}
	s.targetSource = nil
}

// SurfaceTexture is a texture acquired from a surface for rendering.
type SurfaceTexture struct {
	r       *rwgpu.SurfaceTexture
	texture *Texture
	surface *Surface
}

// AsTexture returns the underlying Texture for direct WriteTexture access.
func (st *SurfaceTexture) AsTexture() *Texture { return st.texture }

// CreateView creates a texture view of this surface texture.
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	if st.texture == nil || st.texture.r == nil {
		return nil, ErrReleased
	}

	var rDesc *rwgpu.TextureViewDescriptor
	if desc != nil {
		rDesc = &rwgpu.TextureViewDescriptor{
			Label:           desc.Label,
			Format:          desc.Format,
			Dimension:       desc.Dimension,
			Aspect:          rwgpu.TextureAspect(desc.Aspect),
			BaseMipLevel:    desc.BaseMipLevel,
			MipLevelCount:   desc.MipLevelCount,
			BaseArrayLayer:  desc.BaseArrayLayer,
			ArrayLayerCount: desc.ArrayLayerCount,
		}
	}

	rv, err := st.texture.r.CreateView(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface texture view: %w", err)
	}

	return &TextureView{r: rv, device: st.surface.device, texture: st.AsTexture()}, nil
}

// Texture returns the underlying Texture.
func (st *SurfaceTexture) Texture() *Texture {
	return st.texture
}
