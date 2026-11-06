// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm) && !wgpu_gles_force_es

package gles

// forceGLES makes the backend create OpenGL ES 3.0 contexts only, never
// desktop GL. It is a test seam: building with the tag wgpu_gles_force_es
// (context_api_force_es_linux.go) runs the backend's OpenGL ES paths on a
// driver that also offers desktop GL, as Mesa does. Rust wgpu-hal egl.rs has
// no such switch: it picks desktop GL whenever EGL_CLIENT_APIS lists it.
const forceGLES = false
