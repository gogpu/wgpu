// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/naga/glsl"
)

// =============================================================================
// GLSLVersionToNaga Tests — version conversion from int → glsl.Version
// =============================================================================

func TestGLSLVersionToNaga(t *testing.T) {
	tests := []struct {
		name        string
		glslVersion int
		isES        bool
		want        glsl.Version
	}{
		// Desktop GLSL versions
		{
			name:        "desktop/330",
			glslVersion: 330,
			isES:        false,
			want:        glsl.Version{Major: 3, Minor: 30, ES: false},
		},
		{
			name:        "desktop/400",
			glslVersion: 400,
			isES:        false,
			want:        glsl.Version{Major: 4, Minor: 0, ES: false},
		},
		{
			name:        "desktop/410",
			glslVersion: 410,
			isES:        false,
			want:        glsl.Version{Major: 4, Minor: 10, ES: false},
		},
		{
			name:        "desktop/420",
			glslVersion: 420,
			isES:        false,
			want:        glsl.Version{Major: 4, Minor: 20, ES: false},
		},
		{
			name:        "desktop/430",
			glslVersion: 430,
			isES:        false,
			want:        glsl.Version{Major: 4, Minor: 30, ES: false},
		},
		{
			name:        "desktop/450",
			glslVersion: 450,
			isES:        false,
			want:        glsl.Version{Major: 4, Minor: 50, ES: false},
		},
		{
			name:        "desktop/460",
			glslVersion: 460,
			isES:        false,
			want:        glsl.Version{Major: 4, Minor: 60, ES: false},
		},

		// ES GLSL versions
		{
			name:        "es/300",
			glslVersion: 300,
			isES:        true,
			want:        glsl.Version{Major: 3, Minor: 0, ES: true},
		},
		{
			name:        "es/310",
			glslVersion: 310,
			isES:        true,
			want:        glsl.Version{Major: 3, Minor: 10, ES: true},
		},
		{
			name:        "es/320",
			glslVersion: 320,
			isES:        true,
			want:        glsl.Version{Major: 3, Minor: 20, ES: true},
		},

		// Zero fallback → safe minimums
		{
			name:        "zero/desktop_defaults_to_330",
			glslVersion: 0,
			isES:        false,
			want:        glsl.Version330,
		},
		{
			name:        "zero/es_defaults_to_es300",
			glslVersion: 0,
			isES:        true,
			want:        glsl.VersionES300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GLSLVersionToNaga(tt.glslVersion, tt.isES)
			if got != tt.want {
				t.Errorf("GLSLVersionToNaga(%d, es=%v) = %+v, want %+v",
					tt.glslVersion, tt.isES, got, tt.want)
			}
		})
	}
}

// =============================================================================
// SupportsExplicitLocations boundary tests — verify the 419/420 boundary
// =============================================================================

func TestGLSLVersionToNaga_SupportsExplicitLocations(t *testing.T) {
	tests := []struct {
		name         string
		glslVersion  int
		isES         bool
		wantBindings bool // whether layout(binding=N) would be emitted
	}{
		// Desktop boundary: 420 is the minimum
		{"desktop/330_no_bindings", 330, false, false},
		{"desktop/400_no_bindings", 400, false, false},
		{"desktop/410_no_bindings", 410, false, false},
		{"desktop/419_no_bindings", 419, false, false}, // just below boundary
		{"desktop/420_has_bindings", 420, false, true}, // exact boundary
		{"desktop/430_has_bindings", 430, false, true},
		{"desktop/450_has_bindings", 450, false, true},

		// ES boundary: 310 is the minimum
		{"es/300_no_bindings", 300, true, false},
		{"es/309_no_bindings", 309, true, false}, // just below boundary
		{"es/310_has_bindings", 310, true, true}, // exact boundary
		{"es/320_has_bindings", 320, true, true},

		// Zero fallbacks
		{"zero/desktop_no_bindings", 0, false, false}, // defaults to 330
		{"zero/es_no_bindings", 0, true, false},       // defaults to ES 300
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := GLSLVersionToNaga(tt.glslVersion, tt.isES)
			got := ver.SupportsExplicitLocations()
			if got != tt.wantBindings {
				t.Errorf("GLSLVersionToNaga(%d, es=%v).SupportsExplicitLocations() = %v, want %v",
					tt.glslVersion, tt.isES, got, tt.wantBindings)
			}
		})
	}
}

// =============================================================================
// shaderBindingLayout capability detection
// =============================================================================

func TestShaderBindingLayout(t *testing.T) {
	// Verify that shaderBindingLayout is true iff SupportsExplicitLocations is true.
	// This mirrors Rust wgpu-hal PrivateCapabilities::SHADER_BINDING_LAYOUT.
	tests := []struct {
		name        string
		glslVersion int
		isES        bool
		want        bool
	}{
		// Desktop GL
		{"GL_3.3/GLSL_330_no_layout", 330, false, false},
		{"GL_4.0/GLSL_400_no_layout", 400, false, false},
		{"GL_4.1/GLSL_410_no_layout", 410, false, false},
		{"GL_4.2/GLSL_420_has_layout", 420, false, true},
		{"GL_4.3/GLSL_430_has_layout", 430, false, true},
		{"GL_4.5/GLSL_450_has_layout", 450, false, true},
		{"GL_4.6/GLSL_460_has_layout", 460, false, true},

		// GLES
		{"GLES_3.0/GLSL_300_no_layout", 300, true, false},
		{"GLES_3.1/GLSL_310_has_layout", 310, true, true},
		{"GLES_3.2/GLSL_320_has_layout", 320, true, true},

		// Zero fallback
		{"zero_desktop_no_layout", 0, false, false},
		{"zero_es_no_layout", 0, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := GLSLVersionToNaga(tt.glslVersion, tt.isES)
			// shaderBindingLayout is set to ver.SupportsExplicitLocations() at
			// device Open time (adapter.go / adapter_linux.go).
			got := ver.SupportsExplicitLocations()
			if got != tt.want {
				t.Errorf("shaderBindingLayout for GLSL %d (ES=%v) = %v, want %v",
					tt.glslVersion, tt.isES, got, tt.want)
			}
		})
	}
}
