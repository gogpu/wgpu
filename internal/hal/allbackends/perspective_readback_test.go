// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The perspective tests draw a rectangle seen in perspective: its bottom
// edge at w = 1, its top edge at w = 4. Per the WebGPU spec ("Interpolation"
// under rasterization), a perspective varying is linear in clip space, a
// linear one is linear on the screen, the depth is linear on the screen, and
// the fragment's position.w is the interpolated 1/w. The software backend
// interpolated every varying linearly on the screen and gave every fragment
// w = 1 (P1-SW-037).

const perspectiveShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) uv: vec2f,
    @location(1) @interpolate(linear) uvLinear: vec2f,
}

struct FOut {
    @location(0) varyings: vec4f,
    @location(1) position: vec4f,
}

@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var smp: sampler;

// Instance 0 is the rectangle; instance 1, drawn after it, a full-screen
// plane at depth 0.5 whose varyings are all 5. Vertices 6 to 11 draw the
// rectangle with its top edge beyond the far plane (depth 1.2).
@vertex
fn vs_main(@builtin(vertex_index) i: u32, @builtin(instance_index) inst: u32) -> VOut {
    var idx = array<u32, 6>(0u, 1u, 2u, 0u, 2u, 3u);
    let k = idx[i % 6u];
    if inst == 1u {
        var plane = array<vec2f, 4>(vec2f(-1.0, -1.0), vec2f(1.0, -1.0), vec2f(1.0, 1.0), vec2f(-1.0, 1.0));
        return VOut(vec4f(plane[k], 0.5, 1.0), vec2f(5.0), vec2f(5.0));
    }
    var zTop = 3.2;
    if i >= 6u {
        zTop = 4.8;
    }
    var p = array<vec4f, 4>(
        vec4f(-0.9, -0.9, 0.2, 1.0),
        vec4f(0.9, -0.9, 0.2, 1.0),
        vec4f(0.9, 3.6, zTop, 4.0),
        vec4f(-0.9, 3.6, zTop, 4.0));
    var uv = array<vec2f, 4>(vec2f(0.0, 0.0), vec2f(1.0, 0.0), vec2f(1.0, 1.0), vec2f(0.0, 1.0));
    return VOut(p[k], uv[k], uv[k]);
}

