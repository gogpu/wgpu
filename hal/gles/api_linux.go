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

// vendorUnknown is the placeholder vendor name used when the actual GPU vendor
// cannot be determined (e.g., no surface available during adapter enumeration).
const vendorUnknown = "Unknown"

// Backend implements hal.Backend for OpenGL ES / OpenGL 3.3+ on Linux.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendGL
}

// CreateInstance creates a new OpenGL instance with an optional EGL context.
// Attempts to create an EGL context at instance level (Rust wgpu-hal egl.rs:846
// parity) for adapter enumeration without a surface. Uses surfaceless/pbuffer
// context — same role as Windows hidden 1×1 HWND (v0.28.6).
//
// On Wayland, this may fail (EGL needs wl_display*) — that's OK, CreateSurface
// provides the proper context later. On X11/headless, this succeeds.
func (Backend) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	if err := egl.Init(); err != nil {
		return nil, fmt.Errorf("gles: failed to initialize EGL: %w", err)
	}

	// Try to create instance-level EGL context (Rust wgpu-hal parity).
	// NativeDisplay=0 uses EGL_DEFAULT_DISPLAY or EGL_MESA_platform_surfaceless.
	// Desktop GL (GLES=false) works on X11/headless. On Wayland this fails
	// (expected — no wl_display* yet); CreateSurface Path B handles Wayland.
	config := egl.DefaultContextConfig()
	ctx, err := egl.NewContext(config)
	if err != nil {
		hal.Logger().Info("gles: instance context unavailable (expected on Wayland)", "err", err)
		return &Instance{}, nil
	}

	if err := ctx.MakeCurrent(); err != nil {
		ctx.Destroy()
		hal.Logger().Warn("gles: instance context MakeCurrent failed", "err", err)
		return &Instance{}, nil
	}

	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress); err != nil {
		ctx.Destroy()
		hal.Logger().Warn("gles: instance GL load failed", "err", err)
		return &Instance{}, nil
	}

	hal.Logger().Info("gles: instance created with EGL context",
		"version", glCtx.GetString(gl.VERSION),
		"renderer", glCtx.GetString(gl.RENDERER))

	return &Instance{eglCtx: ctx, glCtx: glCtx}, nil
}

// Instance implements hal.Instance for the OpenGL backend on Linux.
// eglCtx/glCtx are non-nil when an instance-level EGL context was created
// successfully (X11/headless). On Wayland they may be nil — CreateSurface
// provides the context when a window handle is available.
type Instance struct {
	eglCtx *egl.Context
	glCtx  *gl.Context
}

// CreateSurface creates an OpenGL surface from window handles.
// On Linux: displayHandle and windowHandle are platform-specific.
// For X11: displayHandle is X11 Display*, windowHandle is Window.
// For Wayland: displayHandle is wl_display*, windowHandle is wl_surface*.
//
// When Instance has a pre-created EGL context (X11/headless), the context is
// SHARED — Surface only creates an EGL window surface for presentation. This
// matches the Windows pattern where Instance owns AdapterContext and Surface
// is lightweight (just HWND + reference to shared ctx).
//
// When Instance has no context (Wayland — no wl_display* at init), CreateSurface
// creates a new EGL context with the caller's displayHandle.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (hal.Surface, error) {
	// Path A: share Instance context (X11 — context matches window system).
	// Do NOT share if Instance context is surfaceless (headless/Wayland fallback)
	// and Surface needs a window — the EGL display won't support eglCreateWindowSurface.
	if i.eglCtx != nil && i.glCtx != nil && i.eglCtx.WindowKind() != egl.WindowKindSurfaceless {
		hal.Logger().Info("gles: surface sharing Instance EGL context")
		return &Surface{
			displayHandle: displayHandle,
			windowHandle:  windowHandle,
			eglCtx:        i.eglCtx,
			glCtx:         i.glCtx,
			ownsContext:   false, // Instance owns context, Surface only references it
			version:       i.glCtx.GetString(gl.VERSION),
			renderer:      i.glCtx.GetString(gl.RENDERER),
		}, nil
	}

	// Path B: create new context (Wayland — Instance had no wl_display* at init).
	// GLES=true: Wayland Mesa EGL only exposes EGL_OPENGL_ES3_BIT configs, not
	// EGL_OPENGL_BIT. Desktop GL 3.3 request fails with "no suitable EGL configs".
	config := egl.ContextConfig{
		GLVersionMajor: 3,
		GLVersionMinor: 0,
		GLES:           true,
		NativeDisplay:  displayHandle,
	}
	ctx, err := egl.NewContext(config)
	if err != nil {
		return nil, fmt.Errorf("gles: failed to create EGL context: %w", err)
	}

	if err := ctx.MakeCurrent(); err != nil {
		ctx.Destroy()
		return nil, fmt.Errorf("gles: failed to make context current: %w", err)
	}

	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress); err != nil {
		ctx.Destroy()
		return nil, fmt.Errorf("gles: failed to load GL functions: %w", err)
	}

	version := glCtx.GetString(gl.VERSION)
	renderer := glCtx.GetString(gl.RENDERER)
	hal.Logger().Info("gles: surface created with new EGL context",
		"version", version, "renderer", renderer)

	return &Surface{
		displayHandle: displayHandle,
		windowHandle:  windowHandle,
		eglCtx:        ctx,
		glCtx:         glCtx,
		ownsContext:   true, // Surface owns this context (no Instance context)
		version:       version,
		renderer:      renderer,
	}, nil
}

