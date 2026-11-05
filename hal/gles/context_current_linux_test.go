// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"runtime"
	"testing"

	"github.com/gogpu/wgpu/hal/gles/egl"
)

// TestInstanceContextNotLeftCurrent checks that CreateInstance unbinds the
// EGL context it creates. EGL lets a context be current on one thread only:
// left current on the creating thread, it made a Lock on another OS thread
// fail with EGL_BAD_ACCESS while the creating thread lived on. Rust wgpu-hal
// pairs every make_current with an unmake_current (src/gles/egl.rs).
func TestInstanceContextNotLeftCurrent(t *testing.T) {
	type created struct {
		inst        *Instance
		leftCurrent bool
	}
	result := make(chan created)
	release := make(chan struct{})
	go func() {
		// The creating thread stays alive, and locked, until the check on
		// the other thread is done.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var c created
		if i, err := (Backend{}).CreateInstance(nil); err == nil {
			c.inst = i.(*Instance)
			if ctx := c.inst.context(); ctx != nil && ctx.EGL() != nil {
				c.leftCurrent = egl.GetCurrentContext() == ctx.EGL().EGLContext()
			}
		}
		result <- c
		<-release
	}()
	c := <-result
	defer close(release)
	if c.inst == nil || c.inst.context() == nil || c.inst.context().EGL() == nil {
		t.Skip("no EGL instance context (no display or driver)")
	}
	defer c.inst.Destroy()
	if c.leftCurrent {
		t.Error("CreateInstance left its EGL context current on the creating thread")
	}

	ctx := c.inst.context()
	current := make(chan bool)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if _, err := ctx.Lock(); err != nil {
			t.Errorf("Lock: %v", err)
			current <- false
			return
		}
		ok := egl.GetCurrentContext() == ctx.EGL().EGLContext()
		ctx.Unlock()
		current <- ok
	}()
	if !<-current {
		t.Error("Lock on another OS thread could not make the instance context current")
	}
}
