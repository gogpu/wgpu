//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gl

import "strings"

// isESVersionString reports whether a GL_VERSION string belongs to an OpenGL
// ES context. The ES specification fixes the GL_VERSION format as
// "OpenGL ES N.M <vendor info>"; desktop GL strings start with the version
// number. Rust wgpu-hal detects ES the same way (gles/adapter.rs parse_version).
func isESVersionString(version string) bool {
	return strings.HasPrefix(version, "OpenGL ES")
}
