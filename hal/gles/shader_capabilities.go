// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import "github.com/gogpu/gputypes"

// drawParameters reports Rust wgpu-hal's FULLY_FEATURED_INSTANCING (gles/
// adapter.rs): desktop GL 4.2+ with GL_ARB_shader_draw_parameters. Shaders
// are then compiled with glsl.Options.DrawParameters, as Rust sets
// WriterFlags::DRAW_PARAMETERS from it (gles/device.rs).
func drawParameters(exts map[string]bool, glMajor, glMinor int, isES bool) bool {
	return !isES && (glMajor > 4 || glMajor == 4 && glMinor >= 2) &&
		hasExtension(exts, "GL_ARB_shader_draw_parameters")
}

// shaderCapabilityFeatures returns the shader capability features (the naga
// capabilities a device grants) a GL context offers, as Rust wgpu-hal
// detects them (gles/adapter.rs).
//
// ClipDistances is not offered: GL clips only with GL_CLIP_DISTANCEi enabled
// per pipeline, which Rust drives from naga's reflected clip distance count;
// naga's GLSL backend does not report it yet (requested from naga,
// P1-API-008).
func shaderCapabilityFeatures(exts map[string]bool, glMajor, glMinor int, isES bool) gputypes.Features {
	var result gputypes.Features
	set := func(f gputypes.Feature, on bool) {
		if on {
			result.Insert(f)
		}
	}
	set(gputypes.FeaturePrimitiveIndex,
		glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 2}, [2]int{3, 2}) ||
			hasExtension(exts, "GL_OES_geometry_shader", "GL_ARB_geometry_shader4"))
	set(gputypes.FeatureShaderEarlyDepthTest,
		glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 2}) ||
			hasExtension(exts, "GL_ARB_shader_image_load_store"))
	// naga writes GLSL 4.50 at most, so draw_index needs gl_DrawIDARB from
	// GL_ARB_shader_draw_parameters even on GL 4.6; GLSL ES has none.
	set(gputypes.FeatureShaderDrawIndex, drawParameters(exts, glMajor, glMinor, isES))
	return result
}
