//go:build js && wasm

package browser

import (
	"errors"
	"fmt"
	"syscall/js"
)

// ErrCanvasContextFailed is returned when canvas.getContext("webgpu") returns null.
// This happens when WebGPU is not available or the canvas is already bound to
// another context type (e.g., "2d" or "webgl2").
//
// Matches Rust wgpu's CreateSurfaceErrorKind::Web for the null-context case.
var ErrCanvasContextFailed = errors.New("wgpu: canvas.getContext(\"webgpu\") returned null; webgpu not available or canvas already in use")

// Surface wraps an HTML canvas element and its GPUCanvasContext.
//
// On the browser, a "surface" is a canvas + the GPUCanvasContext obtained from
// canvas.getContext("webgpu"). The context is configured with a device, format,
// and size, then getCurrentTexture() returns the next frame texture.
//
// Presentation happens automatically when the JS event loop runs -- there is no
// explicit present() call. This matches Rust wgpu WebSurface / WebSurfaceOutputDetail
// where present() and texture_discard() are both no-ops.
type Surface struct {
	// canvas is the HTMLCanvasElement (or OffscreenCanvas).
	canvas js.Value

	// context is the GPUCanvasContext from canvas.getContext("webgpu").
	context js.Value

	// gpu is the navigator.gpu reference, used for getPreferredCanvasFormat().
	gpu js.Value

	// Pre-bound context methods to avoid property lookups per frame.
	fnGetCurrentTexture js.Value
	fnConfigure         js.Value

	// Configuration state.
	configured bool
	width      uint32
	height     uint32
	format     string
}

// NewSurface creates a Surface from a canvas element by obtaining its GPUCanvasContext.
//
// The gpu parameter is the navigator.gpu object (needed for getPreferredCanvasFormat).
// The canvas must be an HTMLCanvasElement or OffscreenCanvas.
//
// Returns ErrCanvasContextFailed if getContext("webgpu") returns null (WebGPU
// not available or canvas already in use). Panics if getContext throws an
// exception (indicates misuse of canvas state).
//
// Matches Rust wgpu ContextWebGpu::create_surface_from_context.
func NewSurface(gpu js.Value, canvas js.Value) (*Surface, error) {
	// Call canvas.getContext("webgpu"). This may return null (not supported
	// or canvas already has another context) or throw (canvas state misuse).
	//
	// See: https://html.spec.whatwg.org/multipage/canvas.html#dom-canvas-getcontext
	context := canvas.Call("getContext", "webgpu")
	if context.IsNull() || context.IsUndefined() {
		return nil, ErrCanvasContextFailed
	}

	s := &Surface{
		canvas:  canvas,
		context: context,
		gpu:     gpu,
	}

	// Pre-bind context methods for per-frame performance.
	s.fnGetCurrentTexture = bindMethod(context, "getCurrentTexture")
	s.fnConfigure = bindMethod(context, "configure")

	return s, nil
}

// Configure sets the surface configuration on the GPUCanvasContext.
//
// This sets the canvas dimensions and calls context.configure() with the
// provided parameters. After Configure, GetCurrentTexture() can be called.
//
// Matches Rust wgpu SurfaceInterface::configure for WebSurface.
func (s *Surface) Configure(config js.Value, width, height uint32, format string) {
	// Set canvas dimensions (Rust wgpu does this in configure too).
	s.canvas.Set("width", width)
	s.canvas.Set("height", height)

	// Call context.configure(config).
	s.fnConfigure.Invoke(config)

	s.configured = true
	s.width = width
	s.height = height
	s.format = format
}

// Unconfigure removes the surface configuration.
// After this call, GetCurrentTexture() will fail until Configure is called again.
func (s *Surface) Unconfigure() {
	unconfigure := s.context.Get("unconfigure")
	if !unconfigure.IsUndefined() && !unconfigure.IsNull() {
		s.context.Call("unconfigure")
	}
	s.configured = false
}

// GetCurrentTexture returns the current frame texture from the GPUCanvasContext.
//
// The returned GPUTexture is automatically presented when control returns to
// the browser event loop after the command buffer using it is submitted.
//
// Returns an error if the surface is not configured.
//
// Matches Rust wgpu SurfaceInterface::get_current_texture for WebSurface.
func (s *Surface) GetCurrentTexture() (*Texture, error) {
	if !s.configured {
		return nil, fmt.Errorf("wgpu: surface not configured")
	}

	jsTexture := s.fnGetCurrentTexture.Invoke()
	if jsTexture.IsNull() || jsTexture.IsUndefined() {
		return nil, fmt.Errorf("wgpu: getCurrentTexture returned null")
	}

	return NewTexture(jsTexture), nil
}

// GetPreferredCanvasFormat returns the preferred texture format for the display.
//
// This calls navigator.gpu.getPreferredCanvasFormat() which returns either
// "bgra8unorm" or "rgba8unorm" depending on the platform.
//
// See: https://www.w3.org/TR/webgpu/#dom-gpu-getpreferredcanvasformat
//
// Matches Rust wgpu SurfaceInterface::get_capabilities which reads
// gpu.get_preferred_canvas_format() to order the formats list.
func (s *Surface) GetPreferredCanvasFormat() string {
	if s.gpu.IsUndefined() || s.gpu.IsNull() {
		return "bgra8unorm" // safe fallback
	}
	return s.gpu.Call("getPreferredCanvasFormat").String()
}

// Canvas returns the underlying canvas js.Value.
func (s *Surface) Canvas() js.Value { return s.canvas }

// Context returns the underlying GPUCanvasContext js.Value.
func (s *Surface) Context() js.Value { return s.context }

// Width returns the configured canvas width in pixels.
func (s *Surface) Width() uint32 { return s.width }

// Height returns the configured canvas height in pixels.
func (s *Surface) Height() uint32 { return s.height }

// Format returns the configured texture format string (e.g., "bgra8unorm").
func (s *Surface) Format() string { return s.format }

// Configured reports whether the surface has been configured.
func (s *Surface) Configured() bool { return s.configured }

// Destroy releases the surface. On browser this unconfigures the context.
// Matches Rust wgpu Drop for WebSurface (no-op in Rust, but we unconfigure
// for clean teardown).
func (s *Surface) Destroy() {
	if s.configured {
		s.Unconfigure()
	}
}