// EnumerateAdapters returns available OpenGL adapters.
// Uses surface context (preferred), instance context (X11/headless), or placeholder.
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	// Priority 1: surface provides the best context (has window handle)
	if surface, ok := surfaceHint.(*Surface); ok {
		return []hal.ExposedAdapter{surface.GetAdapterInfo()}
	}

	// Priority 2: instance-level context (created in CreateInstance via pbuffer/surfaceless)
	if i.glCtx != nil {
		return []hal.ExposedAdapter{
			makeAdapterFromGL(i.glCtx, i.eglCtx),
		}
	}

	// Priority 3: no context available (Wayland without surface hint)
	// Return placeholder — Open() has nil guard from PR #210.
	return []hal.ExposedAdapter{
		{
			Adapter: &Adapter{},
			Info: gputypes.AdapterInfo{
				Name:       "OpenGL Adapter",
				Vendor:     vendorUnknown,
				DeviceType: gputypes.DeviceTypeOther,
				Driver:     "OpenGL",
				DriverInfo: "OpenGL 3.3+ / ES 3.0+ (no context — use RequestAdapterWithSurface)",
				Backend:    gputypes.BackendGL,
			},
			Capabilities: hal.Capabilities{
				Limits: gputypes.DefaultLimits(),
				AlignmentsMask: hal.Alignments{
					BufferCopyOffset: 4,
					BufferCopyPitch:  256,
				},
			},
		},
	}
}

// makeAdapterFromGL creates an ExposedAdapter using a live GL context.
func makeAdapterFromGL(glCtx *gl.Context, eglCtx *egl.Context) hal.ExposedAdapter {
	version := glCtx.GetString(gl.VERSION)
	renderer := glCtx.GetString(gl.RENDERER)
	vendor := glCtx.GetString(gl.VENDOR)

	return hal.ExposedAdapter{
		Adapter: &Adapter{
			glCtx:  glCtx,
			eglCtx: eglCtx,
		},
		Info: gputypes.AdapterInfo{
			Name:       renderer,
			Vendor:     vendor,
			DeviceType: gputypes.DeviceTypeIntegratedGPU,
			Driver:     "OpenGL",
			DriverInfo: version,
			Backend:    gputypes.BackendGL,
		},
		Capabilities: hal.Capabilities{
			Limits: gputypes.DefaultLimits(),
			AlignmentsMask: hal.Alignments{
				BufferCopyOffset: 4,
				BufferCopyPitch:  256,
			},
		},
	}
}

// Destroy releases the instance resources.
func (i *Instance) Destroy() {
	if i.eglCtx != nil {
		i.eglCtx.Destroy()
		i.eglCtx = nil
		i.glCtx = nil
	}
}
