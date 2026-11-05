//go:build integration && linux && !(js && wasm)

package gles

import (
	"errors"
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// newTestAdapterContext creates a real EGL/GL context wrapped in an
// AdapterContext, or skips when no EGL display is available. The caller holds
// its first reference.
func newTestAdapterContext(t *testing.T) *AdapterContext {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)

	if err := egl.Init(); err != nil {
		t.Skipf("egl.Init() failed: %v", err)
	}
	config := egl.DefaultContextConfig()
	config.GLES = false
	eglCtx, err := egl.NewContext(config)
	if err != nil {
		t.Skipf("egl.NewContext() failed: %v", err)
	}
	if err := eglCtx.MakeCurrent(); err != nil {
		eglCtx.Destroy()
		t.Fatalf("MakeCurrent failed: %v", err)
	}
	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress); err != nil {
		eglCtx.Destroy()
		t.Fatalf("GL load failed: %v", err)
	}
	_ = egl.MakeCurrent(eglCtx.Display(), egl.NoSurface, egl.NoSurface, egl.NoContext)
	return NewAdapterContext(eglCtx, glCtx)
}

// TestSurfaceDestroyKeepsDeviceContext is the #362 order: the window closes
// (Surface first), then the Device, then the Instance. Surface.Destroy must
// not free the context the Device still uses (before #362 a later
// Device.Destroy locked a nil EGL context and panicked), and since ADR-077
// the context dies with the Device, not with the Instance.
func TestSurfaceDestroyKeepsDeviceContext(t *testing.T) {
	ctx := newTestAdapterContext(t) // the Surface's reference
	surf := &Surface{ctx: ctx}
	adapters := (&Instance{}).EnumerateAdapters(surf)
	if len(adapters) == 0 {
		t.Skip("context below OpenGL 3.3 / ES 3.0: no adapter")
	}
	open, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	device := open.Device.(*Device)

	// Window close destroys the Surface first.
	surf.Destroy()
	if !ctx.Alive() {
		t.Fatal("Surface.Destroy freed the context the Device still uses")
	}

	// The Device still works on the context.
	buf, err := device.CreateBuffer(&hal.BufferDescriptor{Size: 16, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer after Surface.Destroy: %v", err)
	}
	device.DestroyBuffer(buf)

	// Shutdown destroys the Device next — must not nil-deref, and it is the
	// last holder of the context.
	device.Destroy()
	if ctx.Alive() {
		t.Fatal("the context outlived its last Surface and Device")
	}
}

// TestCachedAdapterRequalifiedOnRealContext is the window-closed-and-reopened
// case on real EGL: the adapter enumerated with the first surface fails Open
// with a typed error once that surface is gone, and QualifySurface gives a
// working adapter for the next surface.
func TestCachedAdapterRequalifiedOnRealContext(t *testing.T) {
	first := &Surface{ctx: newTestAdapterContext(t)}
	adapters := (&Instance{}).EnumerateAdapters(first)
	if len(adapters) == 0 {
		t.Skip("context below OpenGL 3.3 / ES 3.0: no adapter")
	}
	cached := adapters[0].Adapter.(*Adapter)
	first.Destroy()

	if _, err := cached.Open(0, gputypes.DefaultLimits()); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Open on the closed window's adapter = %v, want ErrContextReleased", err)
	}

	second := &Surface{ctx: newTestAdapterContext(t)}
	qualified, err := cached.QualifySurface(second)
	if err != nil {
		t.Fatalf("QualifySurface: %v", err)
	}
	open, err := qualified.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open on the requalified adapter: %v", err)
	}
	buf, err := open.Device.CreateBuffer(&hal.BufferDescriptor{Size: 16, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	open.Device.DestroyBuffer(buf)
	open.Device.Destroy()
	second.Destroy()
	if second.ctx != nil {
		t.Fatal("Surface.Destroy kept its context pointer")
	}
}

// TestInstanceSurvivesContextOfReleasedDevice checks the instance context's
// own reference: a Device opened on it and released does not take it down.
func TestInstanceSurvivesContextOfReleasedDevice(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	raw, err := Backend{}.CreateInstance(nil)
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	inst := raw.(*Instance)
	defer inst.Destroy()
	ctx := inst.context()
	if ctx == nil {
		t.Skip("no EGL instance context (no display or driver)")
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 || adapters[0].Adapter.(*Adapter).ctx == nil {
		t.Skip("no adapter on the instance context")
	}
	open, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	open.Device.Destroy()
	if !ctx.Alive() {
		t.Fatal("releasing a Device destroyed the Instance's own context")
	}
	if _, err := ctx.Lock(); err != nil {
		t.Fatalf("Lock on the instance context: %v", err)
	}
	ctx.Unlock()
}
