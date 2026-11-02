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
		name           string
		major, minor   int
		es             bool
		fullInstancing bool
		exts           []string
		want           gputypes.Features
	}{
		{"GL 3.3", 3, 3, false, false, nil, prim},
		{"GL 4.1 (Mesa d3d12)", 4, 1, false, false, nil, prim},
		{"GL 4.1 with image load/store", 4, 1, false, false, []string{"GL_ARB_shader_image_load_store"}, prim | early},
		{"GL 4.2", 4, 2, false, false, nil, prim | early},
		{"GL 4.5", 4, 5, false, false, nil, prim | early},
		{"GL 4.5 with full instancing", 4, 5, false, true, nil, prim | early | draw},
		{"GL 4.6", 4, 6, false, false, nil, prim | early | draw},
		{"GL 4.6 with full instancing", 4, 6, false, true, nil, prim | early | draw},
		{"ES 3.0", 3, 0, true, false, nil, 0},
		{"ES 3.0 with OES_geometry_shader", 3, 0, true, false, []string{"GL_OES_geometry_shader"}, prim},
		{"ES 3.0 with bare OES_geometry_shader", 3, 0, true, false, []string{"OES_geometry_shader"}, prim},
		{"ES 3.0 with ARB_geometry_shader4", 3, 0, true, false, []string{"GL_ARB_geometry_shader4"}, prim},
		{"ES 3.1", 3, 1, true, false, nil, early},
		{"ES 3.2", 3, 2, true, false, nil, prim | early},
		{"ES 3.2 never has draw index", 3, 2, true, true, nil, prim | early},
		{"desktop clip distance extension", 4, 6, false, false, []string{"GL_EXT_clip_cull_distance"}, prim | early | draw},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exts := map[string]bool{}
			for _, e := range tt.exts {
				exts[e] = true
			}
			got := shaderCapabilityFeatures(exts, tt.major, tt.minor, tt.es, tt.fullInstancing)
			if got != tt.want {
				t.Errorf("shaderCapabilityFeatures() = %#x, want %#x", uint64(got), uint64(tt.want))
			}
			if got.Contains(gputypes.FeatureClipDistances) {
				t.Error("ClipDistances offered: the backend enables no GL_CLIP_DISTANCEi")
			}
		})
	}
}

// TestFullyFeaturedInstancing checks Rust wgpu-hal's FULLY_FEATURED_INSTANCING
// condition (gles/adapter.rs): desktop GL 4.2+ with
// GL_ARB_shader_draw_parameters.
func TestFullyFeaturedInstancing(t *testing.T) {
	params := []string{"GL_ARB_shader_draw_parameters"}
	tests := []struct {
		name         string
		major, minor int
		es           bool
		exts         []string
		want         bool
	}{
		{"GL 4.6 with draw parameters", 4, 6, false, params, true},
		{"GL 4.2 with draw parameters", 4, 2, false, params, true},
		{"GL 4.1 with draw parameters", 4, 1, false, params, false},
		{"GL 4.6 without draw parameters", 4, 6, false, nil, false},
		{"ES 3.2 with draw parameters", 3, 2, true, params, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exts := map[string]bool{}
			for _, e := range tt.exts {
				exts[e] = true
			}
			if got := fullyFeaturedInstancing(exts, tt.major, tt.minor, tt.es); got != tt.want {
				t.Errorf("fullyFeaturedInstancing() = %v, want %v", got, tt.want)
			}
		})
	}
}
