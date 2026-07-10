//go:build js && wasm

package wgpu

import (
	"fmt"
	"image"
	"syscall/js"

	"github.com/gogpu/wgpu/internal/browser"
)

// Surface represents a platform rendering surface.
// On browser, this wraps an HTMLCanvasElement + GPUCanvasContext.
//
// Matches Rust wgpu WebSurface which holds a Canvas enum, a GpuCanvasContext,
// and an optional Gpu reference.
type Surface struct {
	browser  *browser.Surface
	device   *Device
	released bool

	// Cached configuration for GetCurrentTexture texture creation.
	configFormat TextureFormat
}

// CreateSurface creates a rendering surface from an HTML canvas element.
//
// On browser, displayHandle is ignored and windowHandle is treated as a
// numeric canvas element ID (data-raw-handle attribute lookup). If windowHandle
// is 0, the first <canvas> element in the document is used.
//
// For direct js.Value canvas access, use CreateSurfaceFromCanvas instead.
//
// Matches Rust wgpu InstanceInterface::create_surface for WebSurface which
// uses RawWindowHandle::Web to query the DOM by data-raw-handle attribute.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (*Surface, error) {
	if i.released {
		return nil, ErrReleased
	}

	// On browser, resolve a canvas element from the DOM.
	var canvas js.Value
	doc := js.Global().Get("document")

	if windowHandle == 0 {
		// Default: use the first <canvas> in the document.
		canvas = doc.Call("querySelector", "canvas")
	} else {
		// Lookup by data-raw-handle attribute (Rust wgpu convention).
		selector := fmt.Sprintf("[data-raw-handle=\"%d\"]", windowHandle)
		canvas = doc.Call("querySelector", selector)
	}

	if canvas.IsNull() || canvas.IsUndefined() {
		return nil, fmt.Errorf("wgpu: no canvas element found for handle %d", windowHandle)
	}

	return i.createSurfaceFromCanvas(canvas)
}

// CreateSurfaceFromCanvas creates a rendering surface from a js.Value canvas.
//
// This is the browser-specific entry point that accepts a direct canvas
// reference (HTMLCanvasElement or OffscreenCanvas). Use this when you have
// a canvas js.Value from JavaScript interop.
//
// Matches Rust wgpu's SurfaceTarget::Canvas variant which takes an
// HtmlCanvasElement directly.
func (i *Instance) CreateSurfaceFromCanvas(canvas js.Value) (*Surface, error) {
	if i.released {
		return nil, ErrReleased
	}
	return i.createSurfaceFromCanvas(canvas)
}

// createSurfaceFromCanvas is the shared implementation for both CreateSurface
// and CreateSurfaceFromCanvas.
func (i *Instance) createSurfaceFromCanvas(canvas js.Value) (*Surface, error) {
	bs, err := browser.NewSurface(i.browser.GPU(), canvas)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface: %w", err)
	}
	return &Surface{browser: bs}, nil
}

// Configure configures the surface for presentation.
//
// Must be called before GetCurrentTexture(). Sets the canvas dimensions and
// calls GPUCanvasContext.configure() with the device, format, usage, and alpha mode.
//
// On browser, PresentMode is ignored (browser always uses FIFO / VSync).
//
// Matches Rust wgpu SurfaceInterface::configure for WebSurface.
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

	// Validate present mode (Rust wgpu panics on Mailbox/Immediate on web).
	switch config.PresentMode {
	case PresentModeMailbox, PresentModeImmediate:
		return fmt.Errorf("wgpu: present mode %v not supported on browser; only Fifo is supported", config.PresentMode)
	}

	// Build the JS GPUCanvasConfiguration object.
	jsConfig := browser.BuildSurfaceConfiguration(
		device.browser.Ref(),
		config.Format,
		config.Usage,
		config.AlphaMode,
		nil, // viewFormats -- SurfaceConfiguration doesn't expose them yet
	)

	formatStr := browser.TextureFormatToJS(config.Format)
	s.browser.Configure(jsConfig, config.Width, config.Height, formatStr)
	s.device = device
	s.configFormat = config.Format
	return nil
}

