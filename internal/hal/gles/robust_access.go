// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

// RobustAccess reports whether the device's GL context was created with
// robust buffer access, which makes the driver contain out-of-bounds buffer
// accesses from shaders: naga's GLSL output leaves them unchecked, as Rust's
// does. EGL requests it as Rust wgpu-hal does; WGL requests it too, beyond
// Rust (wgl.CreateRobustContext).
func (d *Device) RobustAccess() bool {
	return d.ctx != nil && d.ctx.RobustAccess()
}
