// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// Backend implements hal.Backend for OpenGL ES / OpenGL 3.3+ on Linux.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendGL
}

// CreateInstance creates a new OpenGL instance.
func (Backend) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	// Initialize EGL on Linux
	if err := egl.Init(); err != nil {
		return nil, fmt.Errorf("gles: failed to initialize EGL: %w", err)
	}
	hal.Logger().Info("gles: instance created", "platform", "linux")
	return &Instance{}, nil
}

// Instance implements hal.Instance for the OpenGL backend on Linux.
type Instance struct{}

// CreateSurface creates an OpenGL surface from window handles.
// On Linux: displayHandle and windowHandle are platform-specific.
// For X11: displayHandle is X11 Display*, windowHandle is Window.
// For Wayland: displayHandle is wl_display*, windowHandle is wl_surface*.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (hal.Surface, error) {
	// Create EGL context with automatic platform detection
	config := egl.DefaultContextConfig()
	config.GLES = false // Use desktop OpenGL
	ctx, err := egl.NewContext(config)
	if err != nil {
		return nil, fmt.Errorf("gles: failed to create EGL context: %w", err)
	}

	// Make it current to load GL functions
	if err := ctx.MakeCurrent(); err != nil {
		ctx.Destroy()
		return nil, fmt.Errorf("gles: failed to make context current: %w", err)
	}

	// Load GL function pointers
	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress); err != nil {
		ctx.Destroy()
		return nil, fmt.Errorf("gles: failed to load GL functions: %w", err)
	}

	// Query OpenGL version
	version := glCtx.GetString(gl.VERSION)
	renderer := glCtx.GetString(gl.RENDERER)

	hal.Logger().Info("gles: surface created",
		"version", version,
		"renderer", renderer,
	)

	return &Surface{
		displayHandle: displayHandle,
		windowHandle:  windowHandle,
		eglCtx:        ctx,
		glCtx:         glCtx,
		version:       version,
		renderer:      renderer,
	}, nil
}

// EnumerateAdapters returns available OpenGL adapters.
// For OpenGL, there's typically one adapter per display.
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	// If we have a surface, use its GL context for info
	if surface, ok := surfaceHint.(*Surface); ok {
		return []hal.ExposedAdapter{
			surface.GetAdapterInfo(),
		}
	}

	// Without a surface, we can't query OpenGL info
	// Return a placeholder that will be updated when surface is created
	return []hal.ExposedAdapter{
		{
			Adapter: &Adapter{},
			Info: gputypes.AdapterInfo{
				Name:       "OpenGL Adapter",
				Vendor:     "Unknown",
				VendorID:   0,
				DeviceID:   0,
				DeviceType: gputypes.DeviceTypeOther,
				Driver:     "OpenGL",
				DriverInfo: "OpenGL 3.3+ / ES 3.0+",
				Backend:    gputypes.BackendGL,
			},
			Features: 0,
			Capabilities: hal.Capabilities{
				Limits: gputypes.DefaultLimits(),
				AlignmentsMask: hal.Alignments{
					BufferCopyOffset: 4,
					BufferCopyPitch:  256,
				},
				DownlevelCapabilities: hal.DownlevelCapabilities{
					ShaderModel: 50, // SM5.0
					Flags:       0,
				},
			},
		},
	}
}

// Destroy releases the instance resources.
func (i *Instance) Destroy() {
	// Nothing to clean up at instance level
}