// Unconfigure removes the surface configuration.
// After this call, GetCurrentTexture() will fail until Configure is called again.
func (s *Surface) Unconfigure() {
	if s.released {
		return
	}
	s.browser.Unconfigure()
	s.device = nil
}

// GetCurrentTexture acquires the next texture for rendering.
//
// Returns the surface texture and whether the surface is suboptimal (always
// false on browser). The texture is automatically presented when the command
// buffer using it is submitted and control returns to the browser event loop.
//
// Matches Rust wgpu SurfaceInterface::get_current_texture for WebSurface.
func (s *Surface) GetCurrentTexture() (*SurfaceTexture, bool, error) {
	if s.released {
		return nil, false, ErrReleased
	}
	if s.device == nil {
		return nil, false, fmt.Errorf("wgpu: surface not configured")
	}

	bt, err := s.browser.GetCurrentTexture()
	if err != nil {
		return nil, false, fmt.Errorf("wgpu: %w", err)
	}

	return &SurfaceTexture{
		texture: &Texture{
			browser: bt,
			format:  s.configFormat,
		},
	}, false, nil // Browser surfaces are never suboptimal
}

// Present presents a surface texture to the screen.
//
// On browser, this is a NO-OP. The swapchain is presented automatically when
// control returns to the browser event loop. This matches Rust wgpu where
// WebSurfaceOutputDetail::present() is an empty function.
func (s *Surface) Present(_ *SurfaceTexture) error {
	// No-op on browser. Presentation is automatic.
	return nil
}

// PresentWithDamage presents a surface texture, optionally with damage rects.
// On browser, damage rects are ignored — the browser composites the full canvas.
// This method exists for API compatibility with the native backend.
func (s *Surface) PresentWithDamage(st *SurfaceTexture, _ []image.Rectangle) error {
	return s.Present(st)
}

// ActualExtent returns the configured surface dimensions.
// On browser, the canvas dimensions are always used as-is (no driver clamping).
// Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s.released || s.browser == nil {
		return 0, 0
	}
	return s.browser.Width(), s.browser.Height()
}

// DiscardTexture discards the acquired surface texture without presenting it.
//
// On browser, this is a NO-OP. The browser does not support discarding a
// surface texture -- it will be presented regardless when the event loop runs.
// Matches Rust wgpu where WebSurfaceOutputDetail::texture_discard() is a no-op.
func (s *Surface) DiscardTexture() {
	// No-op on browser. Cannot discard the texture.
}

// Release releases the surface.
func (s *Surface) Release() {
	if s.released {
		return
	}
	s.released = true
	if s.browser != nil {
		s.browser.Destroy()
	}
}

// SurfaceTexture is a texture acquired from a surface for rendering.
//
// On browser, the texture is automatically presented when the command buffer
// using it is submitted and control returns to the browser event loop.
type SurfaceTexture struct {
	texture  *Texture
	released bool
}

// AsTexture returns the underlying Texture for direct WriteTexture access.
func (st *SurfaceTexture) AsTexture() *Texture { return st.texture }

// CreateView creates a texture view of this surface texture.
//
// Pass nil for desc to create a default view (all mips, all layers, same format).
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	if st.released || st.texture == nil || st.texture.browser == nil {
		return nil, ErrReleased
	}

	var jsDesc js.Value
	if desc != nil {
		jsDesc = browser.BuildTextureViewDescriptor(
			desc.Label,
			desc.Format, desc.Dimension, desc.Aspect,
			desc.BaseMipLevel, desc.MipLevelCount,
			desc.BaseArrayLayer, desc.ArrayLayerCount,
		)
	} else {
		jsDesc = js.Undefined()
	}

	bv := st.texture.browser.CreateView(jsDesc)
	return &TextureView{
		browser: bv,
		texture: st.AsTexture(),
	}, nil
}

// Texture returns the underlying Texture. This is useful for operations that
// need direct texture access (e.g., creating additional views).
func (st *SurfaceTexture) Texture() *Texture {
	return st.texture
}