@fragment
fn fs_main(in: VOut) -> FOut {
    let t = textureSampleLevel(tex, smp, in.uv, 0.0);
    return FOut(vec4f(in.uv, in.uvLinear), vec4f(in.pos.z, in.pos.w, t.r, t.g));
}
`

// perspectiveQuad holds the clip-space positions and uvs of vs_main's
// rectangle, perspectiveFarQuad those of instance 2, and
// perspectiveTriangles the two triangles it is drawn as. The rectangle is
// planar in (x, y, w): u = x/1.8 - 0.2083y + 0.3125w and z = y/3 + 0.5w
// (5y/9 + 0.7w for instance 2), so the uv and the depth are those of one
// plane in both triangles.
var (
	perspectiveQuad    = [4][4]float64{{-0.9, -0.9, 0.2, 1}, {0.9, -0.9, 0.2, 1}, {0.9, 3.6, 3.2, 4}, {-0.9, 3.6, 3.2, 4}}
	perspectiveFarQuad = [4][4]float64{{-0.9, -0.9, 0.2, 1}, {0.9, -0.9, 0.2, 1}, {0.9, 3.6, 4.8, 4}, {-0.9, 3.6, 4.8, 4}}
	perspectiveUV      = [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}

	perspectiveTriangles = [2][3]int{{0, 1, 2}, {0, 2, 3}}
)

const (
	// perspectiveSize is the target's width and height.
	perspectiveSize = 16

	// perspectiveVaryingTolerance bounds the error of an interpolated uv.
	// GPUs interpolate in float32 from barycentrics of limited precision:
	// 1e-3 is a few thousand float32 ulps of values below 1, and far below
	// the gap between perspective and screen-linear interpolation inside the
	// rectangle (up to 0.29).
	perspectiveVaryingTolerance = 1e-3

	// perspectiveTextureTolerance bounds the error of the filtered texel.
	// D3D12 and Vulkan hardware quantise the bilinear weights to 8 bits
	// (subTexelPrecisionBits): the result may be off by 1/256 of one texel
	// step, 1/3 here, plus the error of the uv it was sampled at.
	perspectiveTextureTolerance = 4e-3
)

// TestPerspectiveInterpolation checks, at every pixel the rectangle covers
// for sure, its perspective and linear uv, its depth, its position.w and
// the texel sampled at its uv.
func TestPerspectiveInterpolation(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		img := perspectiveScene(t, device, 0, 0, 1).run(t, device)
		background := [4]float64{rigClearColor, rigClearColor, rigClearColor, rigClearColor}
		covered := 0
		forEachPerspectivePixel(func(x, y int, nx, ny float64, tri int) {
			if tri < 0 {
				checkTexel(t, "varyings", x, y, img.at(0, x, y), background, 0)
				checkDepth(t, x, y, img.depthAt(x, y), 1, 0)
				return
			}
			covered++
			want := perspectiveExpect(&perspectiveQuad, tri, nx, ny)
			got, pos := img.at(0, x, y), img.at(1, x, y)
			checkTexel(t, "perspective uv", x, y, [4]float64{got[0], got[1]}, [4]float64{want.uv[0], want.uv[1]},
				perspectiveVaryingTolerance)
			checkTexel(t, "linear uv", x, y, [4]float64{got[2], got[3]}, [4]float64{want.uvLinear[0], want.uvLinear[1]},
				perspectiveVaryingTolerance)
			checkDepth(t, x, y, img.depthAt(x, y), want.depth, viewportTolerance)
			checkTexel(t, "position.z", x, y, [4]float64{pos[0]}, [4]float64{want.depth}, viewportTolerance)
			checkTexel(t, "position.w", x, y, [4]float64{pos[1]}, [4]float64{want.invW}, viewportTolerance)
			checkTexel(t, "texture", x, y, [4]float64{pos[2], pos[3]}, [4]float64{want.texel[0], want.texel[1]},
				perspectiveTextureTolerance)
		})
		if covered == 0 {
			t.Fatal("the expected coverage is empty: the test geometry is wrong")
		}
	})
}

// TestPerspectiveDepthTest draws the rectangle, then a plane at depth 0.5
// with depth test Less: the plane shows where the rectangle is farther than
// 0.5, the upper part of the screen, since the depth is linear on the
// screen from 0.2 at the bottom edge to 0.8 at the top.
func TestPerspectiveDepthTest(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		img := perspectiveScene(t, device, gputypes.CompareFunctionLess, 0, 2).run(t, device)
		plane := [4]float64{5, 5, 5, 5}
		planeShown, quadShown := 0, 0
		forEachPerspectivePixel(func(x, y int, nx, ny float64, tri int) {
			if tri < 0 {
				checkTexel(t, "plane", x, y, img.at(0, x, y), plane, 0)
				checkDepth(t, x, y, img.depthAt(x, y), 0.5, 0)
				return
			}
			want := perspectiveExpect(&perspectiveQuad, tri, nx, ny)
			if math.Abs(want.depth-0.5) < 1e-3 {
				return // a tie: precision decides
			}
			if want.depth > 0.5 {
				planeShown++
				checkTexel(t, "plane over the rectangle", x, y, img.at(0, x, y), plane, 0)
				checkDepth(t, x, y, img.depthAt(x, y), 0.5, 0)
				return
			}
			quadShown++
			got := img.at(0, x, y)
			checkTexel(t, "rectangle over the plane", x, y, [4]float64{got[0], got[1]},
				[4]float64{want.uv[0], want.uv[1]}, perspectiveVaryingTolerance)
			checkDepth(t, x, y, img.depthAt(x, y), want.depth, viewportTolerance)
		})
		if planeShown == 0 || quadShown == 0 {
			t.Fatalf("the depth test splits nothing (plane %d, rectangle %d pixels): the test geometry is wrong",
				planeShown, quadShown)
		}
	})
}

// TestPerspectiveFarClip draws the rectangle with its top edge at depth
// 1.2: the far plane cuts it where the depth, linear on the screen from 0.2
// at the bottom edge, reaches 1. The vertices clipping creates on the
// slanted edges must carry the perspective uv of their clip-space point and
// the linear uv of their screen position, so the varyings of the pixels
// below the cut still match the unclipped rectangle's.
func TestPerspectiveFarClip(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		img := perspectiveScene(t, device, 0, 6, 1).run(t, device)
		background := [4]float64{rigClearColor, rigClearColor, rigClearColor, rigClearColor}
		covered, cut := 0, 0
		forEachPerspectivePixel(func(x, y int, nx, ny float64, tri int) {
			var want perspectiveValues
			if tri >= 0 {
				want = perspectiveExpect(&perspectiveFarQuad, tri, nx, ny)
				if math.Abs(want.depth-1) < 1e-3 {
					return // on the far plane: precision decides
				}
			}
			if tri < 0 || want.depth > 1 {
				if tri >= 0 {
					cut++
				}
				checkTexel(t, "varyings", x, y, img.at(0, x, y), background, 0)
				checkDepth(t, x, y, img.depthAt(x, y), 1, 0)
				return
			}
			covered++
			got := img.at(0, x, y)
			checkTexel(t, "perspective uv", x, y, [4]float64{got[0], got[1]}, [4]float64{want.uv[0], want.uv[1]},
				perspectiveVaryingTolerance)
			checkTexel(t, "linear uv", x, y, [4]float64{got[2], got[3]}, [4]float64{want.uvLinear[0], want.uvLinear[1]},
				perspectiveVaryingTolerance)
			checkDepth(t, x, y, img.depthAt(x, y), want.depth, viewportTolerance)
		})
		if covered == 0 || cut == 0 {
			t.Fatalf("the far plane cuts nothing (%d pixels drawn, %d cut): the test geometry is wrong", covered, cut)
		}
	})
}

// behindEyeShader draws behindEyeVertices with the lane values behindEyeLane
// as a linear and as a perspective varying.
const behindEyeShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) @interpolate(linear) lin: f32,
    @location(1) persp: f32,
}

@vertex
fn vs_main(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec4f, 3>(vec4f(0.0, 0.0, -1.0, -1.0), vec4f(-0.5, -0.5, 0.5, 1.0), vec4f(0.5, -0.5, 0.5, 1.0));
    var a = array<f32, 3>(3.0, 0.0, 1.0);
    return VOut(p[i], a[i], a[i]);
}

@fragment
fn fs_main(in: VOut) -> @location(0) vec4f {
    return vec4f(in.lin, in.persp, 0.0, 1.0);
}
`

