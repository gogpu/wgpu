// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/internal/eglrefs"
)

// ErrContextReleased is returned when an EGL context is used after the last
// Surface and Device built on it were released: the context, and every GL
// object in it, is gone. A cached Adapter of a closed window returns it from
// Open; request an adapter with the new surface instead. It is
// hal.ErrContextReleased (wgpu.ErrContextReleased in the public package),
// which wraps hal.ErrDeviceLost.
var ErrContextReleased = hal.ErrContextReleased

// AdapterContext wraps an EGL/GL context with mutex-protected MakeCurrent
// switching, and counts the objects that keep the context alive (ADR-077 B).
//
// A context lives as long as its strong holders: the Surface (or the
// Instance, for its own context) that created it, every Surface that shares
// it, and every Device opened on it (the Queue borrows the Device's). Adapters
// and the Instance's table of surface-created contexts hold it weakly and
// take a temporary reference to use it. The last Release destroys the
// EGLContext while the native display under it is still open, which the
// documented surface order guarantees; tying it to the Instance instead let
// eglTerminate run after the app had closed its display (#379).
//
// Two locks with separate jobs:
//   - mu is held while the context is current on a thread: Lock or
//     LockForSurface until Unlock (Rust wgpu-hal AdapterContext::lock).
//   - refs has its own lock for the count and the dead mark, never held across
//     a GL call, so bookkeeping never waits behind GL work.
//
// gl and eglCtx are set at construction and never change; destruction only
// marks the context dead. GL() and EGL() therefore take no lock, which code
// under Lock relies on (CreateRenderPipeline → compileFragmentShader).
type AdapterContext struct {
	mu     sync.Mutex
	refs   eglrefs.Refs
	eglCtx *egl.Context
	gl     *gl.Context
}

// NewAdapterContext wraps an already-created EGL context and GL function
// table. The caller holds the first reference and gives it up with Release.
func NewAdapterContext(eglCtx *egl.Context, glCtx *gl.Context) *AdapterContext {
	return &AdapterContext{
		eglCtx: eglCtx,
		gl:     glCtx,
	}
}

// Acquire takes one more reference to the context and reports whether it
// did. It fails once the last reference was released: the context never
// comes back.
func (c *AdapterContext) Acquire() bool {
	return c.refs.Acquire()
}

// Release gives up one reference. The last one destroys the context:
//  1. the dead mark is set under the refs lock, in the same critical section
//     as the decrement, so no Acquire or Lock succeeds after it;
//  2. mu is taken, which waits out a holder that made the context current
//     before the mark;
//  3. the EGLContext is destroyed and its display released on a pinned OS
//     thread, under mu but outside the refs lock. A Lock that comes in
//     meanwhile waits, then fails on the dead mark. mu is only ever taken
//     before the display registry's lock, never after it, so this cannot
//     deadlock.
//
// Release must not be called while the same goroutine holds Lock.
func (c *AdapterContext) Release() {
	last, err := c.refs.Release()
	if err != nil {
		slog.Error("gles: AdapterContext released more often than acquired", "err", err)
		return
	}
	if !last {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.destroy()
}

// Alive reports whether a reference to the context is still held.
func (c *AdapterContext) Alive() bool {
	return c.refs.Alive()
}

// destroy deletes the EGL context. egl.Context.Destroy pins its own OS thread.
func (c *AdapterContext) destroy() {
	if c.eglCtx != nil {
		c.eglCtx.Destroy()
	}
}

// RobustAccess reports whether the EGL context was created with robust buffer
// access.
func (c *AdapterContext) RobustAccess() bool {
	return c.eglCtx != nil && c.eglCtx.RobustAccess()
}

// Lock acquires the mutex, pins the goroutine to the current OS thread, and
// makes the GL context current on the pbuffer / surfaceless draw surface.
//
// On a released context it returns ErrContextReleased with nothing held: the
// caller must not call Unlock. Every caller follows one convention:
//
//	glCtx, err := c.Lock()
//	if err != nil {
//		return …
//	}
//	defer c.Unlock()
//
// Mirrors Windows AdapterContext.Lock() (hidden DC) and Rust AdapterContext::lock().
func (c *AdapterContext) Lock() (*gl.Context, error) {
	if err := c.lockAlive(); err != nil {
		return nil, err
	}
	if c.eglCtx == nil {
		slog.Error("gles: AdapterContext.Lock: nil egl context")
		return c.gl, nil
	}
	if err := c.eglCtx.MakeCurrent(); err != nil {
		slog.Error("gles: AdapterContext.Lock MakeCurrent failed", "err", err)
	}
	return c.gl, nil
}

// LockForSurface acquires the mutex, pins the goroutine to the current OS thread,
// and makes the GL context current on the given window EGLSurface. Its error
// convention is Lock's.
//
// Mirrors Windows AdapterContext.LockForDC() and Rust lock_with_dc / egl present path.
func (c *AdapterContext) LockForSurface(surf egl.EGLSurface) (*gl.Context, error) {
	if err := c.lockAlive(); err != nil {
		return nil, err
	}
	if c.eglCtx == nil {
		slog.Error("gles: AdapterContext.LockForSurface: nil egl context")
		return c.gl, nil
	}
	if err := c.eglCtx.MakeCurrentSurface(surf); err != nil {
		slog.Error("gles: AdapterContext.LockForSurface MakeCurrent failed",
			"err", err,
			"surface", fmt.Sprintf("0x%x", surf))
	}
	return c.gl, nil
}

// lockAlive takes mu and pins the OS thread when the context is alive. The
// alive check runs under mu, after any final Release's dead mark it must see:
// a Release that marked the context dead waits for mu before it destroys.
func (c *AdapterContext) lockAlive() error {
	c.mu.Lock()
	if !c.refs.Alive() {
		c.mu.Unlock()
		return ErrContextReleased
	}
	runtime.LockOSThread()
	return nil
}

// Unlock unmakes the GL context current, unpins the goroutine from the OS
// thread, and releases the mutex. Only after a Lock that returned no error.
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

// GL returns the GL function table. It takes no lock; GL calls on the
// returned context require Lock.
func (c *AdapterContext) GL() *gl.Context {
	return c.gl
}

// EGL returns the underlying EGL context wrapper. It takes no lock.
func (c *AdapterContext) EGL() *egl.Context {
	return c.eglCtx
}
