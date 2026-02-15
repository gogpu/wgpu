// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"fmt"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/wgpu/hal"
)

// compileWGSLToGLSL compiles a WGSL shader source to GLSL for the given entry point.
// OpenGL does not understand WGSL, so we use naga to parse WGSL and emit GLSL 4.30 core.
// GLSL 4.30 is required because naga emits layout(binding=N) qualifiers which are
// not available in GLSL 3.30. OpenGL 4.3+ is supported on all modern GPUs (2012+).
func compileWGSLToGLSL(source hal.ShaderSource, entryPoint string) (string, error) {
	if source.WGSL == "" {
		return "", fmt.Errorf("gles: shader source has no WGSL code")
	}

	// Parse WGSL to AST.
	ast, err := naga.Parse(source.WGSL)
	if err != nil {
		return "", fmt.Errorf("gles: WGSL parse error: %w", err)
	}

	// Lower AST to IR.
	module, err := naga.Lower(ast)
	if err != nil {
		return "", fmt.Errorf("gles: WGSL lower error: %w", err)
	}

	// Compile IR to GLSL 4.30 core.
	// Version 4.30 is needed for layout(binding=N) resource binding qualifiers
	// and compute shader support (local_size_x/y/z).
	glslCode, _, err := glsl.Compile(module, glsl.Options{
		LangVersion:        glsl.Version430,
		EntryPoint:         entryPoint,
		ForceHighPrecision: true,
	})
	if err != nil {
		return "", fmt.Errorf("gles: GLSL compile error for entry point %q: %w", entryPoint, err)
	}

	return glslCode, nil
}
