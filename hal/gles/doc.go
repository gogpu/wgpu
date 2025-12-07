// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package gles provides an OpenGL ES backend for the HAL.
//
// # Status
//
// This backend is PLANNED and not yet implemented.
// See TASK-018 for implementation roadmap.
//
// # Target Versions
//
//   - OpenGL 3.3+ (Desktop)
//   - OpenGL ES 3.0+ (Mobile/WebGL)
//
// # Why OpenGL First?
//
//  1. Most portable (Windows, Linux, macOS, WebGL, Android, iOS)
//  2. Simplest API among modern graphics APIs
//  3. go-gl/gl provides excellent reference
//  4. Good for learning HAL implementation patterns
//
// # Reference Libraries
//
//   - go-gl/gl      - OpenGL bindings (Pure Go via purego possible)
//   - Gio           - Has GL backend in Pure Go
//   - Ebitengine    - Uses OpenGL for some platforms
//
// # Implementation Plan
//
//  1. Basic initialization (context creation)
//  2. Buffer management (VBO, UBO)
//  3. Texture management
//  4. Shader compilation (GLSL from SPIR-V)
//  5. Render pipeline
//  6. Compute (if OpenGL 4.3+)
//
// # Usage (Future)
//
//	import (
//	    "github.com/gogpu/wgpu/hal"
//	    _ "github.com/gogpu/wgpu/hal/gl" // Register OpenGL backend
//	)
//
//	backend := hal.GetBackend(types.BackendGL)
//	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{})
package gles
