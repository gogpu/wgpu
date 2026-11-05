// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package egl

import (
	"fmt"
	"log/slog"

	"github.com/gogpu/wgpu/internal/eglrefs"
)

// displayVersion is the EGL version eglInitialize reports.
type displayVersion struct{ major, minor EGLInt }

// displays counts wgpu's users of each EGLDisplay in the process (ADR-077 R,
// Rust wgpu-hal DISPLAYS_REFERENCE_COUNT). eglGetPlatformDisplay returns one
// EGLDisplay for one native display, so two contexts on the surfaceless
// platform, or two on one wl_display, share it, and the first eglTerminate
// would kill the other's context. Every eglInitialize and eglTerminate in this
// package goes through it.
var displays = eglrefs.NewRegistry(initializeDisplay, terminateDisplay)

func initializeDisplay(display EGLDisplay) (displayVersion, error) {
	var v displayVersion
	if Initialize(display, &v.major, &v.minor) == False {
		return displayVersion{}, fmt.Errorf("eglInitialize failed: error 0x%x", GetError())
	}
	return v, nil
}

func terminateDisplay(display EGLDisplay) error {
	if Terminate(display) == False {
		return fmt.Errorf("eglTerminate failed: error 0x%x", GetError())
	}
	return nil
}

// acquireDisplay initializes display for one more user and returns its EGL
// version.
func acquireDisplay(display EGLDisplay) (major, minor EGLInt, err error) {
	v, err := displays.Acquire(display)
	if err != nil {
		return 0, 0, err
	}
	return v.major, v.minor, nil
}

// releaseDisplay drops one user of display and terminates it with the last.
// A failure has no caller to report to (it runs in teardown), so it is logged:
// a release without an acquire is a reference counting bug in this package.
func releaseDisplay(display EGLDisplay) {
	if err := displays.Release(display); err != nil {
		slog.Error("egl: release display", "display", fmt.Sprintf("0x%x", display), "err", err)
	}
}
