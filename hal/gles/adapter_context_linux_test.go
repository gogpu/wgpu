// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

func TestNewAdapterContext_Accessors(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, false)

	if ctx.GL() != glCtx {
		t.Fatalf("GL() = %p, want %p", ctx.GL(), glCtx)
	}
	if ctx.EGL() != nil {
		t.Fatalf("EGL() = %v, want nil", ctx.EGL())
	}
	if ctx.owns {
		t.Fatal("owns should be false")
	}
}

func TestAdapterContext_Destroy_OwnedNilEGLIsNoOp(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, true)
	ctx.Destroy()
	// eglCtx is nil → Destroy body skipped; GL table retained.
	if ctx.GL() != glCtx {
		t.Fatal("Destroy with nil eglCtx should not clear GL table")
	}
}

func TestAdapterContext_Destroy_BorrowedDoesNotClear(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, false)
	ctx.Destroy()
	if ctx.GL() != glCtx {
		t.Fatal("borrowed Destroy must not clear GL table")
	}
	if ctx.owns {
		t.Fatal("borrowed flag must stay false")
	}
}

func TestAdapterContext_LockUnlock_NilEGL(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, false)

	got := ctx.Lock()
	if got != glCtx {
		t.Fatalf("Lock() = %p, want %p", got, glCtx)
	}
	// Must not panic even without egl.Init (Unlock short-circuits on nil eglCtx).
	ctx.Unlock()
}

func TestAdapterContext_LockForSurface_NilEGL(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, false)

	got := ctx.LockForSurface(egl.NoSurface)
	if got != glCtx {
		t.Fatalf("LockForSurface() = %p, want %p", got, glCtx)
	}
	ctx.Unlock()
}

func TestAdapterContext_LockSerializesConcurrentAccess(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, false)

	const goroutines = 32
	const iters = 50
	var wg sync.WaitGroup
	var counter int

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iters {
				_ = ctx.Lock()
				counter++
				ctx.Unlock()
			}
		}()
	}
	wg.Wait()

	want := goroutines * iters
	if counter != want {
		t.Fatalf("counter = %d, want %d (mutex failed to serialize)", counter, want)
	}
}

func TestInstance_EnumerateAdapters_NoContextReturnsPlaceholder(t *testing.T) {
	inst := &Instance{}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) != 1 {
		t.Fatalf("len(adapters) = %d, want 1", len(adapters))
	}
	info := adapters[0].Info
	if info.Vendor != vendorUnknown {
		t.Errorf("Vendor = %q, want %q", info.Vendor, vendorUnknown)
	}
	if !strings.Contains(info.DriverInfo, "RequestAdapterWithSurface") {
		t.Errorf("DriverInfo %q should guide caller to RequestAdapterWithSurface", info.DriverInfo)
	}
	if adapters[0].Adapter.(*Adapter).ctx != nil {
		t.Fatal("placeholder adapter must have nil AdapterContext")
	}
}

func TestInstance_Destroy_NilSafe(t *testing.T) {
	inst := &Instance{}
	inst.Destroy() // must not panic
	inst.Destroy()
}

func TestInstance_Destroy_ClearsOwnedContext(t *testing.T) {
	glCtx := &gl.Context{}
	inst := &Instance{ctx: NewAdapterContext(nil, glCtx, true)}
	inst.Destroy()
	if inst.ctx != nil {
		t.Fatal("Instance.Destroy should nil out ctx")
	}
}

func TestSurface_Destroy_SharedDoesNotDestroyAdapterContext(t *testing.T) {
	glCtx := &gl.Context{}
	shared := NewAdapterContext(nil, glCtx, true)
	surf := &Surface{
		ctx:         shared,
		ownsContext: false,
	}
	surf.Destroy()
	if surf.ctx != nil {
		t.Fatal("Surface.Destroy should nil local ctx pointer")
	}
	if shared.GL() != glCtx {
		t.Fatal("shared AdapterContext must survive Surface.Destroy when ownsContext=false")
	}
}

func TestSurface_Destroy_OwnedCallsAdapterContextDestroy(t *testing.T) {
	glCtx := &gl.Context{}
	owned := NewAdapterContext(nil, glCtx, true)
	surf := &Surface{
		ctx:         owned,
		ownsContext: true,
	}
	surf.Destroy()
	if surf.ctx != nil {
		t.Fatal("Surface.Destroy should nil local ctx pointer")
	}
	// owns=true but eglCtx=nil → AdapterContext.Destroy is a no-op for GL table;
	// ownership path still invoked (no panic). Covered fully in integration tests.
}

