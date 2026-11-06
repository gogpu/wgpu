// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm) && wgpu_gles_force_es

package gles

// forceGLES is true under the wgpu_gles_force_es test tag: every context is
// OpenGL ES 3.0 (see context_api_linux.go).
const forceGLES = true