var (
	behindEyeVertices = [3][4]float64{{0, 0, -1, -1}, {-0.5, -0.5, 0.5, 1}, {0.5, -0.5, 0.5, 1}}
	behindEyeLane     = [3]float64{3, 0, 1}
)

// TestPerspectiveBehindEyeLinear draws a triangle with a vertex behind the
// eye and a linear varying. The near plane keeps its part with z >= 0. The
// software clipper derived a clipped vertex's linear varying from the
// screen position of the vertex it was cut from; the w plane's vertex there
// rounds to w = 0, which made the varying NaN at every pixel.
func TestPerspectiveBehindEyeLinear(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		const size = 16
		img := rigScene{size: size, wgsl: behindEyeShader, vsEntry: "vs_main", fsEntry: "fs_main", targets: 1,
			draw: func(_ *testing.T, _ *wgpu.Device, pass *wgpu.RenderPassEncoder) {
				pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
			}}.run(t, device)
		background := [4]float64{rigClearColor, rigClearColor, rigClearColor, rigClearColor}
		covered := 0
		for y := range size {
			for x := range size {
				nx, ny := (float64(x)+0.5)/size*2-1, 1-(float64(y)+0.5)/size*2
				in, sure := homogeneousCoverage(behindEyeVertices, nx, ny, 0.05*2/size)
				if !sure {
					continue // a pixel center on an edge: the fill rule decides
				}
				persp, linear, depth := clipSpaceExpect(behindEyeVertices, behindEyeLane, nx, ny)
				if in && math.Abs(depth) < 1e-3 {
					continue // on the near plane: precision decides
				}
				if !in || depth < 0 {
					checkTexel(t, "varyings", x, y, img.at(0, x, y), background, 0)
					checkDepth(t, x, y, img.depthAt(x, y), 1, 0)
					continue
				}
				covered++
				got := img.at(0, x, y)
				checkTexel(t, "varyings", x, y, [4]float64{got[0], got[1]}, [4]float64{linear, persp},
					perspectiveVaryingTolerance)
				checkDepth(t, x, y, img.depthAt(x, y), depth, viewportTolerance)
			}
		}
		if covered == 0 {
			t.Fatal("the expected coverage is empty: the test geometry is wrong")
		}
	})
}

