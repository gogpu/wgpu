// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package stencilblit

import (
	"fmt"
	"strings"

	"github.com/gogpu/gputypes"
)

// Entry points of the blit shader.
const (
	VertexEntryPoint = "vs_main"
	ClearEntryPoint  = "fs_clear"
)

// Bindings of the blit bind group: the R8Uint source view and the Params
// uniform.
const (
	SourceBinding = 0
	ParamsBinding = 1
)

// BitEntryPoint returns the fragment entry point of bit.
func BitEntryPoint(bit int) string {
	return fmt.Sprintf("fs_bit%d", bit)
}

// ShaderSource returns the WGSL of the blit for a source view of dim (2d or
// 2d-array): the full-screen triangle, the clear fragment entry point and one
// fragment entry point per stencil bit, each with its bit baked in as a
// constant.
//
// Dawn's shader (BlitBufferToDepthStencil.cpp, kBlitStencilShaderCommon) reads
// the bit from the instance index; see RecordLayer for why we bake it.
//
// The mip level is an i32: naga's GLSL writer passes a u32 level to
// texelFetch, textureSize and textureQueryLevels as it is, and GLSL takes an
// int there with no implicit conversion, so the GL driver rejects the shader.
func ShaderSource(dim gputypes.TextureViewDimension) (string, error) {
	var texType, load string
	switch dim {
	case gputypes.TextureViewDimension2D:
		texType = "texture_2d<u32>"
		load = "textureLoad(src_tex, coords, 0i)"
	case gputypes.TextureViewDimension2DArray:
		texType = "texture_2d_array<u32>"
		load = "textureLoad(src_tex, coords, params.layer, 0i)"
	default:
		return "", fmt.Errorf("stencilblit: source view dimension %v is neither 2d nor 2d-array", dim)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `struct Params {
    origin: vec2u,
    layer: u32,
}

@group(0) @binding(%d) var src_tex: %s;
@group(0) @binding(%d) var<uniform> params: Params;

@vertex
fn %s(@builtin(vertex_index) vertex_index: u32) -> @builtin(position) vec4f {
    var positions = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    return vec4f(positions[vertex_index], 0.0, 1.0);
}

// The clear pipeline only needs coverage: the stencil test writes the reference.
@fragment
fn %s() {}

fn source_stencil(position: vec4f) -> u32 {
    let coords = vec2u(position.xy) - params.origin;
    return %s.x;
}
`, SourceBinding, texType, ParamsBinding, VertexEntryPoint, ClearEntryPoint, load)

	for bit := range StencilBits {
		fmt.Fprintf(&b, `
const STENCIL_BIT_%d: u32 = %du;

@fragment
fn %s(@builtin(position) position: vec4f) {
    if ((source_stencil(position) & STENCIL_BIT_%d) == 0u) {
        discard;
    }
}
`, bit, uint32(1)<<uint(bit), BitEntryPoint(bit), bit)
	}
	return b.String(), nil
}
