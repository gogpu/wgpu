// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// AdapterContext wraps a GL context with mutex-protected MakeCurrent switching.
// Shared by Instance → Adapter → Device → Queue (all hold *AdapterContext).
//
// The GL context is lazily created on the first Lock() call, ensuring it is
// born on the OS thread that will use it (the render thread). This avoids
// cross-thread WGL issues: creating the context on the main thread and using
// it on the render thread causes wglMakeCurrent failures because WGL binds
// the context to the creating thread.
//
// Follows Rust wgpu-hal/src/gles/wgl.rs AdapterContext (lines 40-94):
//   - lock()         → MakeCurrent to hidden DC
//   - lock_with_dc() → MakeCurrent to user DC
//   - Drop           → UnmakeCurrent
type AdapterContext struct {
	mu       sync.Mutex
	gl       *gl.Context
	hglrc    wgl.HGLRC
	hiddenDC wgl.HDC

	initOnce sync.Once
	initErr  error
}

// NewAdapterContext creates an AdapterContext with a hidden window DC.
// The GL context is NOT created here — it will be created lazily on the
// first Lock() call, on whatever OS thread that call runs on.
func NewAdapterContext(hiddenDC wgl.HDC) *AdapterContext {
	return &AdapterContext{
		hiddenDC: hiddenDC,
	}
}

// ensureInit creates the GL context and loads GL functions on first call.
// Must be called with the mutex held and LockOSThread active.
func (c *AdapterContext) ensureInit() error {
	c.initOnce.Do(func() {
		hglrc, err := wgl.CreateContext(c.hiddenDC)
		if err != nil {
			c.initErr = fmt.Errorf("wglCreateContext: %w", err)
			return
		}
		if err := wgl.MakeCurrent(c.hiddenDC, hglrc); err != nil {
			_ = wgl.DeleteContext(hglrc)
			c.initErr = fmt.Errorf("wglMakeCurrent: %w", err)
			return
		}

		glCtx := &gl.Context{}
		if err := glCtx.Load(wgl.GetGLProcAddress); err != nil {
			_ = wgl.MakeCurrent(0, 0)
			_ = wgl.DeleteContext(hglrc)
			c.initErr = fmt.Errorf("GL load: %w", err)
			return
		}

		c.hglrc = hglrc
		c.gl = glCtx

		slog.Info("gles: GL context created on render thread",
			"hiddenDC", fmt.Sprintf("0x%x", c.hiddenDC),
			"hglrc", fmt.Sprintf("0x%x", hglrc),
			"version", glCtx.GetString(gl.VERSION),
			"renderer", glCtx.GetString(gl.RENDERER))
	})
	return c.initErr
}

// Lock acquires the mutex, pins the goroutine to the current OS thread, and
// makes the GL context current on the hidden window DC.
//
// On first call, lazily creates the GL context on the calling OS thread.
// This ensures the context is born on the render thread — no cross-thread
// WGL issues.
//
// Mirrors Rust AdapterContext::lock() (wgl.rs:62-80).
func (c *AdapterContext) Lock() *gl.Context {
	c.mu.Lock()
	runtime.LockOSThread()

	if err := c.ensureInit(); err != nil {
		slog.Error("gles: AdapterContext.Lock init failed", "err", err)
		return c.gl
	}

	if err := wgl.MakeCurrent(c.hiddenDC, c.hglrc); err != nil {
		slog.Error("gles: AdapterContext.Lock MakeCurrent failed",
			"err", err,
			"hiddenDC", fmt.Sprintf("0x%x", c.hiddenDC),
			"hglrc", fmt.Sprintf("0x%x", c.hglrc))
	}
	return c.gl
}

// LockForDC acquires the mutex, pins the goroutine to the current OS thread,
// and makes the GL context current on the specified device context.
//
// Mirrors Rust AdapterContext::lock_with_dc() (wgl.rs:83-94).
func (c *AdapterContext) LockForDC(hdc wgl.HDC) *gl.Context {
	c.mu.Lock()
	runtime.LockOSThread()

	if err := c.ensureInit(); err != nil {
		slog.Error("gles: AdapterContext.LockForDC init failed", "err", err)
		return c.gl
	}

	if err := wgl.MakeCurrent(hdc, c.hglrc); err != nil {
		slog.Error("gles: AdapterContext.LockForDC MakeCurrent failed",
			"err", err,
			"hdc", fmt.Sprintf("0x%x", hdc),
			"hglrc", fmt.Sprintf("0x%x", c.hglrc))
	}
	return c.gl
}

// Unlock unmakes the GL context current, unpins the goroutine from the OS
// thread, and releases the mutex.
//
// Guards against double-unmake: only calls wglMakeCurrent(0, 0) if a
// context is actually current on this thread. Matches Rust wgpu-hal
// WglContext::unmake_current() which checks wglGetCurrentContext().is_invalid()
// before unmaking (wgl.rs:128-133).
func (c *AdapterContext) Unlock() {
	if wgl.GetCurrentContext() != 0 {
		if err := wgl.MakeCurrent(0, 0); err != nil {
			slog.Error("gles: AdapterContext.Unlock UnmakeCurrent failed", "err", err)
		}
	}
	runtime.UnlockOSThread()
	c.mu.Unlock()
}

// GL returns the GL function table. Must be called while locked.
func (c *AdapterContext) GL() *gl.Context {
	return c.gl
}

// HGLRC returns the GL rendering context handle. Zero if not yet initialized.
func (c *AdapterContext) HGLRC() wgl.HGLRC {
	return c.hglrc
}

// Destroy deletes the GL context if it was created.
func (c *AdapterContext) Destroy() {
	if c.hglrc != 0 {
		_ = wgl.MakeCurrent(0, 0)
		_ = wgl.DeleteContext(c.hglrc)
		c.hglrc = 0
	}
}
