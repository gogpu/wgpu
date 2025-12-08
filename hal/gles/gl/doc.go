// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package gl provides low-level OpenGL function bindings using syscall.
//
// This package is used internally by the GLES HAL backend.
// It loads OpenGL functions at runtime via wglGetProcAddress (Windows)
// or platform-specific loaders.
//
// # Usage
//
// The Context type holds function pointers loaded at runtime:
//
//	ctx := &gl.Context{}
//	ctx.Load(wgl.GetGLProcAddress)
//	ctx.ClearColor(0.2, 0.3, 0.3, 1.0)
//	ctx.Clear(gl.COLOR_BUFFER_BIT)
package gl
