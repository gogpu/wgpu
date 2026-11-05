// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

func TestNewAdapterContext_Accessors(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)

	if ctx.GL() != glCtx {
		t.Fatalf("GL() = %p, want %p", ctx.GL(), glCtx)
	}
	if ctx.EGL() != nil {
		t.Fatalf("EGL() = %v, want nil", ctx.EGL())
	}
	if !ctx.Alive() {
		t.Fatal("a new AdapterContext is not alive")
	}
}

func TestAdapterContext_Release_NilEGLKeepsGLTable(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)
	ctx.Release()
	// eglCtx is nil → nothing to destroy; the GL table is immutable.
	if ctx.GL() != glCtx {
		t.Fatal("Release must not clear the GL table")
	}
	if ctx.Alive() {
		t.Fatal("context alive after its only reference was released")
	}
}

func TestAdapterContext_LockUnlock_NilEGL(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)

	got, err := ctx.Lock()
	if err != nil || got != glCtx {
		t.Fatalf("Lock() = (%p, %v), want (%p, nil)", got, err, glCtx)
	}
	// Must not panic even without egl.Init (Unlock short-circuits on nil eglCtx).
	ctx.Unlock()
}

func TestAdapterContext_LockForSurface_NilEGL(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)

	got, err := ctx.LockForSurface(egl.NoSurface)
	if err != nil || got != glCtx {
		t.Fatalf("LockForSurface() = (%p, %v), want (%p, nil)", got, err, glCtx)
	}
	ctx.Unlock()
}

func TestAdapterContext_LockSerializesConcurrentAccess(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)

	const goroutines = 32
	const iters = 50
	var wg sync.WaitGroup
	var counter int

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iters {
				if _, err := ctx.Lock(); err != nil {
					t.Errorf("Lock: %v", err)
					return
				}
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

func TestInstance_Destroy_ReleasesOwnContext(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{})
	inst := &Instance{ctx: ctx}
	inst.Destroy()
	if inst.ctx != nil {
		t.Fatal("Instance.Destroy should nil out ctx")
	}
	if ctx.Alive() {
		t.Fatal("Instance.Destroy did not release its own context")
	}
}

func TestSurface_Destroy_SharedDoesNotDestroyAdapterContext(t *testing.T) {
	shared := NewAdapterContext(nil, &gl.Context{}) // the Instance's reference
	if !shared.Acquire() {
		t.Fatal("surface Acquire failed")
	}
	surf := &Surface{ctx: shared}
	surf.Destroy()
	if surf.ctx != nil {
		t.Fatal("Surface.Destroy should nil local ctx pointer")
	}
	if !shared.Alive() {
		t.Fatal("a shared AdapterContext must survive Surface.Destroy")
	}
}

func TestSurface_Destroy_LastReferenceDestroysAdapterContext(t *testing.T) {
	owned := NewAdapterContext(nil, &gl.Context{}) // the Surface's reference
	surf := &Surface{ctx: owned}
	surf.Destroy()
	if surf.ctx != nil {
		t.Fatal("Surface.Destroy should nil local ctx pointer")
	}
	if owned.Alive() {
		t.Fatal("the context outlived its only Surface")
	}
}

func TestSurface_GetAdapterInfo_NilCtxReturnsPlaceholder(t *testing.T) {
	surf := &Surface{}
	info, ok := surf.GetAdapterInfo()
	if !ok {
		t.Fatal("GetAdapterInfo without a context: ok=false, want the placeholder")
	}
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
	surf := &Surface{ctx: NewAdapterContext(nil, &gl.Context{})}
	err := surf.Configure(nil, &hal.SurfaceConfiguration{Width: 0, Height: 64})
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Fatalf("Configure(zero width) = %v, want ErrZeroArea", err)
	}
}

func TestQueue_Present_InvalidSurfaceType(t *testing.T) {
	q := &Queue{ctx: NewAdapterContext(nil, &gl.Context{})}
	err := q.Present(nil, nil, nil)
	if err == nil {
		t.Fatal("Present(nil) should return error")
	}
	if !strings.Contains(err.Error(), "invalid surface") {
		t.Errorf("error %q should mention invalid surface", err.Error())
	}
}

func TestQueue_WriteBuffer_InvalidType(t *testing.T) {
	q := &Queue{ctx: NewAdapterContext(nil, &gl.Context{})}
	err := q.WriteBuffer(nil, 0, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("WriteBuffer(nil) error = %v", err)
	}
}

func TestQueue_ReleasedContext(t *testing.T) {
	ctx := NewAdapterContext(nil, &gl.Context{})
	q := &Queue{ctx: ctx, submissionIndex: 7}
	ctx.Release()

	if _, err := q.Submit(nil); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Submit on a released context = %v, want ErrContextReleased", err)
	}
	if err := q.WriteBuffer(&Buffer{}, 0, []byte{1}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("WriteBuffer on a released context = %v, want ErrContextReleased", err)
	}
	if err := q.Present(&Surface{}, nil, nil); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Present on a released context = %v, want ErrContextReleased", err)
	}
	// A released context runs nothing more: everything submitted is done.
	q.fence = NewFence(nil)
	if got := q.PollCompleted(); got != 7 {
		t.Fatalf("PollCompleted on a released context = %d, want the submission index 7", got)
	}
}

func TestDevice_DestroyBuffer_NilCtxSafe(t *testing.T) {
	d := &Device{} // no AdapterContext
	d.DestroyBuffer(&Buffer{id: 42, glCtx: &gl.Context{}})
	// Must not panic; without ctx we skip GL delete.
}

func TestDevice_DestroyPaths_AcquireLock(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)
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

// TestDevice_ReleasedContext checks that every Device entry point returns
// ErrContextReleased, or does nothing, on a released context instead of
// calling GL through a context that is gone.
func TestDevice_ReleasedContext(t *testing.T) {
	glCtx := &gl.Context{}
	ctx := NewAdapterContext(nil, glCtx)
	d := &Device{ctx: ctx}
	ctx.Release()

	if _, err := d.CreateBuffer(&hal.BufferDescriptor{Size: 4}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("CreateBuffer = %v, want ErrContextReleased", err)
	}
	if _, err := d.CreateTexture(&hal.TextureDescriptor{}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("CreateTexture = %v, want ErrContextReleased", err)
	}
	if _, err := d.CreateSampler(&hal.SamplerDescriptor{}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("CreateSampler = %v, want ErrContextReleased", err)
	}
	if _, err := d.CreateCommandEncoder(nil); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("CreateCommandEncoder = %v, want ErrContextReleased", err)
	}
	if _, err := d.MapBuffer(&Buffer{size: 4}, 0, 4); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("MapBuffer = %v, want ErrContextReleased", err)
	}
	if err := d.UnmapBuffer(&Buffer{mapped: []byte{0}}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("UnmapBuffer = %v, want ErrContextReleased", err)
	}
	if _, err := d.Wait(&Fence{}, 1, 0); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Wait = %v, want ErrContextReleased", err)
	}
	if _, err := d.GetFenceStatus(&Fence{}); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("GetFenceStatus = %v, want ErrContextReleased", err)
	}
	if err := d.WaitIdle(); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("WaitIdle = %v, want ErrContextReleased", err)
	}
	// Destroy paths skip the GL work of a context that is gone.
	d.DestroyBuffer(&Buffer{glCtx: glCtx, id: 1})
	d.DestroyTexture(&Texture{glCtx: glCtx, id: 1})
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
