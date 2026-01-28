//go:build software

package software

import (
	"sync"
	"sync/atomic"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
)

// Resource is a placeholder implementation for most HAL resource gputypes.
// It implements the hal.Resource interface with a no-op Destroy method.
type Resource struct{}

// Destroy is a no-op.
func (r *Resource) Destroy() {}

// Buffer implements hal.Buffer with real data storage.
// All software buffers store their data in memory.
type Buffer struct {
	Resource
	data  []byte
	size  uint64
	usage gputypes.BufferUsage
	mu    sync.RWMutex // Protects data access
}

// GetData returns a copy of the buffer data (thread-safe).
func (b *Buffer) GetData() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]byte, len(b.data))
	copy(result, b.data)
	return result
}

// WriteData writes data to the buffer (thread-safe).
func (b *Buffer) WriteData(offset uint64, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	copy(b.data[offset:], data)
}

// Texture implements hal.Texture with real pixel storage.
type Texture struct {
	Resource
	data          []byte
	width         uint32
	height        uint32
	depth         uint32
	format        gputypes.TextureFormat
	usage         gputypes.TextureUsage
	mipLevelCount uint32
	sampleCount   uint32
	mu            sync.RWMutex // Protects data access
}

// GetData returns a copy of the texture data (thread-safe).
func (t *Texture) GetData() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]byte, len(t.data))
	copy(result, t.data)
	return result
}

// WriteData writes data to the texture (thread-safe).
func (t *Texture) WriteData(offset uint64, data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	copy(t.data[offset:], data)
}

// Clear fills the texture with a color value.
func (t *Texture) Clear(color gputypes.Color) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Simple RGBA8 clear (4 bytes per pixel)
	r := uint8(color.R * 255)
	g := uint8(color.G * 255)
	b := uint8(color.B * 255)
	a := uint8(color.A * 255)

	for i := 0; i < len(t.data); i += 4 {
		t.data[i+0] = r
		t.data[i+1] = g
		t.data[i+2] = b
		t.data[i+3] = a
	}
}

// TextureView implements hal.TextureView.
// In software backend, views just reference the original texture.
type TextureView struct {
	Resource
	texture *Texture
}

// Surface implements hal.Surface for the software backend.
type Surface struct {
	Resource
	configured  bool
	width       uint32
	height      uint32
	format      gputypes.TextureFormat
	framebuffer []byte
	mu          sync.RWMutex // Protects framebuffer access
	presentMode hal.PresentMode
	alphaMode   hal.CompositeAlphaMode
}

// Configure configures the surface with the given settings.
//
// Returns hal.ErrZeroArea if width or height is zero.
// This commonly happens when the window is minimized or not yet fully visible.
// Wait until the window has valid dimensions before calling Configure again.
func (s *Surface) Configure(_ hal.Device, config *hal.SurfaceConfiguration) error {
	// Validate dimensions first (before any side effects).
	// This matches wgpu-core behavior which returns ConfigureSurfaceError::ZeroArea.
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.configured = true
	s.width = config.Width
	s.height = config.Height
	s.format = config.Format
	s.presentMode = config.PresentMode
	s.alphaMode = config.AlphaMode

	// Allocate framebuffer (assuming 4 bytes per pixel - RGBA8)
	size := int(config.Width) * int(config.Height) * 4
	s.framebuffer = make([]byte, size)

	return nil
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.configured = false
	s.framebuffer = nil
}

// AcquireTexture returns a surface texture backed by the framebuffer.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &hal.AcquiredSurfaceTexture{
		Texture: &SurfaceTexture{
			surface: s,
			Texture: Texture{
				data:   s.framebuffer,
				width:  s.width,
				height: s.height,
				depth:  1,
				format: s.format,
				usage:  gputypes.TextureUsageRenderAttachment,
			},
		},
		Suboptimal: false,
	}, nil
}

// DiscardTexture is a no-op (framebuffer stays allocated).
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {}

// GetFramebuffer returns a copy of the current framebuffer data (thread-safe).
// This is the key method for reading rendered results in software backend.
func (s *Surface) GetFramebuffer() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.framebuffer == nil {
		return nil
	}

	result := make([]byte, len(s.framebuffer))
	copy(result, s.framebuffer)
	return result
}

// SurfaceTexture implements hal.SurfaceTexture.
// It shares the framebuffer with the surface.
type SurfaceTexture struct {
	Texture
	surface *Surface
}

// Fence implements hal.Fence with an atomic counter for synchronization.
type Fence struct {
	Resource
	value atomic.Uint64
}
