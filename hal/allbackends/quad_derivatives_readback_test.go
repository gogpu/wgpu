// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The quad tests check that fragments run in 2x2 quads aligned to even
// framebuffer coordinates: derivatives are differences between the lanes of a
// quad (WGSL "Derivative Built-in Functions"), and implicit-LOD sampling
// takes its level of detail from those differences (index.bs "texture
// sampling", Vulkan "Scale Factor Operation"). The software backend returned
// 0 for every derivative and sampled level 0 for every implicit-LOD sample.

// derivativeShader draws the whole colorRigSize target with two triangles
// whose shared edge is the target's diagonal, so the quads on the diagonal
// are covered partly by each triangle and need helper lanes. uv runs from 0
// to 1 across the target, with y up as in NDC. body is the fragment return
// expression.
func derivativeShader(body string) string {
	return fmt.Sprintf(`
struct VOut {
    @builtin(position) pos: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0));
    return VOut(vec4<f32>(p[i], 0.0, 1.0), p[i] * 0.5 + vec2<f32>(0.5, 0.5));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    let x = in.pos.x;
    let y = in.pos.y;
    return %s;
}
`, body)
}

// derivativeTolerance bounds the difference from the exact value. The
// quadratic cases are exact in float32 (small integers); the uv cases depend
// on the backend's varying interpolation, which is exact to well under 1e-4
// on a 4-pixel target.
const derivativeTolerance = 1e-4

// TestQuadDerivatives reads back derivatives of quadratic and linear
// functions of the fragment position and of an interpolated varying.
//
// A fine derivative of x*x is the difference between the two lanes of the
// pixel's quad row: (xq+1.5)^2 - (xq+0.5)^2 = 2*xq + 2 with xq the even x of
// the quad. It differs between neighboring quads, which proves both the lane
// differences and the even alignment of the quads. Coarse and plain
// derivatives are checked on linear functions only: WGSL lets an
// implementation compute them fine, so only a linear function has one
// portable value.
func TestQuadDerivatives(t *testing.T) {
	cases := []struct {
		name string
		body string
		want func(x, y int) [4]float64
	}{
		{
			name: "fine",
			body: "vec4<f32>(dpdxFine(x * x), dpdyFine(y * y), fwidthFine(x * x + y * y), dpdxFine(in.uv.x))",
			want: func(x, y int) [4]float64 {
				dx, dy := float64(2*(x&^1)+2), float64(2*(y&^1)+2)
				return [4]float64{dx, dy, dx + dy, 1.0 / colorRigSize}
			},
		},
		{
			name: "coarse",
			body: "vec4<f32>(dpdxCoarse(x), dpdyCoarse(3.0 * y), fwidthCoarse(3.0 * x + 5.0 * y), dpdyCoarse(in.uv.y))",
			want: func(int, int) [4]float64 { return [4]float64{1, 3, 8, -1.0 / colorRigSize} },
		},
		{
			name: "plain",
			body: "vec4<f32>(dpdx(2.0 * x), dpdy(in.uv.y), fwidth(3.0 * x + 5.0 * y), fwidth(in.uv.x - in.uv.y))",
			want: func(int, int) [4]float64 { return [4]float64{2, -1.0 / colorRigSize, 8, 2.0 / colorRigSize} },
		},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				tex, view := newColorTexture(t, device, gputypes.TextureFormatRGBA32Float, nil,
					wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
				colorPass{
					view:  view,
					wgsl:  derivativeShader(c.body),
					state: gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA32Float, WriteMask: gputypes.ColorWriteMaskAll},
				}.run(t, device)
				texels := readColorTexels(t, device, tex, 16)
				for y := range colorRigSize {
					for x := range colorRigSize {
						got := decodeRGBA32F(texels[y*colorRigSize+x])
						want := c.want(x, y)
						for ch := range 4 {
							if math.Abs(got[ch]-want[ch]) > derivativeTolerance {
								t.Errorf("pixel (%d, %d) channel %d = %g, want %g (±%g); pixel %v", x, y, ch, got[ch], want[ch],
									derivativeTolerance, got)
							}
						}
					}
				}
			})
		}
	})
}

func decodeRGBA32F(b []byte) [4]float64 {
	var v [4]float64
	for i := range v {
		v[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:])))
	}
	return v
}

// implicitLodShader samples newLodChainView's 8x8 chain with implicit level
// of detail. The 64-pixel-wide target is split into four 16-pixel columns;
// column c scales the texture coordinate so that one pixel steps 2^c texels
// in x and y, which is level of detail c. The column width is even, so every
// quad lies in one column. sample is the WGSL sampling call on uv.
func implicitLodShader(sample string) string {
	return fmt.Sprintf(`
@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var s: sampler;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    return VOut(vec4<f32>(quad[v %% 6u], 0.0, 1.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    let scale = exp2(floor(in.pos.x / 16.0));
    let uv = in.pos.xy * (scale / 8.0);
    return %s;
}
`, sample)
}

// TestImplicitLodSampling samples a mip chain whose levels have distinct
// colors with textureSample and textureSampleBias and checks which level each
// column read. A nearest mipmap filter rounds the level of detail, so the
// exact power-of-two scales select whole levels.
func TestImplicitLodSampling(t *testing.T) {
	cases := []struct {
		name   string
		sample string
		want   [4]int // the mip level read in column 0, 1, 2, 3
	}{
		{"textureSample", "textureSample(t, s, uv)", [4]int{0, 1, 2, 3}},
		// Bias adds to the derivative level of detail; level 4 does not
		// exist, so the last column clamps to level 3.
		{"textureSampleBias", "textureSampleBias(t, s, uv, 1.0)", [4]int{1, 2, 3, 3}},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		view := newLodChainView(t, device)
		frag := wgpu.ShaderStageFragment
		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: frag, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
			}},
			gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		)
		sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
			MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest,
			MipmapFilter: gputypes.FilterModeNearest,
		})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		t.Cleanup(sampler.Release)
		group := newBindGroup(t, device, bgl,
			wgpu.BindGroupEntry{Binding: 0, TextureView: view},
			wgpu.BindGroupEntry{Binding: 1, Sampler: sampler},
		)
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				pipeline := newRebindRenderPipeline(t, device, implicitLodShader(c.sample), bgl)
				px := renderDynamicTarget(t, device, func(pass *wgpu.RenderPassEncoder) {
					pass.SetPipeline(pipeline)
					pass.SetBindGroup(0, group, nil)
					pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
				})
				row := featureTargetHeight / 2 * featureTargetWidth
				for col, level := range c.want {
					got := px[row+col*16+8]
					if want := lodLevelWord(level); got != want {
						t.Errorf("column %d: pixel %#08x (level %d), want %#08x (level %d)", col, got, lodLevelOf(got), want, level)
					}
				}
			})
		}
	})
}
