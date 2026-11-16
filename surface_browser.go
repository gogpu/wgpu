//go:build js && wasm

package wgpu

import (
	"fmt"
	"image"
	"syscall/js"

	"github.com/gogpu/wgpu/internal/browser"
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

// Surface represents a platform rendering surface.
// On browser, this wraps an HTMLCanvasElement + GPUCanvasContext.
//
// The lifecycle implementation lives in internal/surface (ADR-070 S10); this
// type is the declaration layer the public API is frozen on, and it keeps the
// target ownership source, which is a public value the destination cannot name.
//
// Matches Rust wgpu WebSurface which holds a Canvas enum, a GpuCanvasContext,
// and an optional Gpu reference.
type Surface struct {
	impl *surface.Surface

	// targetSource is retained only for CreateSurfaceFromTarget.
	targetSource SurfaceTarget
}

// CreateSurface creates a rendering surface from a legacy numeric canvas
// handle. New code should prefer CreateSurfaceFromTarget,
// CreateSurfaceUnsafe, or CreateSurfaceFromCanvas.
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
	return i.createSurfaceFromCanvasID(windowHandle, nil)
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
	return i.createSurfaceFromTarget(rawTarget, target)
}

// CreateSurfaceUnsafe creates a browser surface from a raw numeric canvas ID
// without retaining an ownership source.
func (i *Instance) CreateSurfaceUnsafe(target SurfaceTargetUnsafe) (*Surface, error) {
	if i == nil || i.released {
		return nil, ErrReleased
	}
	if err := target.validate(); err != nil {
		return nil, err
	}
	return i.createSurfaceFromTarget(target, nil)
}

func (i *Instance) createSurfaceFromTarget(target SurfaceTargetUnsafe, targetSource SurfaceTarget) (*Surface, error) {
	if target.kind != surfaceTargetWebCanvasID {
		return nil, fmt.Errorf("%w: browser backend requires a Web canvas ID", ErrUnsupportedSurfaceTarget)
	}
	return i.createSurfaceFromCanvasID(target.windowHandle, targetSource)
}

func (i *Instance) createSurfaceFromCanvasID(windowHandle uintptr, targetSource SurfaceTarget) (*Surface, error) {
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

	surface, err := i.createSurfaceFromCanvas(canvas)
	if err != nil {
		return nil, err
	}
	surface.targetSource = targetSource
	return surface, nil
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
	return &Surface{impl: surface.NewBrowser(bs)}, nil
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
	var host surface.DeviceHost
	if device != nil && device.impl != nil {
		host = device.impl
	}
	return s.impl.Configure(host, surfaceConfiguration(config))
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

// Unconfigure removes the surface configuration.
// After this call, GetCurrentTexture() will fail until Configure is called again.
func (s *Surface) Unconfigure() {
	s.impl.Unconfigure()
}

// GetCurrentTexture acquires the next texture for rendering.
//
// Returns the surface texture and whether the surface is suboptimal (always
// false on browser). The texture is automatically presented when the command
// buffer using it is submitted and control returns to the browser event loop.
//
// Matches Rust wgpu SurfaceInterface::get_current_texture for WebSurface.
func (s *Surface) GetCurrentTexture() (*SurfaceTexture, bool, error) {
	impl, suboptimal, err := s.impl.GetCurrentTexture()
	if err != nil {
		return nil, false, err
	}
	st := &SurfaceTexture{impl: impl}
	// The browser surface creates one resource texture per acquisition; the
	// public wrapper is cached on it, so AsTexture and Texture keep returning
	// the same handle the pre-move field held.
	if res := st.impl.Resource(); res != nil {
		st.texture = &Texture{impl: res}
		res.Pub = st.texture
	}
	return st, suboptimal, nil
}

// Present presents a surface texture to the screen.
//
// On browser, this is a NO-OP. The swapchain is presented automatically when
// control returns to the browser event loop. This matches Rust wgpu where
// WebSurfaceOutputDetail::present() is an empty function.
func (s *Surface) Present(_ *SurfaceTexture) error {
	// No-op on browser. Presentation is automatic, and the receiver is never
	// read, so a surface without an implementation handle (a zero value) still
	// answers nil as it always has.
	return nil
}

// PresentWithDamage presents a surface texture, optionally with damage rects.
// On browser, damage rects are ignored — the browser composites the full canvas.
// This method exists for API compatibility with the native backend.
func (s *Surface) PresentWithDamage(st *SurfaceTexture, _ []image.Rectangle) error {
	return s.impl.PresentWithDamage(surfaceTextureRef(st), nil)
}

// ReadPixels is not supported by browser WebGPU surfaces.
// Headless surface readback is a Pure-Go software-backend extension.
func (s *Surface) ReadPixels() ([]byte, error) {
	if s == nil {
		return nil, ErrReleased
	}
	return s.impl.ReadPixels()
}

// ActualExtent returns the configured surface dimensions.
// On browser, the canvas dimensions are always used as-is (no driver clamping).
// Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	return s.impl.ActualExtent()
}

