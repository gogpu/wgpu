// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

// stageConstantsShader takes the green channel from a vertex stage override
// (passed on as a varying, equal at every vertex so interpolation keeps it
// exact), and red and blue from fragment stage overrides: a bool and an
// override keyed by its numeric @id.
const stageConstantsShader = `
override vg: f32 = 1.0;
override fr: bool = false;
@id(1000) override fb: f32 = 0.0;

struct VOut {
    @builtin(position) pos: vec4<f32>,
    @location(0) g: f32,
}

@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
    var out: VOut;
    out.pos = vec4<f32>(p[i], 0.5, 1.0);
    out.g = vg;
    return out;
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    return vec4<f32>(select(0.0, 1.0, fr), in.g, fb, 1.0);
}
`

// TestRenderStageConstants renders with the pipeline constants of the
// vertex and the fragment stage (WebGPU GPUProgrammableStage.constants) on
// every backend and checks a pixel. Each stage is specialized with its own
// map: a key that names the other stage's override changes nothing.
func TestRenderStageConstants(t *testing.T) {
	type consts = map[string]float64
	tests := []struct {
		name     string
		vertex   consts
		fragment consts
		want     [4]byte
	}{
		{name: "defaults", want: [4]byte{0, 255, 0, 255}},
		{name: "vertex set", vertex: consts{"vg": 0}, want: [4]byte{0, 0, 0, 255}},
		{name: "fragment set", fragment: consts{"fr": 1, "1000": 1}, want: [4]byte{255, 255, 255, 255}},
		{name: "both set", vertex: consts{"vg": 0}, fragment: consts{"1000": 1}, want: [4]byte{0, 0, 255, 255}},
		{name: "vertex key in fragment", fragment: consts{"vg": 0}, want: [4]byte{0, 255, 0, 255}},
		{name: "fragment key in vertex", vertex: consts{"fr": 1, "1000": 1}, want: [4]byte{0, 255, 0, 255}},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if px := renderAndReadConstants(t, device, stageConstantsShader, tt.vertex, tt.fragment); px != tt.want {
					t.Errorf("pixel (0,0) = %v, want %v", px, tt.want)
				}
			})
		}
	})
}
