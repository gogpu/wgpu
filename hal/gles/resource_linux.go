// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// Surface implements hal.Surface for OpenGL on Linux.
// When Instance has a pre-created context (X11/headless), ownsContext=false —
// Surface shares Instance's context (like Windows AdapterContext pattern).
// When Instance has no context (Wayland), ownsContext=true — Surface owns its own.
type Surface struct {
	displayHandle uintptr
	windowHandle  uintptr
	eglCtx        *egl.Context
	eglDisplay    egl.EGLDisplay
	eglSurface    egl.EGLSurface
	glCtx         *gl.Context
	ownsContext   bool // true = Surface owns context, false = shared from Instance
	version       string
	renderer      string
	configured    bool
	config        *hal.SurfaceConfiguration

	// Wayland-specific: wl_egl_window handle. On Wayland, EGL cannot use
	// the raw wl_surface* directly — it needs a wl_egl_window wrapper.
	// Created in Configure via libwayland-egl.so, destroyed in Destroy.
	// Zero on X11 (where windowHandle is used directly with eglCreateWindowSurface).
	eglWindow uintptr // wl_egl_window* (0 on X11 or before Configure)
	isWayland bool    // true if the Context was created for Wayland

	// Swapchain offscreen framebuffer. User render passes that target this
	// Surface render into swapchainFBO (backed by colorRenderbuffer), not FBO 0.
	// Queue.Present blits this FBO to the default framebuffer with an explicit
	// Y-flip before SwapBuffers. Mirrors Rust wgpu-hal/src/gles/egl.rs
	// Surface::configure (1537-1562) / Surface::present (1280-1308).
	swapchainFBO        uint32
	colorRenderbuffer   uint32
	fboWidth, fboHeight uint32
}

// GetAdapterInfo returns adapter information from this surface's GL context.
// Probes GL version, extensions, features, limits, and MSAA support to build
// an accurate ExposedAdapter. Follows Rust wgpu-hal adapter.rs expose pattern.
func (s *Surface) GetAdapterInfo() hal.ExposedAdapter {
	caps := queryAdapterCapabilities(s.glCtx)

	driverInfo := "OpenGL 3.3+"
	if caps.IsES {
		driverInfo = fmt.Sprintf("OpenGL ES %d.%d", caps.GLMajor, caps.GLMinor)
	} else if caps.GLMajor > 0 {
		driverInfo = fmt.Sprintf("OpenGL %d.%d", caps.GLMajor, caps.GLMinor)
	}

	return hal.ExposedAdapter{
		Adapter: &Adapter{
			glCtx:         s.glCtx,
			eglCtx:        s.eglCtx,
			displayHandle: s.displayHandle,
			windowHandle:  s.windowHandle,
			version:       s.version,
			renderer:      s.renderer,
			caps:          caps,
		},
		Info: gputypes.AdapterInfo{
			Name:       caps.Renderer,
			Vendor:     caps.Vendor,
			VendorID:   caps.VendorID,
			DeviceID:   0,
			DeviceType: caps.DeviceType,
			Driver:     caps.Version,
			DriverInfo: driverInfo,
			Backend:    gputypes.BackendGL,
		},
		Features: caps.Features,
		Capabilities: hal.Capabilities{
			Limits: caps.Limits,
			AlignmentsMask: hal.Alignments{
				BufferCopyOffset: 4,
				BufferCopyPitch:  4,
			},
			DownlevelCapabilities: hal.DownlevelCapabilities{
				ShaderModel: 50, // SM5.0
				Flags:       caps.DownlevelFlags,
			},
		},
	}
}

