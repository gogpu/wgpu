// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import "github.com/gogpu/gputypes"

// shaderCapabilityFeatures returns the shader capability features (the naga
// capabilities a device grants) a GL context offers, as Rust wgpu-hal
// detects them (gles/adapter.rs):
//   - PrimitiveIndex (gl_PrimitiveID in the fragment stage): GL 3.2 / ES 3.2,
//     OES_geometry_shader or GL_ARB_geometry_shader4;
//   - ShaderEarlyDepthTest (layout(early_fragment_tests)): GL 4.2 / ES 3.1 or
//     GL_ARB_shader_image_load_store;
//   - ShaderDrawIndex, which Rust does not offer on GLES: desktop GL 4.6
//     (GLSL 4.60 gl_DrawID), or fullInstancing, whose shaders are compiled
//     with naga's DrawParameters (gl_DrawIDARB). GLSL ES has no draw index.
//     A multi-draw counts its records only because MultiDrawIndirect is
//     offered just where glMultiDraw*Indirect issues them as one call.
//
// ClipDistances is not offered: GL clips only with GL_CLIP_DISTANCEi enabled
// per pipeline, which Rust drives from naga's reflected clip distance count;
// naga's GLSL backend does not report it yet (requested from naga,
// P1-API-008).
func shaderCapabilityFeatures(exts map[string]bool, glMajor, glMinor int, isES, fullInstancing bool) gputypes.Features {
	var result gputypes.Features
	set := func(f gputypes.Feature, on bool) {
		if on {
			result.Insert(f)
		}
	}
	// Rust checks the bare "OES_geometry_shader"; GL reports the GL_ name.
	set(gputypes.FeaturePrimitiveIndex,
		glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 2}, [2]int{3, 2}) ||
			hasExtension(exts, "OES_geometry_shader", "GL_OES_geometry_shader", "GL_ARB_geometry_shader4"))
	set(gputypes.FeatureShaderEarlyDepthTest,
		glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 2}) ||
			hasExtension(exts, "GL_ARB_shader_image_load_store"))
	set(gputypes.FeatureShaderDrawIndex, !isES &&
		(glVersionAtLeast(glMajor, glMinor, false, [2]int{99, 0}, [2]int{4, 6}) || fullInstancing))
	return result
}