func TestSurface_GetAdapterInfo_NilCtxReturnsPlaceholder(t *testing.T) {
	surf := &Surface{}
	info := surf.GetAdapterInfo()
	if info.Adapter == nil {
		t.Fatal("expected placeholder adapter")
	}
	if info.Info.Vendor != vendorUnknown {
		t.Errorf("Vendor = %q, want %q", info.Info.Vendor, vendorUnknown)
	}
	if info.Info.Backend != gputypes.BackendGL {
		t.Errorf("Backend = %v, want BackendGL", info.Info.Backend)
	}
}

func TestSurface_Configure_NilCtxReturnsError(t *testing.T) {
	surf := &Surface{}
	err := surf.Configure(nil, &hal.SurfaceConfiguration{Width: 64, Height: 64})
	if err == nil || !strings.Contains(err.Error(), "AdapterContext") {
		t.Fatalf("Configure(nil ctx) error = %v", err)
	}
}

func TestSurface_Configure_ZeroArea(t *testing.T) {
	surf := &Surface{ctx: NewAdapterContext(nil, &gl.Context{}, false)}
	err := surf.Configure(nil, &hal.SurfaceConfiguration{Width: 0, Height: 64})
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Fatalf("Configure(zero width) = %v, want ErrZeroArea", err)
	}
}

func TestQueue_Present_InvalidSurfaceType(t *testing.T) {
	q := &Queue{ctx: NewAdapterContext(nil, &gl.Context{}, false)}
	err := q.Present(nil, nil, nil)
	if err == nil {
		t.Fatal("Present(nil) should return error")
	}
	if !strings.Contains(err.Error(), "invalid surface") {
		t.Errorf("error %q should mention invalid surface", err.Error())
	}
}

func TestQueue_WriteBuffer_InvalidType(t *testing.T) {
	q := &Queue{ctx: NewAdapterContext(nil, &gl.Context{}, false)}
	err := q.WriteBuffer(nil, 0, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("WriteBuffer(nil) error = %v", err)
	}
}

func TestDevice_DestroyBuffer_NilCtxSafe(t *testing.T) {
	d := &Device{} // no AdapterContext
	d.DestroyBuffer(&Buffer{id: 42, glCtx: &gl.Context{}})
	// Must not panic; without ctx we skip GL delete.
}

func TestDevice_DestroyPaths_AcquireLock(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, false)
	d := &Device{ctx: ctx}

	// All Destroy* paths must Lock/Unlock even with nil EGL (no MakeCurrent).
	d.DestroyBuffer(&Buffer{glCtx: glCtx})
	d.DestroyTexture(&Texture{glCtx: glCtx})
	d.DestroySampler(&Sampler{glCtx: glCtx})
	d.DestroyShaderModule(&ShaderModule{glCtx: glCtx})
	d.DestroyRenderPipeline(&RenderPipeline{glCtx: glCtx})
	d.DestroyComputePipeline(&ComputePipeline{glCtx: glCtx})
	d.DestroyFence(&Fence{glCtx: glCtx})
}

func TestAdapterContext_Destroy_SerializedWithLock(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx, true)

	_ = ctx.Lock()
	done := make(chan struct{})
	go func() {
		ctx.Destroy() // must wait until Unlock
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Destroy must not complete while Lock is held")
	case <-time.After(50 * time.Millisecond):
	}
	ctx.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Destroy did not complete after Unlock")
	}
}

func TestHALInterface_AdapterContextWiring(t *testing.T) {
	var _ hal.Instance = (*Instance)(nil)
	var _ hal.Adapter = (*Adapter)(nil)
	var _ hal.Device = (*Device)(nil)
	var _ hal.Queue = (*Queue)(nil)
	var _ hal.Surface = (*Surface)(nil)

	// Zero Instance must still satisfy EnumerateAdapters contract.
	adapters := (&Instance{}).EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("EnumerateAdapters must return placeholder")
	}
	if adapters[0].Info.Backend != gputypes.BackendGL {
		t.Errorf("Backend = %v, want BackendGL", adapters[0].Info.Backend)
	}
}
