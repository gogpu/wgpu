// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package wgl provides Windows OpenGL (WGL) context management.
//
// This package handles the Windows-specific parts of OpenGL initialization:
//   - Loading opengl32.dll, gdi32.dll, user32.dll
//   - WGL context creation and management
//   - Pixel format selection
//   - Buffer swapping
//
// # Usage
//
//	if err := wgl.Init(); err != nil {
//	    return err
//	}
//	ctx, err := wgl.NewContext(hwnd)
//	if err != nil {
//	    return err
//	}
//	defer ctx.Destroy(hwnd)
//	ctx.MakeCurrent()
//
// # Function Loading
//
// Use GetGLProcAddress to load OpenGL functions:
//
//	glCtx := &gl.Context{}
//	glCtx.Load(wgl.GetGLProcAddress)
package wgl
