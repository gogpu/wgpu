// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/wgpu/hal"
)

// compileWGSLToGLSL compiles a WGSL shader source to GLSL for the given entry point.
// OpenGL does not understand WGSL, so we use naga to parse WGSL and emit GLSL 4.30 core.
// GLSL 4.30 is required because naga emits layout(binding=N) qualifiers which are
// not available in GLSL 3.30. OpenGL 4.3+ is supported on all modern GPUs (2012+).
//
// The bindingMap parameter provides the pre-computed (group, binding) -> GL slot mapping
// from PipelineLayout (computed via per-type sequential counters in CreatePipelineLayout).
// If bindingMap is nil, no binding remapping is applied.
//
// Returns the GLSL source and TranslationInfo containing TextureMappings for
// SamplerBindMap construction (which sampler goes with which texture unit).
func compileWGSLToGLSL(source hal.ShaderSource, entryPoint string, bindingMap map[glsl.BindingMapKey]uint8) (string, glsl.TranslationInfo, error) {
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

// computeBindingMap computes per-type sequential binding indices for all bind group
// layouts in a pipeline layout. This follows the Rust wgpu-hal pattern from
// wgpu-hal/src/gles/device.rs:1154-1221 where five resource type counters
// (samplers, textures, images, uniform buffers, storage buffers) are incremented
// sequentially across all groups, producing flat GL slot indices.
//
// Returns:
//   - bindingMap: maps (group, binding) to GL slot for naga GLSL writer
//   - groupInfos: per-group BindingToSlot tables for runtime SetBindGroup
func computeBindingMap(layouts []*BindGroupLayout) (map[glsl.BindingMapKey]uint8, []BindGroupLayoutInfo) {
	var (
		numSamplers       uint8
		numTextures       uint8
		numImages         uint8
		numUniformBuffers uint8
		numStorageBuffers uint8
	)

	bindingMap := make(map[glsl.BindingMapKey]uint8)
	groupInfos := make([]BindGroupLayoutInfo, len(layouts))

	for groupIdx, bgl := range layouts {
		if bgl == nil {
			continue
		}
		entries := bgl.entries

		// Find max binding number to size the BindingToSlot table.
		maxBinding := uint32(0)
		for _, entry := range entries {
			if entry.Binding > maxBinding {
				maxBinding = entry.Binding
			}
		}

		bindingToSlot := make([]uint8, maxBinding+1)
		for i := range bindingToSlot {
			bindingToSlot[i] = 0xFF // unused
		}

		for _, entry := range entries {
			var counter *uint8
			switch classifyBindGroupEntry(entry) {
			case bindingClassSampler:
				counter = &numSamplers
			case bindingClassTexture:
				counter = &numTextures
			case bindingClassImage:
				counter = &numImages
			case bindingClassUniformBuffer:
				counter = &numUniformBuffers
			case bindingClassStorageBuffer:
				counter = &numStorageBuffers
			default:
				continue
			}

			slot := *counter
			bindingToSlot[entry.Binding] = slot
			bindingMap[glsl.BindingMapKey{
				Group:   uint32(groupIdx),
				Binding: entry.Binding,
			}] = slot
			*counter++
		}

		groupInfos[groupIdx] = BindGroupLayoutInfo{BindingToSlot: bindingToSlot}
	}

	return bindingMap, groupInfos
}

// bindingClass represents the GL resource type for a binding entry.
type bindingClass uint8

const (
	bindingClassUnknown       bindingClass = iota
	bindingClassSampler                    // GL sampler objects
	bindingClassTexture                    // GL texture units (sampled textures)
	bindingClassImage                      // GL image units (storage textures)
	bindingClassUniformBuffer              // GL uniform buffer binding points
	bindingClassStorageBuffer              // GL shader storage buffer binding points
)

// classifyBindGroupEntry determines the GL resource type for a bind group layout entry.
// Matches the Rust wgpu-hal classification in device.rs:1169-1193.
func classifyBindGroupEntry(entry gputypes.BindGroupLayoutEntry) bindingClass {
	switch {
	case entry.Sampler != nil:
		return bindingClassSampler
	case entry.Texture != nil:
		return bindingClassTexture
	case entry.StorageTexture != nil:
		return bindingClassImage
	case entry.Buffer != nil:
		switch entry.Buffer.Type {
		case gputypes.BufferBindingTypeUniform:
			return bindingClassUniformBuffer
		case gputypes.BufferBindingTypeStorage, gputypes.BufferBindingTypeReadOnlyStorage:
			return bindingClassStorageBuffer
		default:
			// Default buffer type treated as uniform buffer.
			return bindingClassUniformBuffer
		}
	default:
		return bindingClassUnknown
	}
}
