// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

// Package egl provides EGL (EGL) context management for OpenGL ES on Linux.
//
// EGL is the native interface for OpenGL ES on Linux, supporting:
//   - X11 displays (via EGL_PLATFORM_X11_KHR)
//   - Wayland displays (via EGL_PLATFORM_WAYLAND_KHR)
//   - Surfaceless contexts (via EGL_PLATFORM_SURFACELESS_MESA)
//
// This implementation uses github.com/go-webgpu/goffi for Pure Go FFI
// without requiring CGO.
package egl
