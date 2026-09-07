// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && linux && !(js && wasm)

package gles

import (
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestEGLInit tests basic EGL initialization.
// This requires Mesa/EGL libraries to be installed.
// In CI, this uses the EGL_MESA_platform_surfaceless for headless testing.
// Run with: go test -v -tags integration ./hal/gles/...
func TestEGLInit(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Log("Testing EGL initialization...")

	// Initialize EGL library
	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
	}
	t.Log("EGL library loaded successfully")

	// Log client extensions (available before display initialization)
	clientExt := egl.QueryClientExtensions()
	t.Logf("EGL client extensions: %s", clientExt)

	// Check for surfaceless support
	if egl.HasSurfacelessSupport() {
		t.Log("EGL_MESA_platform_surfaceless is available")
	} else {
		t.Log("EGL_MESA_platform_surfaceless is NOT available")
	}

	// Get EGL display (will use surfaceless if no DISPLAY/WAYLAND_DISPLAY set)
	display, windowKind, displayOwner, err := egl.GetEGLDisplay(0)
	if err != nil {
		t.Fatalf("egl.GetEGLDisplay() failed: %v", err)
	}
	t.Logf("Got EGL display: %v (window kind: %v)", display, windowKind)

	// Validate display before initialization
	if display == egl.NoDisplay {
		t.Fatalf("egl.GetEGLDisplay() returned NoDisplay")
	}

	// Initialize EGL display
	var major, minor egl.EGLInt
	if egl.Initialize(display, &major, &minor) == egl.False {
		eglError := egl.GetError()
		t.Fatalf("egl.Initialize() failed: error 0x%x", eglError)
	}
	t.Logf("EGL version: %d.%d", major, minor)

	// Query EGL extensions
	extensions := egl.QueryString(display, egl.Extensions)
	t.Logf("EGL display extensions: %s", extensions)

	// Terminate EGL display first, then close native display connection
	if egl.Terminate(display) == egl.False {
		t.Errorf("egl.Terminate() failed: error 0x%x", egl.GetError())
	}
	t.Log("EGL terminated successfully")

	// Close native display connection (X11) after eglTerminate
	if displayOwner != nil {
		displayOwner.Close()
	}
}

// TestEGLContext tests EGL context creation.
func TestEGLContext(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Log("Testing EGL context creation...")

	// Initialize EGL
	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
	}

	// Create context with default config
	config := egl.DefaultContextConfig()
	config.GLES = false // Use desktop OpenGL
	config.Debug = true

	ctx, err := egl.NewContext(config)
	if err != nil {
		t.Skipf("egl.NewContext() failed (headless environment?): %v", err)
	}
	t.Logf("Created EGL context, window kind: %v", ctx.WindowKind())

	// Make current
	if err := ctx.MakeCurrent(); err != nil {
		t.Fatalf("ctx.MakeCurrent() failed: %v", err)
	}
	t.Log("Context made current")

	// Destroy context
	ctx.Destroy()
	t.Log("Context destroyed")
}

// TestGLObjectCreation verifies that GL object creation works through goffi.
// This catches the fundamental FFI pointer convention bug found by @lkmavi
// (PR #210): pointer-type args must use unsafe.Pointer(&ptr), not unsafe.Pointer(&value).
// Without this test, the bug went undetected because CI only tested EGL init.
func TestGLObjectCreation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
	}

	config := egl.DefaultContextConfig()
	config.GLES = false
	ctx, err := egl.NewContext(config)
	if err != nil {
		t.Skipf("egl.NewContext() failed: %v", err)
	}
	defer ctx.Destroy()

	if err := ctx.MakeCurrent(); err != nil {
		t.Fatalf("MakeCurrent failed: %v", err)
	}

	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress); err != nil {
		t.Fatalf("GL load failed: %v", err)
	}

	// GenBuffers — would return 0 with old FFI bug (garbage pointer to OpenGL)
	buf := glCtx.GenBuffers(1)
	if buf == 0 {
		t.Fatal("GenBuffers returned 0 — FFI pointer convention bug (see ADR-044)")
	}
	t.Logf("GenBuffers: %d", buf)
	glCtx.DeleteBuffers(buf)

	// GenTextures
	tex := glCtx.GenTextures(1)
	if tex == 0 {
		t.Fatal("GenTextures returned 0")
	}
	t.Logf("GenTextures: %d", tex)
	glCtx.DeleteTextures(tex)

	// GenVertexArrays
	vao := glCtx.GenVertexArrays(1)
	if vao == 0 {
		t.Fatal("GenVertexArrays returned 0")
	}
	t.Logf("GenVertexArrays: %d", vao)
	glCtx.DeleteVertexArrays(vao)

	// GenFramebuffers
	fbo := glCtx.GenFramebuffers(1)
	if fbo == 0 {
		t.Fatal("GenFramebuffers returned 0")
	}
	t.Logf("GenFramebuffers: %d", fbo)
	glCtx.DeleteFramebuffers(fbo)

	t.Log("All GL object creation/deletion passed — FFI pointer convention correct")
}

// TestGLESBackend tests the full GLES backend integration.
func TestGLESBackend(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Log("Testing GLES backend...")

	// Create backend
	backend := Backend{}
	if backend.Variant() != gputypes.BackendGL {
		t.Errorf("Expected BackendGL variant, got %v", backend.Variant())
	}

	// Create instance
	instance, err := backend.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance() failed: %v", err)
	}
	t.Log("Created GLES instance")

	// Enumerate adapters (without surface hint)
	adapters := instance.EnumerateAdapters(nil)
	t.Logf("Found %d adapter(s)", len(adapters))
	for i, adapter := range adapters {
		t.Logf("  Adapter %d: %s (%s)", i, adapter.Info.Name, adapter.Info.Driver)
	}

	// Destroy instance
	instance.Destroy()
	t.Log("Instance destroyed")
}