// DiscardTexture discards the acquired surface texture without presenting it.
//
// On browser, this is a NO-OP. The browser does not support discarding a
// surface texture -- it will be presented regardless when the event loop runs.
// Matches Rust wgpu where WebSurfaceOutputDetail::texture_discard() is a no-op.
func (s *Surface) DiscardTexture() {
	s.impl.DiscardTexture()
}

// PresentPixels is not supported on browser backend.
// Software-backend extension for direct pixel presentation. The backend never
// reads the pixels, so the declaration layer passes the destination the zero
// values; the golden records the unnamed parameters this signature has always
// had.
func (s *Surface) PresentPixels(_ []byte, _, _ uint32, _ []image.Rectangle) error {
	if s == nil {
		// The pre-move browser method never dereferenced the receiver: a nil
		// surface kept answering that the call is unsupported.
		return fmt.Errorf("wgpu: PresentPixels is not supported on the browser backend")
	}
	return s.impl.PresentPixels(nil, 0, 0, nil)
}

// WritePixels is not supported on browser backend.
// Software-backend extension for direct framebuffer write. The pixels are
// never read, as for PresentPixels.
func (s *Surface) WritePixels(_ []byte, _, _ uint32) error {
	if s == nil {
		return fmt.Errorf("wgpu: WritePixels is not supported on the browser backend")
	}
	return s.impl.WritePixels(nil, 0, 0)
}

// SetPresentsWithTransaction is a no-op on browser backend.
// Core Animation transactions are macOS-only.
func (s *Surface) SetPresentsWithTransaction(_ bool) {
	if s == nil {
		return // the pre-move no-op never touched the receiver
	}
	s.impl.SetPresentsWithTransaction(false)
}

// Release releases the surface: the implementation destroys the browser
// surface, and this layer then drops the target ownership source.
func (s *Surface) Release() {
	s.impl.Release()
	s.targetSource = nil
}

// surfaceTextureRef returns the implementation handle of st, or nil for an
// absent wrapper: the destination reports the same nil-texture answer.
func surfaceTextureRef(st *SurfaceTexture) *surface.SurfaceTexture {
	if st == nil {
		return nil
	}
	return &st.impl
}

// surfaceTextureViewDescriptor converts the public view descriptor into the
// surface domain's mirror, or nil for an absent descriptor.
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

// SurfaceTexture is a texture acquired from a surface for rendering.
//
// On browser, the texture is automatically presented when the command buffer
// using it is submitted and control returns to the browser event loop.
//
// The implementation is embedded by value: an acquisition allocates the public
// wrapper once, exactly as before the move (ADR-070 §15.7).
type SurfaceTexture struct {
	impl surface.SurfaceTexture

	// texture is the public wrapper of the acquisition's resource texture,
	// cached on it so repeated accessors return one handle.
	texture *Texture
}

// AsTexture returns the underlying Texture for direct WriteTexture access.
func (st *SurfaceTexture) AsTexture() *Texture { return st.texture }

// CreateView creates a texture view of this surface texture.
//
// Pass nil for desc to create a default view (all mips, all layers, same format).
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	rv, err := st.impl.CreateView(surfaceTextureViewDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return &TextureView{impl: rv}, nil
}

// Texture returns the underlying Texture. This is useful for operations that
// need direct texture access (e.g., creating additional views).
func (st *SurfaceTexture) Texture() *Texture {
	return st.texture
}
