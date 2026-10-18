// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
)

const glslOverrideShader = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 16>;

override wg: u32 = 4u;
@id(7) override scale: u32 = 2u;

@compute @workgroup_size(wg, 2)
fn main(@builtin(local_invocation_index) i: u32) {
    out[i] = scale * 3u;
}

@compute @workgroup_size(1)
fn other() {
    out[0] = 1u;
}
`

// TestCompileWGSLToGLSLOverrides checks the GLSL that the GLES backend writes
// for a shader with overrides, without a GL context: the workgroup size and
// the arithmetic take the override values, from defaults or from pipeline
// constants (BUG-OVERRIDE-WORKGROUP-001; with naga v0.19.0 the GLSL had
// local_size_x = 1).
func TestCompileWGSLToGLSLOverrides(t *testing.T) {
	sm, err := newShaderModule(&hal.ShaderModuleDescriptor{Source: hal.ShaderSource{WGSL: glslOverrideShader}}, nil)
	if err != nil {
		t.Fatalf("newShaderModule: %v", err)
	}
	for _, tc := range []struct {
		name      string
		constants map[string]float64
		want      []string
	}{
		{name: "defaults", want: []string{"local_size_x = 4, local_size_y = 2", "6u"}},
		{name: "constants", constants: map[string]float64{"wg": 3, "7": 5}, want: []string{"local_size_x = 3, local_size_y = 2", "15u"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, _, err := compileWGSLToGLSL(glsl.Version430, sm, ir.StageCompute, "main", tc.constants, nil)
			if err != nil {
				t.Fatalf("compileWGSLToGLSL: %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(src, w) {
					t.Errorf("GLSL has no %q:\n%s", w, src)
				}
			}
		})
	}

	_, _, err = compileWGSLToGLSL(glsl.Version430, sm, ir.StageCompute, "main", map[string]float64{"nope": 1}, nil)
	if !errors.Is(err, naga.ErrOverrideNotFound) {
		t.Errorf("unknown constant: error %v, want naga.ErrOverrideNotFound", err)
	}
	_, _, err = compileWGSLToGLSL(glsl.Version430, sm, ir.StageCompute, "missing", nil, nil)
	if !errors.Is(err, naga.ErrEntryPointNotFound) {
		t.Errorf("unknown entry point: error %v, want naga.ErrEntryPointNotFound", err)
	}
}