// TestGLProcAddress tests GL function loading via EGL.
func TestGLProcAddress(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Log("Testing GL function loading via EGL...")

	// Initialize EGL
	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
	}

	// Create and make context current
	config := egl.DefaultContextConfig()
	ctx, err := egl.NewContext(config)
	if err != nil {
		t.Skipf("egl.NewContext() failed (headless environment?): %v", err)
	}
	defer ctx.Destroy()

	if err := ctx.MakeCurrent(); err != nil {
		t.Fatalf("ctx.MakeCurrent() failed: %v", err)
	}

	// Test loading common GL functions
	glFunctions := []string{
		"glGetError",
		"glGetString",
		"glClear",
		"glClearColor",
		"glViewport",
		"glEnable",
		"glDisable",
		"glCreateShader",
		"glCreateProgram",
	}

	for _, name := range glFunctions {
		addr := egl.GetGLProcAddress(name)
		if addr == nil {
			t.Errorf("Failed to load %s", name)
		} else {
			t.Logf("Loaded %s: %p", name, addr)
		}
	}
}

// TestHALInterface tests that GLES types implement HAL interfaces.
func TestHALInterface(t *testing.T) {
	t.Log("Testing HAL interface compliance...")

	// Verify Backend implements hal.Backend
	var _ hal.Backend = Backend{}
	t.Log("Backend implements hal.Backend")

	// Verify Instance implements hal.Instance
	var _ hal.Instance = (*Instance)(nil)
	t.Log("Instance implements hal.Instance")

	// Verify Adapter implements hal.Adapter
	var _ hal.Adapter = (*Adapter)(nil)
	t.Log("Adapter implements hal.Adapter")

	// Verify Device implements hal.Device
	var _ hal.Device = (*Device)(nil)
	t.Log("Device implements hal.Device")

	// Verify Queue implements hal.Queue
	var _ hal.Queue = (*Queue)(nil)
	t.Log("Queue implements hal.Queue")

	// Verify Surface implements hal.Surface
	var _ hal.Surface = (*Surface)(nil)
	t.Log("Surface implements hal.Surface")
}

// TestAdapterContext_LockUnlockRoundTrip verifies mutex + LockOSThread + MakeCurrent
// via AdapterContext on a real EGL pbuffer/surfaceless context (#332).
func TestAdapterContext_LockUnlockRoundTrip(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
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

	// Unmake so AdapterContext.Lock must re-bind.
	_ = egl.MakeCurrent(eglCtx.Display(), egl.NoSurface, egl.NoSurface, egl.NoContext)

	ctx := NewAdapterContext(eglCtx, glCtx, true)
	defer ctx.Destroy()

	got := ctx.Lock()
	if got != glCtx {
		ctx.Unlock()
		t.Fatalf("Lock() returned %p, want %p", got, glCtx)
	}
	if egl.GetCurrentContext() == egl.NoContext {
		ctx.Unlock()
		t.Fatal("Lock() left no current EGL context")
	}

	// GL call while locked — GenBuffers must succeed.
	buf := got.GenBuffers(1)
	if buf == 0 {
		ctx.Unlock()
		t.Fatal("GenBuffers returned 0 under AdapterContext.Lock")
	}
	got.DeleteBuffers(buf)
	ctx.Unlock()

	if egl.GetCurrentContext() != egl.NoContext {
		t.Fatal("Unlock() should unmake the EGL context")
	}
}

// TestAdapterContext_LockForSurface_PbufferAsSurface uses the context's own
// pbuffer as the surface stand-in (no native window in headless CI).
func TestAdapterContext_LockForSurface_PbufferAsSurface(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
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

	ctx := NewAdapterContext(eglCtx, glCtx, true)
	defer ctx.Destroy()

	surf := eglCtx.Pbuffer()
	if surf == egl.NoSurface {
		// Surfaceless context — LockForSurface(NoSurface) is still valid EGL.
		t.Log("surfaceless context: LockForSurface(NoSurface)")
	}

	got := ctx.LockForSurface(surf)
	defer ctx.Unlock()

	if egl.GetCurrentContext() == egl.NoContext {
		t.Fatal("LockForSurface left no current EGL context")
	}
	if got.GetString(gl.VERSION) == "" {
		t.Fatal("GL_VERSION empty after LockForSurface")
	}
}

// TestInstance_CreateInstance_WrapsAdapterContext checks that a successful
// CreateInstance stores a non-nil AdapterContext (X11/headless). On Wayland
// skip-instance-context is intentional — then ctx may be nil.
func TestInstance_CreateInstance_WrapsAdapterContext(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	backend := Backend{}
	raw, err := backend.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer raw.Destroy()

	inst, ok := raw.(*Instance)
	if !ok {
		t.Fatalf("CreateInstance type = %T", raw)
	}

	if egl.DetectWindowKind() == egl.WindowKindWayland {
		if inst.ctx != nil {
			t.Log("Wayland with non-nil instance ctx (unusual but allowed if display available)")
		}
		return
	}

	if inst.ctx == nil {
		t.Skip("instance AdapterContext unavailable (no EGL display in environment)")
	}

	glCtx := inst.ctx.Lock()
	defer inst.ctx.Unlock()
	if glCtx == nil {
		t.Fatal("AdapterContext.GL is nil after CreateInstance")
	}
	version := glCtx.GetString(gl.VERSION)
	if version == "" {
		t.Fatal("empty GL_VERSION from instance AdapterContext")
	}
	t.Logf("instance AdapterContext OK: %s", version)
}
