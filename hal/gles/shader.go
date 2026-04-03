// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/wgpu/hal"
)

// compileWGSLToGLSL compiles a WGSL shader source to GLSL for the given entry point.
// OpenGL does not understand WGSL, so we use naga to parse WGSL and emit GLSL 4.30 core.
// GLSL 4.30 is required because naga emits layout(binding=N) qualifiers which are
// not available in GLSL 3.30. OpenGL 4.3+ is supported on all modern GPUs (2012+).
//
// Returns the GLSL source and TranslationInfo containing TextureMappings for
// SamplerBindMap construction (which sampler goes with which texture unit).
func compileWGSLToGLSL(source hal.ShaderSource, entryPoint string) (string, glsl.TranslationInfo, error) {
	if source.WGSL == "" {
		return "", glsl.TranslationInfo{}, fmt.Errorf("gles: shader source has no WGSL code")
	}

	// Parse WGSL to AST.
	ast, err := naga.Parse(source.WGSL)
	if err != nil {
		return "", glsl.TranslationInfo{}, fmt.Errorf("gles: WGSL parse error: %w", err)
	}

	// Lower AST to IR.
	module, err := naga.Lower(ast)
	if err != nil {
		return "", glsl.TranslationInfo{}, fmt.Errorf("gles: WGSL lower error: %w", err)
	}

	// Build a BindingMap that maps (group, binding) to flat GL texture unit indices.
	// The formula group*16 + binding must match SetBindGroupCommand.Execute in command.go,
	// which uses the same maxBindingsPerGroup=16 constant to compute glBinding.
	// Without this map, naga emits layout(binding=N) using the raw WGSL binding number,
	// which does not match the texture units that SetBindGroupCommand activates.
	const maxBindingsPerGroup = 16
	bindingMap := make(map[glsl.BindingMapKey]uint8, len(module.GlobalVariables))
	for _, gv := range module.GlobalVariables {
		if gv.Binding == nil {
			continue
		}
		flatBinding := gv.Binding.Group*maxBindingsPerGroup + gv.Binding.Binding
		bindingMap[glsl.BindingMapKey{
			Group:   gv.Binding.Group,
			Binding: gv.Binding.Binding,
		}] = uint8(flatBinding)
	}

	// Compile IR to GLSL 4.30 core.
	// Version 4.30 is needed for layout(binding=N) resource binding qualifiers
	// and compute shader support (local_size_x/y/z).
	glslCode, translationInfo, err := glsl.Compile(module, glsl.Options{
		LangVersion:        glsl.Version430,
		EntryPoint:         entryPoint,
		ForceHighPrecision: true,
		BindingMap:         bindingMap,
		// ADJUST_COORDINATE_SPACE: naga appends gl_Position.yz = vec2(-gl_Position.y, gl_Position.z * 2.0 - gl_Position.w)
		// at the end of vertex shaders. This flips Y and remaps Z from [0,1] to [-1,1].
		// The scene renders upside-down in GL; the present blit (MSAAResolveCommand) flips it back.
		// This matches Rust wgpu-hal GLES (device.rs:1160) and fixes gl_FragCoord.y convention:
		// with the flip, gl_FragCoord.y=0 is at the top (WebGPU convention), not bottom (GL convention).
		// Without this, rrect_clip_coverage() in fragment shaders gets wrong Y values (BUG-GLES-SCROLLBAR-001).
		WriterFlags: glsl.WriterFlagAdjustCoordinateSpace | glsl.WriterFlagForcePointSize,
	})
	if err != nil {
		return "", glsl.TranslationInfo{}, fmt.Errorf("gles: GLSL compile error for entry point %q: %w", entryPoint, err)
	}

	hal.Logger().Debug("gles: GLSL generated",
		"entryPoint", entryPoint,
		"sourceLen", len(glslCode),
	)
	if hal.Logger().Enabled(context.Background(), slog.LevelDebug) {
		preview := glslCode
		if len(preview) > 2000 {
			preview = preview[:2000] + "..."
		}
		hal.Logger().Debug("gles: GLSL source", "glsl", preview)
	}

	return glslCode, translationInfo, nil
}
