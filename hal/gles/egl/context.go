// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package egl

import (
	"fmt"
	"strings"
	"unsafe"
)

// Context wraps an EGL rendering context with its display, config, and surface.
type Context struct {
	display      EGLDisplay
	config       EGLConfig
	context      EGLContext
	pbuffer      EGLSurface
	windowKind   WindowKind
	displayOwner *DisplayOwner // owns native display connection (X11); closed after eglTerminate
}

// ContextConfig holds configuration options for creating an EGL context.
type ContextConfig struct {
	// GLVersionMajor is the major OpenGL version (e.g., 3 for OpenGL 3.3).
	GLVersionMajor int
	// GLVersionMinor is the minor OpenGL version (e.g., 3 for OpenGL 3.3).
	GLVersionMinor int
	// CoreProfile requests a core profile context (vs compatibility).
	CoreProfile bool
	// Debug enables debug context with validation.
	Debug bool
	// GLES requests OpenGL ES instead of desktop OpenGL.
	GLES bool
	// Surfaceless creates a context without a surface (headless rendering).
	Surfaceless bool
	// NativeDisplay is the native display handle to use for EGL display creation.
	// On Wayland: must be the app's wl_display* — passing 0 causes EGL to open
	// a second connection, which makes wl_surface proxies mismatched on configure.
	// On X11: the X11 Display*. Zero uses the default display.
	NativeDisplay uintptr
}

// DefaultContextConfig returns a sensible default context configuration.
// Creates an OpenGL 3.3 core profile context.
func DefaultContextConfig() ContextConfig {
	return ContextConfig{
		GLVersionMajor: 3,
		GLVersionMinor: 3,
		CoreProfile:    true,
		Debug:          false,
		GLES:           false,
		Surfaceless:    false,
	}
}

// NewContext creates a new EGL context with automatic platform detection.
// It detects the window system (X11, Wayland, or Surfaceless) and creates
// an appropriate EGL context.
func NewContext(config ContextConfig) (*Context, error) {
	// Get EGL display for the detected platform.
	// displayOwner (non-nil for X11) keeps the native display connection alive.
	display, windowKind, displayOwner, err := GetEGLDisplay(config.NativeDisplay)
	if err != nil {
		return nil, fmt.Errorf("failed to get EGL display: %w", err)
	}

	// closeOwner is a helper to close the display owner on error paths.
	closeOwner := func() {
		if displayOwner != nil {
			displayOwner.Close()
		}
	}

	// Initialize EGL
	var major, minor EGLInt
	if Initialize(display, &major, &minor) == False {
		closeOwner()
		return nil, fmt.Errorf("eglInitialize failed: error 0x%x", GetError())
	}

	// Bind OpenGL or OpenGL ES API
	api := OpenGLAPI
	if config.GLES {
		api = OpenGLESAPI
	}
	if BindAPI(api) == False {
		Terminate(display)
		closeOwner()
		return nil, fmt.Errorf("eglBindAPI failed: error 0x%x", GetError())
	}

	// Choose EGL frame buffer configuration
	eglConfig, err := chooseEGLConfig(display, config)
	if err != nil {
		Terminate(display)
		closeOwner()
		return nil, fmt.Errorf("failed to choose EGL config: %w", err)
	}

	// Create EGL context
	eglContext := createEGLContext(display, eglConfig, config)
	if eglContext == NoContext {
		Terminate(display)
		closeOwner()
		return nil, fmt.Errorf("eglCreateContext failed: error 0x%x", GetError())
	}

	// Surfaceless context: EGL 1.5+ or EGL_KHR_surfaceless_context allows
	// MakeCurrent with EGL_NO_SURFACE. Skip pbuffer creation in that case.
	// Fallback to 1×1 pbuffer for older drivers.
	// Matches Rust wgpu-hal egl.rs:735-758.
	hasSurfaceless := (major > 1 || (major == 1 && minor >= 5))
	if !hasSurfaceless {
		displayExts := QueryString(display, Extensions)
		hasSurfaceless = strings.Contains(displayExts, "EGL_KHR_surfaceless_context")
	}

	var pbuffer EGLSurface
	if hasSurfaceless {
		pbuffer = NoSurface
	} else {
		pbuffer = createPbufferSurface(display, eglConfig)
		if pbuffer == NoSurface {
			DestroyContext(display, eglContext)
			Terminate(display)
			closeOwner()
			return nil, fmt.Errorf("eglCreatePbufferSurface failed and no surfaceless support: error 0x%x", GetError())
		}
	}

	return &Context{
		display:      display,
		config:       eglConfig,
		context:      eglContext,
		pbuffer:      pbuffer,
		windowKind:   windowKind,
		displayOwner: displayOwner,
	}, nil
}

