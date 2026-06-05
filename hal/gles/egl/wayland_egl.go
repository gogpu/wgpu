// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

// Package egl provides EGL bindings for OpenGL context management.
//
// This file adds libwayland-egl.so bindings for creating EGL window surfaces
// on Wayland. Unlike X11 where eglCreateWindowSurface takes the raw X11
// Window handle, Wayland requires an intermediate wl_egl_window object.
//
// Enterprise reference: Rust wgpu-hal egl.rs:1390-1533 (configure),
// SDL3 SDL_waylandwindow.c:2978.
package egl

import (
	"log/slog"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	waylandEGLOnce  sync.Once
	waylandEGLReady bool

	waylandEGLLib unsafe.Pointer

	symWlEGLWindowCreate  unsafe.Pointer
	symWlEGLWindowDestroy unsafe.Pointer
	symWlEGLWindowResize  unsafe.Pointer

	// wl_egl_window* wl_egl_window_create(wl_surface*, int width, int height)
	cifWlEGLWindowCreate types.CallInterface
	// void wl_egl_window_destroy(wl_egl_window*)
	cifWlEGLWindowDestroy types.CallInterface
	// void wl_egl_window_resize(wl_egl_window*, int width, int height, int dx, int dy)
	cifWlEGLWindowResize types.CallInterface
)

// InitWaylandEGL loads libwayland-egl.so and prepares CIFs.
// Safe to call multiple times; only the first call does work.
func InitWaylandEGL() bool {
	waylandEGLOnce.Do(func() {
		var err error

		waylandEGLLib, err = ffi.LoadLibrary("libwayland-egl.so.1")
		if err != nil {
			waylandEGLLib, err = ffi.LoadLibrary("libwayland-egl.so")
			if err != nil {
				slog.Debug("egl: libwayland-egl unavailable", "error", err)
				return
			}
		}

		symWlEGLWindowCreate, err = ffi.GetSymbol(waylandEGLLib, "wl_egl_window_create")
		if err != nil {
			slog.Debug("egl: wl_egl_window_create not found", "error", err)
			return
		}
		symWlEGLWindowDestroy, err = ffi.GetSymbol(waylandEGLLib, "wl_egl_window_destroy")
		if err != nil {
			slog.Debug("egl: wl_egl_window_destroy not found", "error", err)
			return
		}
		symWlEGLWindowResize, err = ffi.GetSymbol(waylandEGLLib, "wl_egl_window_resize")
		if err != nil {
			slog.Debug("egl: wl_egl_window_resize not found", "error", err)
			return
		}

		// wl_egl_window* wl_egl_window_create(wl_surface*, int width, int height)
		if err = ffi.PrepareCallInterface(&cifWlEGLWindowCreate, types.DefaultCall,
			types.PointerTypeDescriptor,
			[]*types.TypeDescriptor{
				types.PointerTypeDescriptor, // wl_surface*
				types.SInt32TypeDescriptor,  // width
				types.SInt32TypeDescriptor,  // height
			}); err != nil {
			slog.Debug("egl: failed to prepare wl_egl_window_create CIF", "error", err)
			return
		}

		// void wl_egl_window_destroy(wl_egl_window*)
		if err = ffi.PrepareCallInterface(&cifWlEGLWindowDestroy, types.DefaultCall,
			types.VoidTypeDescriptor,
			[]*types.TypeDescriptor{
				types.PointerTypeDescriptor,
			}); err != nil {
			slog.Debug("egl: failed to prepare wl_egl_window_destroy CIF", "error", err)
			return
		}

		// void wl_egl_window_resize(wl_egl_window*, int width, int height, int dx, int dy)
		if err = ffi.PrepareCallInterface(&cifWlEGLWindowResize, types.DefaultCall,
			types.VoidTypeDescriptor,
			[]*types.TypeDescriptor{
				types.PointerTypeDescriptor, // wl_egl_window*
				types.SInt32TypeDescriptor,  // width
				types.SInt32TypeDescriptor,  // height
				types.SInt32TypeDescriptor,  // dx
				types.SInt32TypeDescriptor,  // dy
			}); err != nil {
			slog.Debug("egl: failed to prepare wl_egl_window_resize CIF", "error", err)
			return
		}

		waylandEGLReady = true
		slog.Debug("egl: libwayland-egl loaded — Wayland EGL window support available")
	})
	return waylandEGLReady
}

// HasWaylandEGL reports whether libwayland-egl.so was loaded successfully.
func HasWaylandEGL() bool {
	return waylandEGLReady
}

// WlEGLWindowCreate creates a wl_egl_window from a wl_surface.
// Returns 0 on failure.
func WlEGLWindowCreate(wlSurface uintptr, width, height int32) uintptr {
	if !waylandEGLReady {
		return 0
	}
	var result uintptr
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&wlSurface),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
	}
	_ = ffi.CallFunction(&cifWlEGLWindowCreate, symWlEGLWindowCreate, unsafe.Pointer(&result), args[:])
	return result
}

// WlEGLWindowDestroy destroys a wl_egl_window.
func WlEGLWindowDestroy(eglWindow uintptr) {
	if !waylandEGLReady || eglWindow == 0 {
		return
	}
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&eglWindow),
	}
	_ = ffi.CallFunction(&cifWlEGLWindowDestroy, symWlEGLWindowDestroy, nil, args[:])
}

// WlEGLWindowResize resizes a wl_egl_window.
func WlEGLWindowResize(eglWindow uintptr, width, height, dx, dy int32) {
	if !waylandEGLReady || eglWindow == 0 {
		return
	}
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&eglWindow),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&dx),
		unsafe.Pointer(&dy),
	}
	_ = ffi.CallFunction(&cifWlEGLWindowResize, symWlEGLWindowResize, nil, args[:])
}