// Configure configures the surface for presentation.
//
// On Wayland, this creates a wl_egl_window via libwayland-egl.so and then
// an EGL window surface. On X11, eglCreateWindowSurface takes the raw X11
// Window handle directly.
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

	// Create EGL window surface if not yet created (first Configure call).
	// On Wayland: need wl_egl_window → eglCreateWindowSurface.
	// On X11: eglCreateWindowSurface with raw X11 Window.
	if s.eglSurface == 0 && s.eglCtx != nil && s.windowHandle != 0 {
		if err := s.createEGLWindowSurface(config.Width, config.Height); err != nil {
			return fmt.Errorf("gles: failed to create EGL window surface: %w", err)
		}
	}

	// On Wayland resize: update wl_egl_window dimensions.
	if s.isWayland && s.eglWindow != 0 && s.config != nil {
		if s.config.Width != config.Width || s.config.Height != config.Height {
			egl.WlEGLWindowResize(s.eglWindow, int32(config.Width), int32(config.Height), 0, 0)
		}
	}

	// Make the EGL window surface current so we can allocate GL resources.
	if s.eglSurface != 0 && s.eglDisplay != 0 {
		egl.MakeCurrent(s.eglDisplay, s.eglSurface, s.eglSurface, s.eglCtx.EGLContext())
	}

	// Allocate / resize the swapchain offscreen FBO. User render passes
	// target this FBO; Present blits it to FBO 0 with Y-flip.
	if err := s.reconfigureSwapchainFBO(config.Format, config.Width, config.Height); err != nil {
		return fmt.Errorf("gles: failed to configure swapchain framebuffer: %w", err)
	}

	s.configured = true
	s.config = config
	return nil
}

// createEGLWindowSurface creates the EGL window surface. On Wayland this
// requires creating a wl_egl_window first via libwayland-egl.so.
func (s *Surface) createEGLWindowSurface(width, height uint32) error {
	s.eglDisplay = s.eglCtx.Display()
	s.isWayland = s.eglCtx.WindowKind() == egl.WindowKindWayland

	if s.isWayland {
		return s.createWaylandEGLSurface(width, height)
	}
	return s.createX11EGLSurface()
}

// createWaylandEGLSurface creates a wl_egl_window then an EGL window surface.
// Prefers EGL 1.5 eglCreatePlatformWindowSurface (spec-correct void* native window)
// with fallback to EGL 1.4 eglCreateWindowSurface.
// Rust wgpu-hal egl.rs:1479-1491 uses the same preference order.
func (s *Surface) createWaylandEGLSurface(width, height uint32) error {
	if !egl.InitWaylandEGL() {
		return fmt.Errorf("libwayland-egl.so not available — cannot create Wayland EGL surface")
	}

	eglWin := egl.WlEGLWindowCreate(s.windowHandle, int32(width), int32(height))
	if eglWin == 0 {
		return fmt.Errorf("wl_egl_window_create failed for wl_surface 0x%x", s.windowHandle)
	}
	s.eglWindow = eglWin

	// EGL 1.5 path: eglCreatePlatformWindowSurface takes void* — spec-correct for Wayland.
	// Falls back to eglCreateWindowSurface internally if EGL 1.5 unavailable.
	attribs := []egl.EGLAttrib{egl.EGLAttrib(egl.None)}
	eglSurface := egl.CreatePlatformWindowSurface(s.eglDisplay, s.eglCtx.Config(), eglWin, &attribs[0])
	if eglSurface == egl.NoSurface {
		egl.WlEGLWindowDestroy(eglWin)
		s.eglWindow = 0
		return fmt.Errorf("eglCreatePlatformWindowSurface failed for wl_egl_window: error 0x%x", egl.GetError())
	}
	s.eglSurface = eglSurface

	eglPath := "eglCreateWindowSurface (EGL 1.4)"
	if egl.HasPlatformWindowSurface() {
		eglPath = "eglCreatePlatformWindowSurface (EGL 1.5)"
	}
	hal.Logger().Info("gles: Wayland EGL window surface created",
		"path", eglPath,
		"eglWindow", fmt.Sprintf("0x%x", eglWin),
		"eglSurface", fmt.Sprintf("0x%x", eglSurface),
		"width", width, "height", height,
	)
	return nil
}

