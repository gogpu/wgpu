// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// These tests use AdapterContexts without an EGL context (nil egl.Context),
// so they run without libEGL; the real-EGL paths are in lifetime_repro_test.go.

// The GLES adapter qualifies cached adapters for a new surface (ADR-077), the
// optional core capability Vulkan also implements.
var _ interface {
	QualifySurface(surface hal.Surface) (hal.Adapter, error)
} = (*Adapter)(nil)

func TestAdapterContextDiesWithLastReference(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{}) // the creator's reference
	if !ctx.Acquire() {
		t.Fatal("Acquire on a live context failed")
	}
	ctx.Release()
	if !ctx.Alive() {
		t.Fatal("context died while a reference remains")
	}
	ctx.Release()
	if ctx.Alive() {
		t.Fatal("context alive after its last reference")
	}
	if ctx.Acquire() {
		t.Fatal("Acquire after the last Release succeeded")
	}
	// An extra release is a bug that is logged, never a panic or a second
	// destroy.
	ctx.Release()

	// gl and egl are immutable: a dead context keeps them, and Lock refuses.
	if ctx.GL() == nil {
		t.Fatal("GL() cleared by destruction; the fields must stay immutable")
	}
	glCtx, err := ctx.Lock()
	if !errors.Is(err, ErrContextReleased) || glCtx != nil {
		t.Fatalf("Lock on a dead context = (%p, %v), want (nil, ErrContextReleased)", glCtx, err)
	}
	glCtx, err = ctx.LockForSurface(egl.NoSurface)
	if !errors.Is(err, ErrContextReleased) || glCtx != nil {
		t.Fatalf("LockForSurface on a dead context = (%p, %v), want (nil, ErrContextReleased)", glCtx, err)
	}
}

// TestAdapterContextLockErrorReleasesMutex checks the R3-N1 convention: a
// failed Lock leaves nothing held, so the caller must not Unlock and a later
// Lock does not block.
func TestAdapterContextLockErrorReleasesMutex(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{})
	ctx.Release()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 3 {
			if _, err := ctx.Lock(); err == nil {
				t.Error("Lock on a dead context succeeded")
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Lock after a failed Lock blocked: the mutex was left held")
	}
}

// TestAdapterContextReleaseWaitsForCurrentHolder checks that the final
// release does not destroy the context while another goroutine has it
// current, and that a Lock after the dead mark fails.
func TestAdapterContextReleaseWaitsForCurrentHolder(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{})
	if _, err := ctx.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}

	released := make(chan struct{})
	go func() {
		ctx.Release()
		close(released)
	}()

	select {
	case <-released:
		t.Fatal("the final Release completed while the context was current")
	case <-time.After(50 * time.Millisecond):
	}
	if ctx.Alive() {
		t.Fatal("the final Release did not mark the context dead before waiting")
	}
	ctx.Unlock()

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("the final Release did not complete after Unlock")
	}
	if _, err := ctx.Lock(); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Lock after the final Release = %v, want ErrContextReleased", err)
	}
}

// TestAdapterContextGLUnderLock checks that GL() takes no lock: code under
// Lock calls it (CreateRenderPipeline → compileFragmentShader).
func TestAdapterContextGLUnderLock(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)
	if _, err := ctx.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	done := make(chan *gl.Context)
	go func() { done <- ctx.GL() }()
	select {
	case got := <-done:
		if got != glCtx {
			t.Fatalf("GL() = %p, want %p", got, glCtx)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GL() blocked behind Lock")
	}
	ctx.Unlock()
}

// TestSurfaceAndDeviceShareTheContext is the #362 order in the new model:
// the window closes (Surface first), the Device keeps the context, and the
// context dies with the Device, not with the Instance.
func TestSurfaceAndDeviceShareTheContext(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{}) // the Surface's reference
	surf := &Surface{ctx: ctx}
	if !ctx.Acquire() { // what Adapter.Open takes for the Device
		t.Fatal("device Acquire failed")
	}
	device := &Device{ctx: ctx}

	surf.Destroy()
	if !ctx.Alive() {
		t.Fatal("Surface.Destroy freed the context the Device still uses")
	}
	device.Destroy()
	if ctx.Alive() {
		t.Fatal("the context outlived its last Surface and Device")
	}
}

func TestDeviceDestroyReleasesItsReferenceOnce(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{}) // another holder's reference
	if !ctx.Acquire() {
		t.Fatal("device Acquire failed")
	}
	device := &Device{ctx: ctx}
	device.Destroy()
	device.Destroy()
	if !ctx.Alive() {
		t.Fatal("a second Device.Destroy released another holder's reference")
	}
}

func TestSurfaceDestroyReleasesItsReferenceOnce(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{}) // another holder's reference
	if !ctx.Acquire() {
		t.Fatal("surface Acquire failed")
	}
	surf := &Surface{ctx: ctx}
	surf.Destroy()
	surf.Destroy()
	if !ctx.Alive() {
		t.Fatal("a second Surface.Destroy released another holder's reference")
	}
}

// TestAdapterOpenOnReleasedContext checks the weak adapter: once every
// surface and device of its context is gone, Open is a typed error, not a
// panic, and the adapter reports no surface support.
func TestAdapterOpenOnReleasedContext(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{})
	adapter := &Adapter{ctx: ctx} // weak: holds no reference
	ctx.Release()

	_, err := adapter.Open(0, gputypes.DefaultLimits())
	if !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Open on a released context = %v, want ErrContextReleased", err)
	}
	if caps := adapter.SurfaceCapabilities(&Surface{}); caps != nil {
		t.Fatalf("SurfaceCapabilities on a released context = %+v, want nil", caps)
	}
}

