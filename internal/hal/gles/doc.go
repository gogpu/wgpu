//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package gles implements the HAL backend for OpenGL ES 3.0 / OpenGL 3.3+.
//
// This backend is Pure Go, without CGO: syscall on Windows, goffi on Linux.
//
// # Platform Support
//
// Supported:
//   - Windows (WGL) - via hal/gles/wgl package
//   - Linux (EGL: X11, Wayland, surfaceless) - via hal/gles/egl package
//
// Planned:
//   - Android (EGL)
//   - WebGL (via wasm)
//
// # Architecture
//
// The backend is organized into subpackages:
//
//	hal/gles/
//	├── gl/       - OpenGL function bindings
//	├── wgl/      - Windows GL context management
//	└── egl/      - EGL display and context management (Linux)
//
// # Usage
//
// Import this package to register the OpenGL backend:
//
//	import _ "github.com/gogpu/wgpu/hal/gles"
//
// The backend is then available via hal.GetBackend(gputypes.BackendGL).
//
// # OpenGL Version Requirements
//
// Minimum requirements:
//   - OpenGL 3.3 Core Profile (desktop)
//   - OpenGL ES 3.0 (mobile/embedded)
//
// The backend queries actual capabilities at runtime and adjusts
// feature availability accordingly.
//
// # Limitations
//
// Compared to Vulkan/Metal/DX12:
//   - No async compute (compute runs on graphics queue)
//   - Limited bindless resource support
//   - Single command queue
//   - Synchronous texture uploads
//
// # Command Recording
//
// This backend uses a command recording pattern similar to wgpu-hal.
// Commands are recorded during render/compute pass encoding and
// executed during Queue.Submit. This allows for:
//   - State tracking and optimization
//   - Better error handling
//   - Potential future multithreading
package gles