// createX11EGLSurface creates an EGL window surface directly from an X11 Window.
func (s *Surface) createX11EGLSurface() error {
	attribs := []egl.EGLInt{egl.None}
	eglSurface := egl.CreateWindowSurface(s.eglDisplay, s.eglCtx.Config(), egl.EGLNativeWindowType(s.windowHandle), &attribs[0])
	if eglSurface == egl.NoSurface {
		return fmt.Errorf("eglCreateWindowSurface failed for X11 window 0x%x: error 0x%x", s.windowHandle, egl.GetError())
	}
	s.eglSurface = eglSurface

	hal.Logger().Info("gles: X11 EGL window surface created",
		"eglSurface", fmt.Sprintf("0x%x", eglSurface),
		"window", fmt.Sprintf("0x%x", s.windowHandle),
	)
	return nil
}

// Unconfigure marks the surface as unconfigured and releases the EGL window
// surface and wl_egl_window (on Wayland).
func (s *Surface) Unconfigure(_ hal.Device) {
	destroySwapchainFBO(s.glCtx, s.swapchainFBO, s.colorRenderbuffer)
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0
	s.fboWidth = 0
	s.fboHeight = 0

	// Destroy EGL surface before wl_egl_window (order matters).
	if s.eglSurface != 0 && s.eglDisplay != 0 {
		egl.DestroySurface(s.eglDisplay, s.eglSurface)
		s.eglSurface = 0
	}
	if s.eglWindow != 0 {
		egl.WlEGLWindowDestroy(s.eglWindow)
		s.eglWindow = 0
	}

	s.configured = false
	s.config = nil
}

// AcquireTexture returns the next surface texture for rendering.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return &hal.AcquiredSurfaceTexture{
		Texture: &SurfaceTexture{
			surface: s,
		},
		Suboptimal: false,
	}, nil
}

// DiscardTexture discards a previously acquired texture.
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {}

// ActualExtent returns the configured surface dimensions.
// GLES does not clamp the extent, so these always match the requested values.
// Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s.config == nil {
		return 0, 0
	}
	return s.config.Width, s.config.Height
}

// Destroy releases the surface resources.
// Order: GL resources → EGL surface → wl_egl_window → EGL context.
func (s *Surface) Destroy() {
	// Release swapchain FBO before tearing down the GL context.
	destroySwapchainFBO(s.glCtx, s.swapchainFBO, s.colorRenderbuffer)
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0

	// Destroy EGL surface before wl_egl_window (order matters per Wayland spec).
	if s.eglSurface != 0 && s.eglDisplay != 0 {
		egl.DestroySurface(s.eglDisplay, s.eglSurface)
		s.eglSurface = 0
	}
	if s.eglWindow != 0 {
		egl.WlEGLWindowDestroy(s.eglWindow)
		s.eglWindow = 0
	}

	// Only destroy context if Surface owns it (Wayland path).
	// When shared from Instance (X11/headless), Instance.Destroy handles cleanup.
	if s.ownsContext && s.eglCtx != nil {
		s.eglCtx.Destroy()
	}
	s.eglCtx = nil
	s.glCtx = nil
}

// SurfaceTexture implements hal.SurfaceTexture for OpenGL.
// It represents the default framebuffer.
type SurfaceTexture struct {
	surface *Surface
}

// CurrentUsage returns 0 — GLES surface textures have no state tracking.
func (t *SurfaceTexture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *SurfaceTexture) AddPendingRef()                      {}
func (t *SurfaceTexture) DecPendingRef()                      {}

// Destroy is a no-op for surface textures.
func (t *SurfaceTexture) Destroy() {}

// NativeHandle returns 0 (OpenGL default framebuffer has no handle).
func (t *SurfaceTexture) NativeHandle() uintptr { return 0 }
