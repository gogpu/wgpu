// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && linux

package gles

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/types"
)

// TestEGLInit tests basic EGL initialization.
// This requires Mesa/EGL libraries to be installed.
// In CI, this uses the EGL_MESA_platform_surfaceless for headless testing.
// Run with: go test -v -tags integration ./hal/gles/...
func TestEGLInit(t *testing.T) {
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
	display, windowKind, err := egl.GetEGLDisplay()
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

	// Terminate
	if egl.Terminate(display) == egl.False {
		t.Errorf("egl.Terminate() failed: error 0x%x", egl.GetError())
	}
	t.Log("EGL terminated successfully")
}

// TestEGLContext tests EGL context creation.
func TestEGLContext(t *testing.T) {
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

// TestGLESBackend tests the full GLES backend integration.
func TestGLESBackend(t *testing.T) {
	t.Log("Testing GLES backend...")

	// Create backend
	backend := Backend{}
	if backend.Variant() != types.BackendGL {
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
}