// clipSpaceExpect returns the perspective and the linear value of a varying
// whose values at the clip-space vertices v are a, and the depth, at the NDC
// point (nx, ny): the clip-space point that projects there is Σ λi*Vi with λ
// proportional to μ = M⁻¹(nx, ny, 1), M's columns being the vertices' (x, y,
// w).
func clipSpaceExpect(v [3][4]float64, a [3]float64, nx, ny float64) (persp, linear, depth float64) {
	col := func(i int) [3]float64 { return [3]float64{v[i][0], v[i][1], v[i][3]} }
	det := func(a, b, c [3]float64) float64 {
		return a[0]*(b[1]*c[2]-b[2]*c[1]) - a[1]*(b[0]*c[2]-b[2]*c[0]) + a[2]*(b[0]*c[1]-b[1]*c[0])
	}
	p := [3]float64{nx, ny, 1}
	c0, c1, c2 := col(0), col(1), col(2)
	mu := [3]float64{det(p, c1, c2), det(c0, p, c2), det(c0, c1, p)}
	sum := mu[0] + mu[1] + mu[2]
	var w, z, wa float64
	for i := range 3 {
		l := mu[i] / sum
		persp += l * a[i]
		w += l * v[i][3]
		z += l * v[i][2]
		wa += l * v[i][3] * a[i]
	}
	return persp, wa / w, z / w
}

// perspectiveScene returns the scene of vs_main's six vertices from
// firstVertex, in instances instances, with a 4x4 RGBA8 texture whose texel
// (x, y) is (x/3, y/3, 0, 1) and a bilinear clamping sampler bound.
func perspectiveScene(t *testing.T, device *wgpu.Device, depthTest gputypes.CompareFunction,
	firstVertex, instances uint32) rigScene {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "perspective-texture",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	texels := make([]byte, 0, 4*4*4)
	for y := range 4 {
		for x := range 4 {
			texels = append(texels, byte(85*x), byte(85*y), 0, 255)
		}
	}
	if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, texels,
		&wgpu.ImageDataLayout{BytesPerRow: 16, RowsPerImage: 4},
		&wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}); err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "perspective-linear",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	t.Cleanup(sampler.Release)
	bgl := newBindGroupLayout(t, device,
		gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageFragment,
			Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D},
		},
		gputypes.BindGroupLayoutEntry{
			Binding: 1, Visibility: wgpu.ShaderStageFragment,
			Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
		})
	group := newBindGroup(t, device, bgl,
		wgpu.BindGroupEntry{Binding: 0, TextureView: view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: sampler})
	return rigScene{
		size: perspectiveSize, wgsl: perspectiveShader, vsEntry: "vs_main", fsEntry: "fs_main", targets: 2,
		depthTest: depthTest, layouts: []*wgpu.BindGroupLayout{bgl},
		draw: func(_ *testing.T, _ *wgpu.Device, pass *wgpu.RenderPassEncoder) {
			pass.SetBindGroup(0, group, nil)
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: instances, FirstVertex: firstVertex})
		},
	}
}

