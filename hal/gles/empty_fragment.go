// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// emptyFragmentShaderSource is the fragment shader linked into a program
// whose pipeline has no fragment stage (a depth-only pass). OpenGL ES 3.0
// fails LinkProgram for a program without a fragment shader; desktop GL links
// one, and the empty shader changes nothing there. Rust wgpu-hal does the same
// (wgpu-hal/src/gles/device.rs, create_program: "Create empty fragment
// shader if only vertex shader is present").
func emptyFragmentShaderSource(version glsl.Version) string {
	return "#version " + version.String() + "\nvoid main(void) {}\n"
}

// compileEmptyFragmentShader compiles emptyFragmentShaderSource. The caller
// holds the AdapterContext lock.
func compileEmptyFragmentShader(glCtx *gl.Context, version glsl.Version) (uint32, error) {
	id := glCtx.CreateShader(gl.FRAGMENT_SHADER)
	glCtx.ShaderSource(id, emptyFragmentShaderSource(version))
	glCtx.CompileShader(id)
	var status int32
	glCtx.GetShaderiv(id, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		log := glCtx.GetShaderInfoLog(id)
		glCtx.DeleteShader(id)
		return 0, fmt.Errorf("gles: empty fragment shader compilation failed: %s", log)
	}
	hal.Logger().Debug("gles: no fragment stage, linking an empty fragment shader", "version", version.String())
	return id, nil
}
