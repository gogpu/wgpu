// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

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
//
// Creates a hidden 1×1 window and initializes a GL context on it. The context
// lives for the Instance lifetime and survives any user Surface destruction.
// Follows Rust wgpu-hal/src/gles/wgl.rs Instance::init (lines 448-563).
func (Backend) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	if err := wgl.Init(); err != nil {
		return nil, fmt.Errorf("gles: failed to initialize WGL: %w", err)
	}

	// Create hidden 1×1 window to host the GL context (Rust: create_instance_device).
	// GL context is NOT created here — it will be lazily created on the render
	// thread's first Lock() call, avoiding cross-thread WGL issues.
	hiddenWindow, err := wgl.NewHiddenWindow()
	if err != nil {
		return nil, fmt.Errorf("gles: failed to create hidden window: %w", err)
	}

	// Set pixel format on hidden window DC. This is thread-safe and must be
	// done before wglCreateContext (which happens lazily in AdapterContext).
	pfd := wgl.DefaultPixelFormat()
	format, err := wgl.ChoosePixelFormat(hiddenWindow.DC(), &pfd)
	if err != nil {
		hiddenWindow.Destroy()
		return nil, fmt.Errorf("gles: hidden window ChoosePixelFormat: %w", err)
	}
	if err := wgl.SetPixelFormat(hiddenWindow.DC(), format, &pfd); err != nil {
		hiddenWindow.Destroy()
		return nil, fmt.Errorf("gles: hidden window SetPixelFormat: %w", err)
	}

	ctx := NewAdapterContext(hiddenWindow.DC())

	hal.Logger().Info("gles: instance created",
		"platform", "windows",
		"hiddenDC", fmt.Sprintf("0x%x", hiddenWindow.DC()),
	)

	return &Instance{
		ctx:          ctx,
		hiddenWindow: hiddenWindow,
	}, nil
}

// Instance implements hal.Instance for the OpenGL backend.
// Owns the hidden 1×1 window. The GL context is lazily created on the
// render thread via AdapterContext.ensureInit().
type Instance struct {
	ctx          *AdapterContext
	hiddenWindow *wgl.HiddenWindow
}

// CreateSurface creates an OpenGL surface from window handles.
// On Windows: displayHandle is ignored, windowHandle is HWND.
//
// The Surface is lightweight — it does NOT own the GL context. The context
// lives on the hidden window owned by Instance. Surface only stores the user
// HWND and a reference to the shared AdapterContext. SetPixelFormat is called
// on the user window DC so that wglMakeCurrent can switch between hidden and
// user DCs during Present.
//
// Follows Rust wgpu-hal/src/gles/wgl.rs Instance::create_surface (lines 624-670).
func (i *Instance) CreateSurface(_, windowHandle uintptr) (hal.Surface, error) {
	hwnd := wgl.HWND(windowHandle)

	// Set pixel format on user window DC. Required for wglMakeCurrent to
	// accept this DC with the hidden window's HGLRC — pixel formats must match.
	hdc := wgl.GetDC(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("gles: GetDC failed for window 0x%x", windowHandle)
	}
	pfd := wgl.DefaultPixelFormat()
	format, err := wgl.ChoosePixelFormat(hdc, &pfd)
	if err != nil {
		wgl.ReleaseDC(hwnd, hdc)
		return nil, fmt.Errorf("gles: surface ChoosePixelFormat: %w", err)
	}
	if err := wgl.SetPixelFormat(hdc, format, &pfd); err != nil {
		wgl.ReleaseDC(hwnd, hdc)
		return nil, fmt.Errorf("gles: surface SetPixelFormat: %w", err)
	}
	wgl.ReleaseDC(hwnd, hdc)

	hal.Logger().Info("gles: surface created", "hwnd", fmt.Sprintf("0x%x", windowHandle))

	return &Surface{
		hwnd: hwnd,
		ctx:  i.ctx,
	}, nil
}

// EnumerateAdapters returns available OpenGL adapters.
// Triggers lazy GL context creation on the calling thread (render thread).
func (i *Instance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	glCtx := i.ctx.Lock()
	defer i.ctx.Unlock()

	if glCtx == nil {
		hal.Logger().Error("gles: EnumerateAdapters: GL context not available")
		return nil
	}

	caps := queryAdapterCapabilities(glCtx)

	version := glCtx.GetString(gl.VERSION)
	renderer := glCtx.GetString(gl.RENDERER)

	driverInfo := "OpenGL 3.3+"
	if caps.IsES {
		driverInfo = fmt.Sprintf("OpenGL ES %d.%d", caps.GLMajor, caps.GLMinor)
	} else if caps.GLMajor > 0 {
		driverInfo = fmt.Sprintf("OpenGL %d.%d", caps.GLMajor, caps.GLMinor)
	}

	return []hal.ExposedAdapter{
		{
			Adapter: &Adapter{
				ctx:      i.ctx,
				version:  version,
				renderer: renderer,
				caps:     caps,
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
		},
	}
}

// Destroy releases the instance resources.
func (i *Instance) Destroy() {
	if i.ctx != nil {
		i.ctx.Destroy()
	}
	if i.hiddenWindow != nil {
		i.hiddenWindow.Destroy()
		i.hiddenWindow = nil
	}
}
