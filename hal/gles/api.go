// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// Backend implements hal.Backend for OpenGL ES / OpenGL 3.3+.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendGL
}

// CreateInstance creates a new OpenGL instance.
func (Backend) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	// Initialize WGL on Windows
	if err := wgl.Init(); err != nil {
		return nil, fmt.Errorf("gles: failed to initialize WGL: %w", err)
	}
	hal.Logger().Info("gles: instance created", "platform", "windows")
	return &Instance{}, nil
}

// Instance implements hal.Instance for the OpenGL backend.
type Instance struct{}

// CreateSurface creates an OpenGL surface from window handles.
// On Windows: displayHandle is ignored, windowHandle is HWND.
func (i *Instance) CreateSurface(_, windowHandle uintptr) (hal.Surface, error) {
	// Create WGL context for the window
	ctx, err := wgl.NewContext(wgl.HWND(windowHandle))
	if err != nil {
		return nil, fmt.Errorf("gles: failed to create WGL context: %w", err)
	}

	// Make it current to load GL functions
	if err := ctx.MakeCurrent(); err != nil {
		ctx.Destroy(wgl.HWND(windowHandle))
		return nil, fmt.Errorf("gles: failed to make context current: %w", err)
	}

	// Load GL function pointers
	glCtx := &gl.Context{}
	if err := glCtx.Load(wgl.GetGLProcAddress); err != nil {
		ctx.Destroy(wgl.HWND(windowHandle))
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
		hwnd:     wgl.HWND(windowHandle),
		wglCtx:   ctx,
		glCtx:    glCtx,
		version:  version,
		renderer: renderer,
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
