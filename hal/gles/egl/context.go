// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package egl

import (
	"fmt"
	"unsafe"
)

// Context wraps an EGL rendering context with its display, config, and surface.
type Context struct {
	display    EGLDisplay
	config     EGLConfig
	context    EGLContext
	pbuffer    EGLSurface
	windowKind WindowKind
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
	// Get EGL display for the detected platform
	display, windowKind, err := GetEGLDisplay()
	if err != nil {
		return nil, fmt.Errorf("failed to get EGL display: %w", err)
	}

	// Initialize EGL
	var major, minor EGLInt
	if Initialize(display, &major, &minor) == False {
		return nil, fmt.Errorf("eglInitialize failed: error 0x%x", GetError())
	}

	// Bind OpenGL or OpenGL ES API
	api := OpenGLAPI
	if config.GLES {
		api = OpenGLESAPI
	}
	if BindAPI(api) == False {
		Terminate(display)
		return nil, fmt.Errorf("eglBindAPI failed: error 0x%x", GetError())
	}

	// Choose EGL frame buffer configuration
	eglConfig, err := chooseEGLConfig(display, config)
	if err != nil {
		Terminate(display)
		return nil, fmt.Errorf("failed to choose EGL config: %w", err)
	}

	// Create EGL context
	eglContext := createEGLContext(display, eglConfig, config)
	if eglContext == NoContext {
		Terminate(display)
		return nil, fmt.Errorf("eglCreateContext failed: error 0x%x", GetError())
	}

	// Create a pbuffer surface for the context
	// This is needed even for surfaceless rendering on some drivers
	pbuffer := createPbufferSurface(display, eglConfig)
	if pbuffer == NoSurface {
		DestroyContext(display, eglContext)
		Terminate(display)
		return nil, fmt.Errorf("eglCreatePbufferSurface failed: error 0x%x", GetError())
	}

	return &Context{
		display:    display,
		config:     eglConfig,
		context:    eglContext,
		pbuffer:    pbuffer,
		windowKind: windowKind,
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

	// Build attribute list
	attribs := []EGLInt{
		SurfaceType, PbufferBit,
		RenderableType, renderableType,
		RedSize, 8,
		GreenSize, 8,
		BlueSize, 8,
		AlphaSize, 8,
		DepthSize, 24,
		StencilSize, 8,
		None,
	}

	// Choose config
	var eglConfig EGLConfig
	var numConfigs EGLInt
	if ChooseConfig(display, &attribs[0], &eglConfig, 1, &numConfigs) == False {
		return 0, fmt.Errorf("eglChooseConfig failed: error 0x%x", GetError())
	}

	if numConfigs == 0 {
		return 0, fmt.Errorf("no suitable EGL configs found")
	}

	return eglConfig, nil
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

// MakeCurrent makes this context current for the calling thread.
func (c *Context) MakeCurrent() error {
	if MakeCurrent(c.display, c.pbuffer, c.pbuffer, c.context) == False {
		return fmt.Errorf("eglMakeCurrent failed: error 0x%x", GetError())
	}
	return nil
}

// Destroy releases the context and its associated resources.
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
}

// Display returns the EGL display.
func (c *Context) Display() EGLDisplay {
	return c.display
}

// Config returns the EGL config.
func (c *Context) Config() EGLConfig {
	return c.config
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
