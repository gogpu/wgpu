// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
)

// Tests for the image instructions ADR-078 step 6 executes, compiled from
// WGSL by naga: OpImageGather (textureGather), OpImageDrefGather
// (textureGatherCompare) and OpImageSampleDrefExplicitLod
// (textureSampleCompareLevel). The implicit-LOD form needs a fragment
// shader; hal/allbackends reads it back against Vulkan and DX12.

// gatherOut turns a gathered vec4<f32> of unorm texels back into bytes.
const gatherOut = `fn put(i: u32, g: vec4<f32>) {
	dst[i] = u32(g.x * 255.0 + 0.5); dst[i + 1u] = u32(g.y * 255.0 + 0.5);
	dst[i + 2u] = u32(g.z * 255.0 + 0.5); dst[i + 3u] = u32(g.w * 255.0 + 0.5);
}
`

// TestTextureGather reads a 4x4 grid texture (texel (x, y) is (10x, 10y,
// x+y, 255)). At uv (0.5, 0.5) the footprint is x0=1, x1=2, y0=1, y1=2, and
// the lanes are the texels (x0, y1), (x1, y1), (x1, y0), (x0, y0).
func TestTextureGather(t *testing.T) {
	sampler := func(wrap uint32) *Sampler {
		return &Sampler{WrapU: wrap, WrapV: wrap, LodMaxClamp: LodClampNone}
	}
	checkTypedCases(t, []typedCase{
		{
			name: "components",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	let uv = vec2<f32>(0.5, 0.5);
	put(0u, textureGather(0, tex, smp, uv));
	put(4u, textureGather(1, tex, smp, uv));
	put(8u, textureGather(2, tex, smp, uv));
	put(12u, textureGather(3, tex, smp, uv));
}`,
			dstLen: 16, tex: gridTexture(4, 4), smp: sampler(WrapClampToEdge),
			want: []uint32{10, 20, 20, 10, 20, 20, 10, 10, 3, 4, 3, 2, 255, 255, 255, 255},
		},
		{
			// At uv 0 the footprint is x0=-1, x1=0: repeat wraps -1 to 3,
			// clamp-to-edge and mirrored repeat both read texel 0.
			name: "repeat",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	put(0u, textureGather(2, tex, smp, vec2<f32>(0.0, 0.0)));
}`,
			dstLen: 4, tex: gridTexture(4, 4), smp: sampler(WrapRepeat), want: []uint32{3, 0, 3, 6},
		},
		{
			name: "clamp_to_edge",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	put(0u, textureGather(2, tex, smp, vec2<f32>(0.0, 0.0)));
	put(4u, textureGather(2, tex, smp, vec2<f32>(1.0, 1.0)));
}`,
			dstLen: 8, tex: gridTexture(4, 4), smp: sampler(WrapClampToEdge), want: []uint32{0, 0, 0, 0, 6, 6, 6, 6},
		},
		{
			// At uv 1 the footprint is x0=3, x1=4: mirrored repeat reflects 4
			// back to 3, repeat wraps it to 0.
			name: "mirrored_repeat",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	put(0u, textureGather(2, tex, smp, vec2<f32>(0.0, 0.0)));
	put(4u, textureGather(2, tex, smp, vec2<f32>(1.0, 1.0)));
}`,
			dstLen: 8, tex: gridTexture(4, 4), smp: sampler(WrapMirroredRepeat), want: []uint32{0, 0, 0, 0, 6, 6, 6, 6},
		},
		{
			name: "repeat_at_one",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	put(0u, textureGather(2, tex, smp, vec2<f32>(1.0, 1.0)));
}`,
			dstLen: 4, tex: gridTexture(4, 4), smp: sampler(WrapRepeat), want: []uint32{3, 0, 3, 6},
		},
		{
			// The offset moves the footprint by whole texels: x0=2, x1=3,
			// y0=0, y1=1.
			name: "offset",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	put(0u, textureGather(0, tex, smp, vec2<f32>(0.5, 0.5), vec2<i32>(1, -1)));
	put(4u, textureGather(1, tex, smp, vec2<f32>(0.5, 0.5), vec2<i32>(1, -1)));
}`,
			dstLen: 8, tex: gridTexture(4, 4), smp: sampler(WrapClampToEdge),
			want: []uint32{20, 30, 30, 20, 10, 10, 0, 0},
		},
		{
			// Gather reads the base level whatever the sampler's LOD clamp.
			name: "base_level_only",
			wgsl: typedHeaderSampled + gatherOut + `@compute @workgroup_size(1) fn main() {
	put(0u, textureGather(0, tex, smp, vec2<f32>(0.5, 0.5)));
}`,
			dstLen: 4, tex: gridTexture(4, 4),
			smp:  &Sampler{WrapU: WrapClampToEdge, WrapV: WrapClampToEdge, LodMinClamp: 1, LodMaxClamp: LodClampNone},
			want: []uint32{10, 20, 20, 10},
		},
	})
}

