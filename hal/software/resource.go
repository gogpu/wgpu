//go:build !(js && wasm)

package software

import (
	"fmt"
	"image"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/shader"
	"github.com/gogpu/wgpu/internal/shaderir"
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

// formatBytesPerPixel returns the bytes per stored texel (per texel block of
// a compressed format) for texture allocation and sampling: the format's
// BlockCopySize where WebGPU defines one. Depth24Plus stores a float32
// depth; the combined depth-stencil formats store a float32 depth and the
// stencil byte in an 8-byte texel (shader.DepthStencilTexelSize).
func formatBytesPerPixel(f gputypes.TextureFormat) uint64 {
	switch f {
	case gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32FloatStencil8:
		return shader.DepthStencilTexelSize
	}
	if s := f.BlockCopySize(); s > 0 {
		return uint64(s)
	}
	return 4
}

// Texture implements hal.Texture with real pixel storage.
type Texture struct {
	Resource
	id     uint64 // unique ID for handle resolution
	data   []byte
	width  uint32
	height uint32
	depth  uint32 // array layers, or depth slices of a 3D texture
	// dimension tells array layers from depth slices: the depth of a 3D
	// texture shrinks per mip level. Undefined (a texture built without
	// CreateTexture) behaves as 2D. Storage layout: subresource.go.
	dimension     gputypes.TextureDimension
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

// Clear fills the texture with a color value encoded in the texture's
// format, as a render pass clear of a view in that format stores it: BGRA
// textures store [B,G,R,A] per pixel, sRGB ones the encoded red, green and
// blue, float and integer ones their own channel types. A depth/stencil
// texture is left as it is.
func (t *Texture) Clear(color gputypes.Color) {
	t.clearAs(t.format, color)
}

// TextureView implements hal.TextureView: a range of a texture's mip levels
// and array layers seen with one view dimension. CreateTextureView resolves
// the descriptor's defaults, so the counts are explicit; a zero count (a view
// built without it) means 1.
//
// The view has its own format, which may differ from the texture's only by
// the sRGB suffix (core validates the texture's ViewFormats): the same bytes
// are then read and written through the view's format, so an RGBA8Unorm
// texture seen through an RGBA8UnormSrgb view decodes sRGB on every fetch and
// encodes it on every colour write. The aspect selects the depth or stencil
// plane of a combined depth-stencil format.
type TextureView struct {
	Resource
	id             uint64 // unique ID for handle resolution
	texture        *Texture
	baseMipLevel   uint32
	mipLevelCount  uint32
	baseArrayLayer uint32
	// arrayLayerCount is the number of array layers in the view: 1 for a 3D
	// view, whose depth slices are not layers.
	arrayLayerCount uint32
	dimension       gputypes.TextureViewDimension
	// format is the view's format; Undefined (a view built without
	// CreateTextureView) means the texture's.
	format gputypes.TextureFormat
	// aspect is the view's aspect; Undefined means All.
	aspect gputypes.TextureAspect
}

// viewFormat returns the format the view reads and writes texels in.
func (v *TextureView) viewFormat() gputypes.TextureFormat {
	if v.format == gputypes.TextureFormatUndefined && v.texture != nil {
		return v.texture.format
	}
	return v.format
}

// viewAspect returns the view's aspect, All when it is Undefined.
func (v *TextureView) viewAspect() gputypes.TextureAspect {
	if v.aspect == gputypes.TextureAspectUndefined {
		return gputypes.TextureAspectAll
	}
	return v.aspect
}

// mipLevels returns the number of mip levels in the view, at least 1.
func (v *TextureView) mipLevels() uint32 { return max(v.mipLevelCount, 1) }

// arrayLayers returns the number of array layers in the view, at least 1.
func (v *TextureView) arrayLayers() uint32 { return max(v.arrayLayerCount, 1) }

// viewDimension returns the view's dimension; Undefined (a view built
// without CreateTextureView) behaves as 2D.
func (v *TextureView) viewDimension() gputypes.TextureViewDimension {
	if v.dimension == gputypes.TextureViewDimensionUndefined {
		return gputypes.TextureViewDimension2D
	}
	return v.dimension
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
	presentMode   gputypes.PresentMode
	alphaMode     gputypes.CompositeAlphaMode
	targetKind    hal.SurfaceTargetKind
	displayHandle uintptr // X11: Display*, macOS/Windows: 0
	hwnd          uintptr // window handle for platform blit (0 = headless)
	platformBlit          // platform-specific blit resources (Windows: DIB section, Linux: X11 GC)
}

var _ hal.PixelReader = (*Surface)(nil)

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

	// Detect Wayland and bind wl_shm eagerly on the main thread, before any
	// render thread calls Present. See BUG-SW-WAYLAND-001: lazy init on first
	// present called wl_display_roundtrip concurrent with event dispatch → SIGSEGV.
	s.configurePlatformBlit()

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

// WritePixels copies RGBA pixel data directly into the surface framebuffer with
// inline BGRA swizzle. Single-pass, zero allocation. Eliminates the 3-copy chain
// (WriteTexture → render pass → blit) for CPU-rendered content.
//
// data must be RGBA8, tightly packed (bytesPerRow = width * 4).
// The surface must be configured before calling.
func (s *Surface) WritePixels(data []byte, width, height uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.configured || s.framebuffer == nil {
		return fmt.Errorf("software: WritePixels: surface not configured")
	}
	if width != s.width || height != s.height {
		return fmt.Errorf("software: WritePixels: size mismatch (%dx%d vs %dx%d)", width, height, s.width, s.height)
	}

	srcSize := int(width) * int(height) * 4
	if len(data) < srcSize || len(s.framebuffer) < srcSize {
		return fmt.Errorf("software: WritePixels: buffer too small")
	}

	bgra := isBGRA(s.format)
	if bgra {
		for i := 0; i < srcSize; i += 4 {
			s.framebuffer[i+0] = data[i+2] // B←R
			s.framebuffer[i+1] = data[i+1] // G
			s.framebuffer[i+2] = data[i+0] // R←B
			s.framebuffer[i+3] = data[i+3] // A
		}
	} else {
		copy(s.framebuffer[:srcSize], data[:srcSize])
	}
	return nil
}

// PresentPixels combines WritePixels + window blit in a single call, bypassing
// the WebGPU render pass pipeline entirely. This is the fastest path for
// CPU-rendered content: RGBA data is swizzled into the DIB section framebuffer
// and immediately BitBlt'd to the window in one pass.
//
// damageRects restricts the blit to specific regions (nil = full surface).
// In headless mode (hwnd=0), pixels are written to the framebuffer but no blit occurs.
func (s *Surface) PresentPixels(data []byte, width, height uint32, damageRects []image.Rectangle) error {
	// WritePixels handles locking, validation, and RGBA→BGRA swizzle.
	if err := s.WritePixels(data, width, height); err != nil {
		return err
	}

	// Blit to window (no-op if headless).
	if s.hwnd == 0 {
		return nil
	}

	if len(damageRects) > 0 {
		s.blitDamageRectsToWindow(s.framebuffer, int32(width), int32(height), damageRects)
	} else {
		s.blitFramebufferToWindow(s.framebuffer, int32(width), int32(height))
	}
	return nil
}

// ActualExtent returns the configured surface dimensions.
// The software backend does not clamp the extent, so these always match
// the requested values. Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.width, s.height
}

// ReadPixels returns a copy of the current framebuffer data in RGBA byte
// order (thread-safe). If the surface format is BGRA, R and B channels are
// swapped so callers always receive consistent RGBA data. The returned slice
// is caller-owned and remains valid after later rendering or surface release.
func (s *Surface) ReadPixels() []byte {
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

// GetFramebuffer returns an owned RGBA snapshot for compatibility with
// existing software HAL callers. New root-wgpu callers should use
// Surface.ReadPixels.
func (s *Surface) GetFramebuffer() []byte {
	return s.ReadPixels()
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
	// vertex and fragment are the parsed SPIR-V of the stages for this
	// pipeline (see stageModule): specialized for it when the shader module
	// has overrides. nil if the shader module has no SPIR-V.
	vertex   *shader.Module
	fragment *shader.Module
	// fragmentMayDiscard is fragment.MayDiscard(), computed once at creation.
	fragmentMayDiscard bool
}

// SamplerResource implements hal.Sampler with actual sampler parameters.
// Used by the SPIR-V interpreter for texture filtering and addressing modes.
type SamplerResource struct {
	Resource
	id   uint64 // unique ID for handle resolution
	Desc *hal.SamplerDescriptor
}

// NativeHandle returns the sampler's unique ID for handle resolution.
func (s *SamplerResource) NativeHandle() uintptr { return uintptr(s.id) }

// BindGroup stores bound resources for the software backend.
// It resolves handle-based entries to concrete software resource pointers.
// bufferSlice holds a buffer reference with the offset and size from BufferBinding.
type bufferSlice struct {
	buf    *Buffer
	offset uint64
	size   uint64
}

type BindGroup struct {
	Resource
	desc             *hal.BindGroupDescriptor
	textureViews     map[uint32]*TextureView     // binding index -> resolved texture view
	buffers          map[uint32]*Buffer          // binding index -> resolved buffer (legacy, offset=0)
	bufferBindings   map[uint32]bufferSlice      // binding index -> buffer + offset/size
	samplers         map[uint32]*SamplerResource // binding index -> resolved sampler
	dynamicOffsets   []uint32                    // applied via SetBindGroup
	hasDynamicOffset map[uint32]bool             // binding index -> true if HasDynamicOffset
}

// ComputePipeline stores compute pipeline configuration for the software backend.
// It holds a reference to the parsed SPIR-V module and entry point name, which
// are used by ComputePassEncoder.Dispatch to invoke the SPIR-V interpreter.
type ComputePipeline struct {
	Resource
	desc       *hal.ComputePipelineDescriptor
	module     *shader.Module // parsed SPIR-V of the compute stage (see stageModule)
	entryPoint string
}

// ShaderModule stores shader source for the software backend.
// When SPIR-V bytecode is available (directly or compiled from WGSL via naga),
// the parsed Module is cached for use by the SPIR-V interpreter in draw calls.
//
// A WGSL module with overrides has no SPIR-V of its own: every pipeline
// stage specializes irModule and parses its own SPIR-V (stageModule).
type ShaderModule struct {
	Resource
	desc     *hal.ShaderModuleDescriptor
	irModule *shaderir.Module // lowered WGSL (nil for SPIR-V input or if lowering failed)
	spirv    []uint32         // SPIR-V bytecode (nil if unavailable)
	parsed   *shader.Module   // parsed SPIR-V module (nil until first use)
}

// ParsedModule returns the parsed SPIR-V module, parsing on first access.
// Returns nil if no SPIR-V bytecode is available.
func (sm *ShaderModule) ParsedModule() *shader.Module {
	if sm.parsed != nil {
		return sm.parsed
	}
	if len(sm.spirv) == 0 {
		return nil
	}
	m, err := shader.ParseModule(sm.spirv)
	if err != nil {
		return nil
	}
	sm.parsed = m
	return sm.parsed
}
