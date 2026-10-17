// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"runtime"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

// lockedWorker runs functions on one goroutine locked to one OS thread.
type lockedWorker struct {
	work chan func()
	done chan struct{}
}

func newLockedWorker() *lockedWorker {
	w := &lockedWorker{work: make(chan func()), done: make(chan struct{})}
	started := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(w.done)
		close(started)
		for fn := range w.work {
			fn()
		}
	}()
	<-started
	return w
}

func (w *lockedWorker) run(fn func()) {
	finished := make(chan struct{})
	w.work <- func() { defer close(finished); fn() }
	<-finished
}

func (w *lockedWorker) stop() {
	close(w.work)
	<-w.done
}

// TestWGLInstanceLifecycle drives one GL instance through its whole life:
// create the instance and GL context on one OS thread, use it from the test
// goroutine, and destroy it on a third OS thread where the context has never
// been current.
//
// All WGL work in this package shares this single instance on purpose: one
// hidden window and one pixel-format setup per test process (Intel's ICD
// keeps per-thread state, see AdapterContext.Destroy).
//
// The final step is the Instance.Release scenario of the wglDeleteContext
// access violation: AdapterContext.Destroy used to call wglDeleteContext on
// whatever thread Release ran on, without making the context current there
// first, which crashes Intel's GL ICD when that thread predates the ICD load
// (the destroyer thread here is locked before the instance is created).
// Skips when no usable GL driver is present.
func TestWGLInstanceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}

	destroyer := newLockedWorker()
	defer destroyer.stop()
	creator := newLockedWorker()
	defer creator.stop()

	var (
		inst     *Instance
		adapters []hal.ExposedAdapter
		err      error
	)
	creator.run(func() {
		var raw hal.Instance
		raw, err = Backend{}.CreateInstance(nil)
		if err != nil {
			return
		}
		inst = raw.(*Instance)
		adapters = inst.EnumerateAdapters(nil)
	})
	if err != nil {
		t.Skipf("GL instance unavailable: %v", err)
	}
	defer destroyer.run(inst.Destroy)

	if len(adapters) == 0 {
		t.Skip("no GL adapter (GL context could not be created)")
	}
	adapter := adapters[0].Adapter.(*Adapter)
	if !adapter.caps.IsES && (adapter.caps.GLMajor < 3 || (adapter.caps.GLMajor == 3 && adapter.caps.GLMinor < 1)) {
		t.Skipf("GL %d.%d < 3.1: glCopyBufferSubData unavailable", adapter.caps.GLMajor, adapter.caps.GLMinor)
	}

	open, err := adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		t.Skipf("GL device unavailable: %v", err)
	}
	defer open.Device.Destroy()

	d := &glTestDevice{adapter: adapter, device: open.Device, queue: open.Queue}
	t.Run("CopyBufferToBuffer", func(t *testing.T) { testGLCopyBufferToBuffer(t, d) })
	t.Run("ComputePairSum", func(t *testing.T) { testGLComputePairSum(t, d) })
}