// TestTextureGatherUint gathers from an integer texture: the result is a
// vec4<u32> of the stored values.
func TestTextureGatherUint(t *testing.T) {
	tex := gridTexture(4, 4)
	tex.Format = uint32(gputypes.TextureFormatRGBA8Uint)
	tex.BytesPerPixel = 4
	checkTypedCases(t, []typedCase{{
		name: "rgba8uint",
		wgsl: typedHeader + `@group(0) @binding(2) var tex: texture_2d<u32>;
@group(0) @binding(3) var smp: sampler;
@compute @workgroup_size(1) fn main() {
	let g = textureGather(0, tex, smp, vec2<f32>(0.5, 0.5));
	let h = textureGather(2, tex, smp, vec2<f32>(0.5, 0.5));
	dst[0] = g.x; dst[1] = g.y; dst[2] = g.z; dst[3] = g.w;
	dst[4] = h.x; dst[5] = h.y; dst[6] = h.z; dst[7] = h.w;
}`,
		dstLen: 8, tex: tex, smp: &Sampler{WrapU: WrapClampToEdge, WrapV: WrapClampToEdge},
		want: []uint32{10, 20, 20, 10, 3, 4, 3, 2},
	}})
}

// depthGrid is a 4x4 Depth32Float view whose texel (x, y) holds
// (x + 4y + 0.5) / 16.
func depthGrid() *Texture2D {
	data := make([]byte, 0, 4*16)
	for k := range 16 {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits((float32(k)+0.5)/16))
	}
	return &Texture2D{
		Width: 4, Height: 4, Data: data, BytesPerPixel: 4,
		Format: uint32(gputypes.TextureFormatDepth32Float),
	}
}

const depthHeader = typedHeader + `@group(0) @binding(2) var tex: texture_depth_2d;
@group(0) @binding(3) var smp: sampler_comparison;
`

// TestTextureDepthCompare covers textureGatherCompare and
// textureSampleCompareLevel on depthGrid. At uv (0.5, 0.5) the footprint
// texels are (1, 2) = 0.59375, (2, 2) = 0.65625, (2, 1) = 0.40625 and
// (1, 1) = 0.34375.
func TestTextureDepthCompare(t *testing.T) {
	cmp := func(fn gputypes.CompareFunction, filter uint32) *Sampler {
		return &Sampler{
			MinFilter: filter, MagFilter: filter, WrapU: WrapClampToEdge, WrapV: WrapClampToEdge,
			LodMaxClamp: LodClampNone, Compare: fn,
		}
	}
	gatherCompare := depthHeader + `@compute @workgroup_size(1) fn main() {
	let g = textureGatherCompare(tex, smp, vec2<f32>(0.5, 0.5), bitcast<f32>(src[0]));
	dst[0] = u32(g.x); dst[1] = u32(g.y); dst[2] = u32(g.z); dst[3] = u32(g.w);
}`
	compareLevel := depthHeader + `@compute @workgroup_size(1) fn main() {
	let uv = bitcast<vec2<f32>>(vec2<u32>(src[1], src[2]));
	dst[0] = u32(textureSampleCompareLevel(tex, smp, uv, bitcast<f32>(src[0])) * 1000.0 + 0.5);
}`
	checkTypedCases(t, []typedCase{
		{
			name: "gather_less_equal", wgsl: gatherCompare, src: []uint32{f32bits(0.4)},
			dstLen: 4, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionLessEqual, FilterNearest),
			want: []uint32{1, 1, 1, 0},
		},
		{
			name: "gather_greater", wgsl: gatherCompare, src: []uint32{f32bits(0.4)},
			dstLen: 4, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionGreater, FilterLinear),
			want: []uint32{0, 0, 0, 1},
		},
		{
			// The gather of a reference equal to one texel's depth.
			name: "gather_equal", wgsl: gatherCompare, src: []uint32{f32bits(0.40625)},
			dstLen: 4, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionEqual, FilterNearest),
			want: []uint32{0, 0, 1, 0},
		},
		{
			// Linear filtering is PCF: the passing fraction of the footprint.
			name: "level_linear_pcf", wgsl: compareLevel, src: []uint32{f32bits(0.4), f32bits(0.5), f32bits(0.5)},
			dstLen: 1, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionLessEqual, FilterLinear),
			want: []uint32{750},
		},
		{
			// A quarter texel right of the footprint's center: fx = 0.75,
			// fy = 0.5. Passing are (1, 2), (2, 2) and (2, 1): 1 - 0.25*0.5.
			name: "level_linear_weights", wgsl: compareLevel, src: []uint32{f32bits(0.4), f32bits(0.5625), f32bits(0.5)},
			dstLen: 1, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionLessEqual, FilterLinear),
			want: []uint32{875},
		},
		{
			// Nearest at the center of texel (2, 1), depth 0.40625.
			name: "level_nearest_fail", wgsl: compareLevel, src: []uint32{f32bits(0.5), f32bits(0.625), f32bits(0.375)},
			dstLen: 1, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionLessEqual, FilterNearest),
			want: []uint32{0},
		},
		{
			name: "level_nearest_pass", wgsl: compareLevel, src: []uint32{f32bits(0.40625), f32bits(0.625), f32bits(0.375)},
			dstLen: 1, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionEqual, FilterNearest),
			want: []uint32{1000},
		},
		{
			// A sampler with no compare function compares as Never.
			name: "level_undefined_is_never", wgsl: compareLevel, src: []uint32{f32bits(0.4), f32bits(0.5), f32bits(0.5)},
			dstLen: 1, tex: depthGrid(), smp: cmp(gputypes.CompareFunctionUndefined, FilterLinear),
			want: []uint32{0},
		},
	})
}

