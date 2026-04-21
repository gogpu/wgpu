package software

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// nextResourceID is a monotonic counter for assigning unique IDs to software resources.
// This enables handle-to-resource resolution in bind groups.
var nextResourceID atomic.Uint64

// Resource is a placeholder implementation for most HAL resource gputypes.
// It implements the hal.Resource interface with a no-op Destroy method.
type Resource struct{}

// Destroy is a no-op.
func (r *Resource) Destroy() {}

// CurrentUsage returns 0 — Software backend has no resource state tracking.
func (r *Resource) CurrentUsage() gputypes.TextureUsage { return 0 }
func (r *Resource) AddPendingRef()                      {}
func (r *Resource) DecPendingRef()                      {}

// NativeHandle returns 0 for software resources (no real GPU handle).
func (r *Resource) NativeHandle() uintptr { return 0 }

// Buffer implements hal.Buffer with real data storage.
// All software buffers store their data in memory.
type Buffer struct {
	Resource
	id    uint64 // unique ID for handle resolution
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

// NativeHandle returns the buffer's unique ID for handle resolution.
func (b *Buffer) NativeHandle() uintptr { return uintptr(b.id) }

// Texture implements hal.Texture with real pixel storage.
type Texture struct {
	Resource
	id            uint64 // unique ID for handle resolution
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

// CurrentUsage returns 0 — Software backend has no resource state tracking.
func (t *Texture) CurrentUsage() gputypes.TextureUsage { return 0 }

// NativeHandle returns the texture's unique ID for handle resolution.
func (t *Texture) NativeHandle() uintptr { return uintptr(t.id) }

// Clear fills the texture with a color value in the texture's native format.
// BGRA textures store [B,G,R,A] per pixel; RGBA textures store [R,G,B,A].
func (t *Texture) Clear(color gputypes.Color) {
	t.mu.Lock()
	defer t.mu.Unlock()

	r := uint8(color.R * 255)
	g := uint8(color.G * 255)
	b := uint8(color.B * 255)
	a := uint8(color.A * 255)

	// Write bytes in format-appropriate order so the data is correct
	// for direct consumption by GDI (BGRA) or other readers.
	c0, c1, c2, c3 := r, g, b, a
	if t.format == gputypes.TextureFormatBGRA8Unorm || t.format == gputypes.TextureFormatBGRA8UnormSrgb {
		c0, c2 = b, r
	}

	for i := 0; i < len(t.data); i += 4 {
		t.data[i+0] = c0
		t.data[i+1] = c1
		t.data[i+2] = c2
		t.data[i+3] = c3
	}
}

// TextureView implements hal.TextureView.
// In software backend, views just reference the original texture.
type TextureView struct {
	Resource
	id      uint64 // unique ID for handle resolution
	texture *Texture
}

// NativeHandle returns the view's unique ID for handle resolution.
func (v *TextureView) NativeHandle() uintptr { return uintptr(v.id) }

// Surface implements hal.Surface for the software backend.
type Surface struct {
	Resource
	configured    bool
	width         uint32
	height        uint32
	format        gputypes.TextureFormat
	framebuffer   []byte
	mu            sync.RWMutex // Protects framebuffer access
	presentMode   hal.PresentMode
	alphaMode     hal.CompositeAlphaMode
	displayHandle uintptr // X11: Display*, macOS/Windows: 0
	hwnd          uintptr // window handle for platform blit (0 = headless)
	platformBlit          // platform-specific blit resources (Windows: DIB section, Linux: X11 GC)
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

	// Allocate framebuffer. On Windows with a window handle, use CreateDIBSection
	// (GDI-managed bitmap) for DWM-friendly presentation via BitBlt.
	// This follows the SDL3/Qt6 enterprise pattern and avoids the DWM freeze
	// that occurs with GetDC+StretchDIBits from a non-UI thread.
	// On headless or non-Windows, fall back to plain Go memory.
	s.framebuffer = s.allocateFramebuffer(config.Width, config.Height)

	slog.Debug("software: Surface.Configure",
		"width", config.Width, "height", config.Height)

	return nil
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.configured = false
	s.destroyPlatformFramebuffer()
	s.framebuffer = nil
}

// allocateFramebuffer creates a framebuffer for the given dimensions.
// Prefers platform DIB section (Windows) for DWM-safe presentation.
// Falls back to Go heap memory with buffer reuse to avoid GC pressure.
func (s *Surface) allocateFramebuffer(width, height uint32) []byte {
	if fb := s.createPlatformFramebuffer(int32(width), int32(height)); fb != nil {
		return fb
	}
	// Fallback: Go heap memory (headless, non-Windows, or DIB creation failure).
	// Reuse if capacity sufficient to avoid GC pressure during drag resize.
	size := int(width) * int(height) * 4
	oldSize := len(s.framebuffer)
	if cap(s.framebuffer) >= size {
		fb := s.framebuffer[:size]
		if size > oldSize {
			clear(fb[oldSize:])
		}
		return fb
	}
	return make([]byte, size)
}

// AcquireTexture returns a surface texture backed by the framebuffer.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.framebuffer) > 0 {
		slog.Debug("software: AcquireTexture",
			"width", s.width, "height", s.height)
	}

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

// GetFramebuffer returns a copy of the current framebuffer data in RGBA byte
// order (thread-safe). If the surface format is BGRA, R and B channels are
// swapped so callers always receive consistent RGBA data. This allows
// platform blit code to do a single RGBA→BGRA conversion for GDI/X11.
func (s *Surface) GetFramebuffer() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.framebuffer == nil {
		return nil
	}

	result := make([]byte, len(s.framebuffer))
	copy(result, s.framebuffer)

	// If surface format is BGRA, the framebuffer stores BGRA bytes.
	// Swap R↔B so the returned data is always RGBA.
	if s.format == gputypes.TextureFormatBGRA8Unorm ||
		s.format == gputypes.TextureFormatBGRA8UnormSrgb {
		for i := 0; i < len(result)-3; i += 4 {
			result[i+0], result[i+2] = result[i+2], result[i+0]
		}
	}

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

// RenderPipeline stores render pipeline configuration for the software backend.
type RenderPipeline struct {
	Resource
	desc *hal.RenderPipelineDescriptor
}

// BindGroup stores bound resources for the software backend.
// It resolves handle-based entries to concrete software resource pointers.
type BindGroup struct {
	Resource
	desc         *hal.BindGroupDescriptor
	textureViews map[uint32]*TextureView // binding index -> resolved texture view
	buffers      map[uint32]*Buffer      // binding index -> resolved buffer
}

// ShaderModule stores shader source for the software backend.
type ShaderModule struct {
	Resource
	desc *hal.ShaderModuleDescriptor
}
