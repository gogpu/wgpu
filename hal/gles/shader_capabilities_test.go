// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestShaderCapabilityFeatures checks the GL versions and extensions behind
// each shader capability feature, as Rust wgpu-hal detects them
// (gles/adapter.rs), and that ClipDistances is never offered.
func TestShaderCapabilityFeatures(t *testing.T) {
	prim := gputypes.Features(gputypes.FeaturePrimitiveIndex)
	early := gputypes.Features(gputypes.FeatureShaderEarlyDepthTest)
	draw := gputypes.Features(gputypes.FeatureShaderDrawIndex)
	tests := []struct {
		name         string
		major, minor int
		es           bool
		exts         []string
		want         gputypes.Features
	}{
		{"GL 3.3", 3, 3, false, nil, prim},
		{"GL 4.1 with image load/store", 4, 1, false, []string{"GL_ARB_shader_image_load_store"}, prim | early},
		{"GL 4.2", 4, 2, false, nil, prim | early},
		{"GL 4.6 without draw parameters", 4, 6, false, nil, prim | early},
		{"GL 4.6 with draw parameters", 4, 6, false, []string{"GL_ARB_shader_draw_parameters"}, prim | early | draw},
		{"GL 4.1 with draw parameters", 4, 1, false, []string{"GL_ARB_shader_draw_parameters"}, prim},
		{"ES 3.0", 3, 0, true, nil, 0},
		{"ES 3.0 with geometry shader", 3, 0, true, []string{"GL_OES_geometry_shader"}, prim},
		{"ES 3.1", 3, 1, true, nil, early},
		{"ES 3.2", 3, 2, true, nil, prim | early},
		{"ES 3.2 with draw parameters", 3, 2, true, []string{"GL_ARB_shader_draw_parameters"}, prim | early},
		{"desktop clip distance extension", 4, 6, false, []string{"GL_EXT_clip_cull_distance"}, prim | early},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exts := map[string]bool{}
			for _, e := range tt.exts {
				exts[e] = true
			}
			if got := shaderCapabilityFeatures(exts, tt.major, tt.minor, tt.es); got != tt.want {
				t.Errorf("shaderCapabilityFeatures() = %v, want %v", got, tt.want)
			}
			if got, want := fullyFeaturedInstancing(exts, tt.major, tt.minor, tt.es), tt.want.Contains(gputypes.FeatureShaderDrawIndex); got != want {
				t.Errorf("fullyFeaturedInstancing() = %v, want %v", got, want)
			}
		})
	}
}
