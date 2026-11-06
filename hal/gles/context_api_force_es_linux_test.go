// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm) && wgpu_gles_force_es

package gles

import (
	"os"
	"strings"
	"testing"
)

// TestForcedGLESInstanceContextIsES checks the wgpu_gles_force_es seam: the
// instance context is OpenGL ES even where the driver offers desktop GL, so
// the tests built with the tag run the backend's ES paths. It skips without
// an EGL context, and fails instead on a runner whose WGPU_REQUIRE lists gles.
func TestForcedGLESInstanceContextIsES(t *testing.T) {
	inst, err := Backend{}.CreateInstance(nil)
	if err != nil {
		skipUnlessGLESRequired(t, "no EGL: "+err.Error())
	}
	defer inst.Destroy()
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("no adapter from the instance context")
	}
	adapter, ok := adapters[0].Adapter.(*Adapter)
	if !ok || adapter.ctx == nil {
		skipUnlessGLESRequired(t, "no instance context (Wayland, or EGL without a surfaceless or pbuffer path)")
	}
	driver := adapters[0].Info.Driver
	if !strings.HasPrefix(driver, "OpenGL ES ") {
		t.Fatalf("instance context version %q, want OpenGL ES under wgpu_gles_force_es", driver)
	}
	t.Logf("instance context: %s (%s)", driver, adapters[0].Info.Name)
}

func skipUnlessGLESRequired(tb testing.TB, reason string) {
	tb.Helper()
	for _, item := range strings.Split(os.Getenv("WGPU_REQUIRE"), ",") {
		if strings.TrimSpace(item) == "gles" {
			tb.Fatalf("%s, but WGPU_REQUIRE=%q requires GLES", reason, os.Getenv("WGPU_REQUIRE"))
		}
	}
	tb.Skip(reason)
}