func TestAdapterQualifySurfaceRejectsUnusableSurfaces(t *testing.T) {
	adapter := &Adapter{}
	if _, err := adapter.QualifySurface(nil); err == nil {
		t.Fatal("QualifySurface(nil) succeeded")
	}
	if _, err := adapter.QualifySurface(&Surface{}); err == nil {
		t.Fatal("QualifySurface of a surface without a context succeeded")
	}
	dead := NewAdapterContext(nil, &gl.Context{})
	dead.Release()
	if _, err := adapter.QualifySurface(&Surface{ctx: dead}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("QualifySurface of a released context = %v, want ErrContextReleased", err)
	}
}

// TestSurfaceConfigureRejectsForeignDevice checks that a Device opened on
// another context cannot configure the surface: GL objects are not shared
// between EGL contexts, and on Wayland the two may sit on different
// wl_displays.
func TestSurfaceConfigureRejectsForeignDevice(t *testing.T) {
	surf := &Surface{ctx: NewAdapterContext(nil, &gl.Context{})}
	device := &Device{ctx: NewAdapterContext(nil, &gl.Context{})}
	err := surf.Configure(device, &hal.SurfaceConfiguration{Width: 64, Height: 64})
	if !errors.Is(err, hal.ErrSurfaceDeviceMismatch) {
		t.Fatalf("Configure with a foreign Device = %v, want hal.ErrSurfaceDeviceMismatch", err)
	}
}

func TestInstanceLookupOrder(t *testing.T) {
	const dpy = 0x1000
	instCtx := NewAdapterContext(nil, &gl.Context{})
	pathB := NewAdapterContext(nil, &gl.Context{})
	inst := &Instance{
		ctx:     instCtx,
		ctxKind: egl.WindowKindX11,
		displayContexts: map[displayKey]*AdapterContext{
			{kind: egl.WindowKindWayland, display: dpy}: pathB,
		},
	}

	// 1. The instance context, by window kind (X11 XIDs are server-global).
	if got := inst.lookupContext(egl.WindowKindX11, 0x2000); got != instCtx {
		t.Fatalf("X11 lookup = %p, want the instance context %p", got, instCtx)
	}
	got := inst.lookupContext(egl.WindowKindWayland, dpy)
	if got != pathB {
		t.Fatalf("Wayland lookup = %p, want the Path B context of that display %p", got, pathB)
	}
	// Each hit is a strong reference for the new Surface.
	got.Release()
	instCtx.Release()
	if !pathB.Alive() || !instCtx.Alive() {
		t.Fatal("lookup did not take a reference for the caller")
	}

	// 3. Nothing for another display: the caller builds a new context.
	if got := inst.lookupContext(egl.WindowKindWayland, dpy+0x10); got != nil {
		t.Fatalf("lookup of an unknown display = %p, want nil", got)
	}

	// A dead Path B context is dropped from the table, never reused.
	pathB.Release()
	if got := inst.lookupContext(egl.WindowKindWayland, dpy); got != nil {
		t.Fatalf("lookup of a released context = %p, want nil", got)
	}
	if _, ok := inst.displayContexts[displayKey{kind: egl.WindowKindWayland, display: dpy}]; ok {
		t.Fatal("the dead Path B context stayed in the table")
	}
}

// TestInstanceContextBoundToCallerDisplay checks D's instance context: built
// on the caller's display, it is shared only with surfaces of that display.
func TestInstanceContextBoundToCallerDisplay(t *testing.T) {
	const dpy = 0x1000
	instCtx := NewAdapterContext(nil, &gl.Context{})
	inst := &Instance{ctx: instCtx, ctxKind: egl.WindowKindWayland, ctxDisplay: dpy}

	if got := inst.lookupContext(egl.WindowKindWayland, dpy); got != instCtx {
		t.Fatalf("lookup on the instance display = %p, want the instance context", got)
	}
	if got := inst.lookupContext(egl.WindowKindWayland, dpy+0x10); got != nil {
		t.Fatalf("lookup on another display = %p, want nil", got)
	}
}

// TestInstanceDestroyKeepsSurfaceContexts checks that the Instance holds no
// reference to Path B contexts: they live with their surfaces and devices.
func TestInstanceDestroyKeepsSurfaceContexts(t *testing.T) {
	pathB := NewAdapterContext(nil, &gl.Context{}) // the Surface's reference
	instCtx := NewAdapterContext(nil, &gl.Context{})
	inst := &Instance{
		ctx: instCtx,
		displayContexts: map[displayKey]*AdapterContext{
			{kind: egl.WindowKindWayland, display: 1}: pathB,
		},
	}
	inst.Destroy()
	if instCtx.Alive() {
		t.Fatal("Instance.Destroy kept its own context alive")
	}
	if !pathB.Alive() {
		t.Fatal("Instance.Destroy released a context only its Surface holds")
	}
	inst.Destroy() // idempotent
}

// TestInstanceLookupRacesFinalRelease runs CreateSurface's lookup against
// the final release of the same context (run with -race).
func TestInstanceLookupRacesFinalRelease(t *testing.T) {
	for range 200 {
		ctx := NewAdapterContext(nil, &gl.Context{})
		inst := &Instance{displayContexts: map[displayKey]*AdapterContext{
			{kind: egl.WindowKindWayland, display: 1}: ctx,
		}}
		var wg sync.WaitGroup
		wg.Go(func() {
			if got := inst.lookupContext(egl.WindowKindWayland, 1); got != nil {
				if _, err := got.Lock(); err != nil {
					t.Errorf("Lock on a context the lookup acquired: %v", err)
					return
				}
				got.Unlock()
				got.Release()
			}
		})
		wg.Go(ctx.Release)
		wg.Wait()
		if ctx.Alive() {
			t.Fatal("context alive after every reference was released")
		}
	}
}
