// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// AdapterContext wraps an EGL/GL context with mutex-protected MakeCurrent switching.
// Shared by Instance → Adapter → Device → Queue (X11/headless), or owned by Surface
// on Wayland (intentional context-per-surface divergence — no wl_display* at Instance init).
//
// Follows Windows AdapterContext (adapter_context.go) and Rust wgpu-hal GLES lock pattern:
//   - Lock()             → MakeCurrent to pbuffer/surfaceless (resource + command work)
//   - LockForSurface()   → MakeCurrent to window EGLSurface (present / swap)
//   - Unlock()           → UnmakeCurrent
type AdapterContext struct {
	mu     sync.Mutex
	eglCtx *egl.Context
	gl     *gl.Context
	owns   bool // true → Destroy() destroys eglCtx
}

// NewAdapterContext wraps an already-created EGL context and GL function table.
// owns=true means Destroy() will call eglCtx.Destroy(); owns=false for borrowed refs.
func NewAdapterContext(eglCtx *egl.Context, glCtx *gl.Context, owns bool) *AdapterContext {
	return &AdapterContext{
		eglCtx: eglCtx,
		gl:     glCtx,
		owns:   owns,
	}
}

// Lock acquires the mutex, pins the goroutine to the current OS thread, and
// makes the GL context current on the pbuffer / surfaceless draw surface.
//
// Mirrors Windows AdapterContext.Lock() (hidden DC) and Rust AdapterContext::lock().
func (c *AdapterContext) Lock() *gl.Context {
	c.mu.Lock()
	runtime.LockOSThread()

	if c.eglCtx == nil {
		slog.Error("gles: AdapterContext.Lock: nil egl context")
		return c.gl
	}
	if err := c.eglCtx.MakeCurrent(); err != nil {
		slog.Error("gles: AdapterContext.Lock MakeCurrent failed", "err", err)
	}
	return c.gl
}

// LockForSurface acquires the mutex, pins the goroutine to the current OS thread,
// and makes the GL context current on the given window EGLSurface.
//
// Mirrors Windows AdapterContext.LockForDC() and Rust lock_with_dc / egl present path.
func (c *AdapterContext) LockForSurface(surf egl.EGLSurface) *gl.Context {
	c.mu.Lock()
	runtime.LockOSThread()

	if c.eglCtx == nil {
		slog.Error("gles: AdapterContext.LockForSurface: nil egl context")
		return c.gl
	}
	if err := c.eglCtx.MakeCurrentSurface(surf); err != nil {
		slog.Error("gles: AdapterContext.LockForSurface MakeCurrent failed",
			"err", err,
			"surface", fmt.Sprintf("0x%x", surf))
	}
	return c.gl
}

// Unlock unmakes the GL context current, unpins the goroutine from the OS
// thread, and releases the mutex.
//
// Guards against double-unmake: only calls eglMakeCurrent(NO_SURFACE, NO_CONTEXT)
// if a context is actually current on this thread. Matches Windows Unlock() and
// Rust WglContext::unmake_current().
func (c *AdapterContext) Unlock() {
	// Check eglCtx first so Unlock is safe before egl.Init (unit tests, teardown).
	if c.eglCtx != nil && egl.GetCurrentContext() != egl.NoContext {
		if egl.MakeCurrent(c.eglCtx.Display(), egl.NoSurface, egl.NoSurface, egl.NoContext) == egl.False {
			slog.Error("gles: AdapterContext.Unlock UnmakeCurrent failed",
				"err", fmt.Sprintf("0x%x", egl.GetError()))
		}
	}
	runtime.UnlockOSThread()
	c.mu.Unlock()
}

// GL returns the GL function table. Must be called while locked (or after init).
func (c *AdapterContext) GL() *gl.Context {
	return c.gl
}

// EGL returns the underlying EGL context wrapper.
func (c *AdapterContext) EGL() *egl.Context {
	return c.eglCtx
}

// Destroy deletes the EGL context when this AdapterContext owns it.
// Takes the mutex so Destroy cannot race with Lock/Unlock on another goroutine.
func (c *AdapterContext) Destroy() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owns && c.eglCtx != nil {
		c.eglCtx.Destroy()
		c.eglCtx = nil
		c.gl = nil
	}
}