// forEachPerspectivePixel calls fn for every pixel of the target whose
// coverage by the rectangle is certain, with its center in NDC and the
// triangle covering it (-1 for none). Pixels whose center lies on an edge,
// the triangles' shared diagonal included, are left to the fill rule.
func forEachPerspectivePixel(fn func(x, y int, nx, ny float64, tri int)) {
	const eps = 0.05 * 2 / perspectiveSize
	for y := range perspectiveSize {
		for x := range perspectiveSize {
			nx := (float64(x)+0.5)/perspectiveSize*2 - 1
			ny := 1 - (float64(y)+0.5)/perspectiveSize*2
			tri, sure := -1, true
			for i, idx := range perspectiveTriangles {
				in, ok := homogeneousCoverage(perspectiveTriangleVertices(&perspectiveQuad, idx), nx, ny, eps)
				sure = sure && ok
				if in {
					tri = i
				}
			}
			if sure {
				fn(x, y, nx, ny, tri)
			}
		}
	}
}

func perspectiveTriangleVertices(quad *[4][4]float64, idx [3]int) [3][4]float64 {
	return [3][4]float64{quad[idx[0]], quad[idx[1]], quad[idx[2]]}
}

// perspectiveValues is what a fragment of the rectangle should get.
type perspectiveValues struct {
	uv, uvLinear, texel [2]float64
	depth, invW         float64
}

// perspectiveExpect computes the values at the NDC point (nx, ny) of
// triangle tri of quad. The clip-space point that projects there is Σ λi*Vi
// with λ proportional to μ = M⁻¹(nx, ny, 1), M's columns being the
// vertices' (x, y, w); its screen-space barycentrics are λi*wi / Σ λj*wj.
func perspectiveExpect(quad *[4][4]float64, tri int, nx, ny float64) perspectiveValues {
	idx := perspectiveTriangles[tri]
	v := perspectiveTriangleVertices(quad, idx)
	col := func(i int) [3]float64 { return [3]float64{v[i][0], v[i][1], v[i][3]} }
	det := func(a, b, c [3]float64) float64 {
		return a[0]*(b[1]*c[2]-b[2]*c[1]) - a[1]*(b[0]*c[2]-b[2]*c[0]) + a[2]*(b[0]*c[1]-b[1]*c[0])
	}
	p := [3]float64{nx, ny, 1}
	c0, c1, c2 := col(0), col(1), col(2)
	mu := [3]float64{det(p, c1, c2), det(c0, p, c2), det(c0, c1, p)}
	sum := mu[0] + mu[1] + mu[2]
	var w, z float64
	var out perspectiveValues
	for i := range 3 {
		l := mu[i] / sum
		w += l * v[i][3]
		z += l * v[i][2]
		out.uv[0] += l * perspectiveUV[idx[i]][0]
		out.uv[1] += l * perspectiveUV[idx[i]][1]
	}
	for i := range 3 {
		b := mu[i] / sum * v[i][3] / w
		out.uvLinear[0] += b * perspectiveUV[idx[i]][0]
		out.uvLinear[1] += b * perspectiveUV[idx[i]][1]
	}
	out.depth = z / w
	out.invW = 1 / w
	// Bilinear filtering of the 4x4 texture at texel coordinate 4*uv - 0.5,
	// clamped to the edge texels' centers.
	for c := range 2 {
		out.texel[c] = math.Min(math.Max(4*out.uv[c]-0.5, 0), 3) / 3
	}
	return out
}