// TestDepthCompareFunctions checks every compare function, the reference
// on the left (WebGPU: the provided value is compared with the sampled one),
// and the reference clamp of a unorm depth format.
func TestDepthCompareFunctions(t *testing.T) {
	d32, d16 := uint32(gputypes.TextureFormatDepth32Float), uint32(gputypes.TextureFormatDepth16Unorm)
	cases := []struct {
		fn       gputypes.CompareFunction
		format   uint32
		ref, d   float32
		wantPass bool
	}{
		{gputypes.CompareFunctionUndefined, d32, 0.5, 0.5, false},
		{gputypes.CompareFunctionNever, d32, 0.5, 0.5, false},
		{gputypes.CompareFunctionLess, d32, 0.25, 0.5, true},
		{gputypes.CompareFunctionLess, d32, 0.5, 0.5, false},
		{gputypes.CompareFunctionEqual, d32, 0.5, 0.5, true},
		{gputypes.CompareFunctionEqual, d32, 0.25, 0.5, false},
		{gputypes.CompareFunctionLessEqual, d32, 0.5, 0.5, true},
		{gputypes.CompareFunctionLessEqual, d32, 0.75, 0.5, false},
		{gputypes.CompareFunctionGreater, d32, 0.75, 0.5, true},
		{gputypes.CompareFunctionGreater, d32, 0.5, 0.5, false},
		{gputypes.CompareFunctionNotEqual, d32, 0.25, 0.5, true},
		{gputypes.CompareFunctionNotEqual, d32, 0.5, 0.5, false},
		{gputypes.CompareFunctionGreaterEqual, d32, 0.5, 0.5, true},
		{gputypes.CompareFunctionGreaterEqual, d32, 0.25, 0.5, false},
		{gputypes.CompareFunctionAlways, d32, 0.25, 0.5, true},
		// Depth16Unorm clamps the reference to [0, 1]; Depth32Float does not.
		{gputypes.CompareFunctionLessEqual, d16, 1.5, 1, true},
		{gputypes.CompareFunctionLessEqual, d32, 1.5, 1, false},
		{gputypes.CompareFunctionEqual, d16, -0.5, 0, true},
		{gputypes.CompareFunctionEqual, d32, -0.5, 0, false},
	}
	for _, c := range cases {
		dc := depthCompare{ref: c.ref, fn: c.fn}
		want := float32(0)
		if c.wantPass {
			want = 1
		}
		if got := dc.test(c.format, c.d); got != want {
			t.Errorf("compare %v format %d: ref %v vs %v = %v, want %v", c.fn, c.format, c.ref, c.d, got, want)
		}
	}
}

func TestWrapTexel(t *testing.T) {
	cases := []struct {
		i, n int
		wrap uint32
		want int
	}{
		{-1, 4, WrapRepeat, 3},
		{4, 4, WrapRepeat, 0},
		{-5, 4, WrapRepeat, 3},
		{9, 4, WrapRepeat, 1},
		{-1, 4, WrapClampToEdge, 0},
		{7, 4, WrapClampToEdge, 3},
		{-1, 4, WrapMirroredRepeat, 0},
		{-2, 4, WrapMirroredRepeat, 1},
		{4, 4, WrapMirroredRepeat, 3},
		{5, 4, WrapMirroredRepeat, 2},
		{8, 4, WrapMirroredRepeat, 0},
		{-9, 4, WrapMirroredRepeat, 0},
		{0, 0, WrapRepeat, 0},
		{3, 1, WrapMirroredRepeat, 0},
	}
	for _, c := range cases {
		if got := wrapTexel(c.i, c.n, c.wrap); got != c.want {
			t.Errorf("wrapTexel(%d, %d, %d) = %d, want %d", c.i, c.n, c.wrap, got, c.want)
		}
	}
}
