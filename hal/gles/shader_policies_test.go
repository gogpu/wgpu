// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
)

// TestGLSLBoundsCheckPolicies pins the policies to Rust wgpu-hal
// gles/device.rs:402-417: image loads are checked only on desktop GL 4.3+;
// naga refuses a checked image load on GLSL ES and below 4.30.
func TestGLSLBoundsCheckPolicies(t *testing.T) {
	for _, c := range []struct {
		version   glsl.Version
		imageLoad backend.BoundsCheckPolicy
	}{
		{glsl.VersionES300, backend.BoundsCheckUnchecked},
		{glsl.VersionES310, backend.BoundsCheckUnchecked},
		{glsl.VersionES320, backend.BoundsCheckUnchecked},
		{glsl.Version330, backend.BoundsCheckUnchecked},
		{glsl.Version420, backend.BoundsCheckUnchecked},
		{glsl.Version430, backend.BoundsCheckReadZeroSkipWrite},
		{glsl.Version460, backend.BoundsCheckReadZeroSkipWrite},
	} {
		want := backend.BoundsCheckPolicies{
			Index:        backend.BoundsCheckUnchecked,
			Buffer:       backend.BoundsCheckUnchecked,
			ImageLoad:    c.imageLoad,
			BindingArray: backend.BoundsCheckUnchecked,
		}
		if got := glslBoundsCheckPolicies(c.version); got != want {
			t.Errorf("%+v: %+v, want %+v", c.version, got, want)
		}
	}
}

// TestGLSLESCompilesTextureLoad compiles a textureLoad with a level for every
// GLSL ES version: with naga's zero policies (Restrict) it would be refused.
func TestGLSLESCompilesTextureLoad(t *testing.T) {
	const src = `
@group(0) @binding(0) var t: texture_2d<f32>;
@fragment
fn fs(@builtin(position) p: vec4<f32>) -> @location(0) vec4<f32> {
	return textureLoad(t, vec2<i32>(p.xy), 0);
}
`
	sm, err := newShaderModule(&hal.ShaderModuleDescriptor{Source: hal.ShaderSource{WGSL: src}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []glsl.Version{glsl.VersionES300, glsl.VersionES310, glsl.VersionES320, glsl.Version330, glsl.Version430} {
		if _, _, err := compileWGSLToGLSL(v, false, sm, ir.StageFragment, "fs", nil, nil); err != nil {
			t.Errorf("%+v: %v", v, err)
		}
	}
}