// chooseEGLConfig selects an appropriate EGL frame buffer configuration.
func chooseEGLConfig(display EGLDisplay, config ContextConfig) (EGLConfig, error) {
	// Determine renderable type
	var renderableType EGLInt
	if config.GLES {
		switch {
		case config.GLVersionMajor >= 3:
			renderableType = OpenGLES3Bit
		case config.GLVersionMajor >= 2:
			renderableType = OpenGLES2Bit
		default:
			renderableType = OpenGLESBit
		}
	} else {
		renderableType = OpenGLBit
	}

	// Tiered config selection (Rust wgpu-hal egl.rs:218-293).
	// Try from best to worst: the config must support pbuffer (for headless
	// MakeCurrent) and ideally window (for later CreateWindowSurface).
	tiers := []EGLInt{
		WindowBit | PbufferBit, // Tier 1: window + pbuffer (can present later)
		PbufferBit,             // Tier 0: pbuffer only (headless/CI fallback)
	}

	baseAttribs := []EGLInt{
		RenderableType, renderableType,
		RedSize, 8,
		GreenSize, 8,
		BlueSize, 8,
		AlphaSize, 8,
		DepthSize, 24,
		StencilSize, 8,
	}

	for _, surfaceType := range tiers {
		attribs := make([]EGLInt, 0, len(baseAttribs)+3)
		attribs = append(attribs, SurfaceType, surfaceType)
		attribs = append(attribs, baseAttribs...)
		attribs = append(attribs, None)

		var eglConfig EGLConfig
		var numConfigs EGLInt
		if ChooseConfig(display, &attribs[0], &eglConfig, 1, &numConfigs) == False {
			continue
		}
		if numConfigs > 0 {
			return eglConfig, nil
		}
	}

	return 0, fmt.Errorf("no suitable EGL configs found (tried window+pbuffer and pbuffer-only)")
}

// createEGLContext creates an EGL rendering context.
func createEGLContext(display EGLDisplay, config EGLConfig, cfg ContextConfig) EGLContext {
	var attribs []EGLInt

	// Set OpenGL version
	attribs = append(attribs,
		ContextMajorVersion, EGLInt(cfg.GLVersionMajor),
		ContextMinorVersion, EGLInt(cfg.GLVersionMinor),
	)

	// Set profile (core vs compatibility)
	if cfg.CoreProfile {
		attribs = append(attribs,
			ContextOpenGLProfileMask, ContextOpenGLCoreProfileBit,
		)
	}

	// Enable debug context if requested
	if cfg.Debug {
		attribs = append(attribs,
			ContextFlagsKHR, ContextOpenGLDebugBitKHR,
		)
	}

	// Terminate attribute list
	attribs = append(attribs, None)

	return CreateContext(display, config, NoContext, &attribs[0])
}

// createPbufferSurface creates a minimal pbuffer surface for the context.
func createPbufferSurface(display EGLDisplay, config EGLConfig) EGLSurface {
	attribs := []EGLInt{
		Width, 16,
		Height, 16,
		None,
	}
	return CreatePbufferSurface(display, config, &attribs[0])
}

// MakeCurrent makes this context current on the pbuffer (headless rendering).
func (c *Context) MakeCurrent() error {
	if MakeCurrent(c.display, c.pbuffer, c.pbuffer, c.context) == False {
		return fmt.Errorf("eglMakeCurrent failed: error 0x%x", GetError())
	}
	return nil
}

// MakeCurrentSurface makes this context current on a window surface (for Present).
func (c *Context) MakeCurrentSurface(surface EGLSurface) error {
	if MakeCurrent(c.display, surface, surface, c.context) == False {
		return fmt.Errorf("eglMakeCurrent(surface) failed: error 0x%x", GetError())
	}
	return nil
}

// CreateWindowSurface creates an EGL window surface for presentation.
// The surface shares this context's display and config.
func (c *Context) CreateWindowSurface(nativeWindow uintptr) (EGLSurface, error) {
	attribs := []EGLInt{None}
	surface := CreateWindowSurface(c.display, c.config, EGLNativeWindowType(nativeWindow), &attribs[0])
	if surface == NoSurface {
		return NoSurface, fmt.Errorf("eglCreateWindowSurface failed: error 0x%x", GetError())
	}
	return surface, nil
}

// Display returns the EGL display handle.
func (c *Context) Display() EGLDisplay { return c.display }

// Config returns the EGL config handle.
func (c *Context) Config() EGLConfig { return c.config }

// Destroy releases the context and its associated resources.
// Order matters: EGL resources first, then the native display connection.
// Closing the native display (e.g. XCloseDisplay) before eglTerminate
// would cause EGL to access a freed connection.
func (c *Context) Destroy() {
	if c.context != NoContext {
		// Unbind context first
		_ = MakeCurrent(c.display, NoSurface, NoSurface, NoContext)
		DestroyContext(c.display, c.context)
		c.context = NoContext
	}
	if c.pbuffer != NoSurface {
		DestroySurface(c.display, c.pbuffer)
		c.pbuffer = NoSurface
	}
	if c.display != NoDisplay {
		Terminate(c.display)
		c.display = NoDisplay
	}
	// Close native display connection AFTER eglTerminate.
	// For X11 this calls XCloseDisplay; for other platforms displayOwner is nil.
	if c.displayOwner != nil {
		c.displayOwner.Close()
		c.displayOwner = nil
	}
}

// EGLContext returns the EGL context handle.
func (c *Context) EGLContext() EGLContext {
	return c.context
}

// Pbuffer returns the pbuffer surface.
func (c *Context) Pbuffer() EGLSurface {
	return c.pbuffer
}

// WindowKind returns the detected window system type.
func (c *Context) WindowKind() WindowKind {
	return c.windowKind
}

// GetGLProcAddress returns the address of an OpenGL function.
// It uses eglGetProcAddress to load both core and extension functions.
// Returns unsafe.Pointer for compatibility with goffi-based GL context.
func GetGLProcAddress(name string) unsafe.Pointer {
	//nolint:govet // Converting uintptr (function address) to unsafe.Pointer is required for FFI
	return unsafe.Pointer(GetProcAddress(name))
}
