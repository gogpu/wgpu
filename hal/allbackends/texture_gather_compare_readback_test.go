// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// gatherCompareShader renders one case into each 8-pixel column of the
// 64x8 target: textureGather, textureGatherCompare, textureSampleCompare
// and textureSampleCompareLevel. Every sample is taken in uniform control
// flow (textureSampleCompare needs it) and the column picks its result.
//
// The color texture is 4x4 RGBA8 with texel k = x + 4y holding red 16k+5 and
// green 250-15k; the depth texture is 4x4 Depth16Unorm with texel k holding
// (2k+1)*2048/65535, about (2k+1)/32. At uv (0.5, 0.5) the gather footprint
// is the texels (1, 2), (2, 2), (2, 1), (1, 1) in lane order, k = 9, 10, 6, 5.
const gatherCompareShader = `
@group(0) @binding(0) var tc: texture_2d<f32>;
@group(0) @binding(1) var sclamp: sampler;
@group(0) @binding(2) var srepeat: sampler;
@group(0) @binding(3) var td: texture_depth_2d;
@group(0) @binding(4) var scmp: sampler_comparison;
// The same texels as tc: GLES binds one sampler per texture unit, so the
// repeat sampler gets a texture of its own.
@group(0) @binding(5) var tr: texture_2d<f32>;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    return VOut(vec4<f32>(quad[v % 6u], 0.0, 1.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    let c = vec2<f32>(0.5, 0.5);
    let implicit = textureSampleCompare(td, scmp, vec2<f32>(0.375, 0.375), 0.3);
    var r = array<vec4<f32>, 8>(
        textureGather(0, tc, sclamp, c),
        textureGather(1, tc, sclamp, c),
        textureGather(0, tr, srepeat, vec2<f32>(0.0, 0.0)),
        textureGather(0, tc, sclamp, c, vec2<i32>(1, 0)),
        textureGatherCompare(td, scmp, c, 0.4),
        vec4<f32>(implicit, implicit, implicit, 1.0),
        vec4<f32>(vec3<f32>(textureSampleCompareLevel(td, scmp, c, 0.4)), 1.0),
        vec4<f32>(vec3<f32>(textureSampleCompareLevel(td, scmp, vec2<f32>(0.625, 0.375), 0.5)), 1.0),
    );
    return r[min(u32(in.pos.x / 8.0), 7u)];
}
`

// gatherCompareTexel is the color texel k of the gather test texture.
func gatherCompareTexel(k int) [4]byte { return [4]byte{byte(16*k + 5), byte(250 - 15*k), 0, 255} }

// TestTextureGatherCompare reads back textureGather (component, address
// mode, offset), textureGatherCompare and both depth-compare samples on
// every backend. Before ADR-078 step 6 the software backend returned zero
// for all of them.
func TestTextureGatherCompare(t *testing.T) {
	r := func(k int) byte { return gatherCompareTexel(k)[0] }
	g := func(k int) byte { return gatherCompareTexel(k)[1] }
	want := [8][4]byte{
		{r(9), r(10), r(6), r(5)},
		{g(9), g(10), g(6), g(5)},
		// Repeat at uv 0: x0 = y0 = -1 wraps to 3: texels (3, 0), (0, 0),
		// (0, 3), (3, 3).
		{r(3), r(0), r(12), r(15)},
		// Offset (1, 0): texels (2, 2), (3, 2), (3, 1), (2, 1).
		{r(10), r(11), r(7), r(6)},
		// LessEqual with ref 0.4 against 0.59, 0.66, 0.41, 0.34.
		{255, 255, 255, 0},
		// Texel (1, 1), 0.34, at its center: 0.3 <= 0.34.
		{255, 255, 255, 255},
		// Linear filtering is PCF: three of the four footprint texels pass.
		{191, 191, 191, 255},
		// Texel (2, 1), 0.41, at its center: 0.5 <= 0.41 fails.
		{0, 0, 0, 255},
	}
	names := [8]string{
		"gather r", "gather g", "gather repeat", "gather offset", "gatherCompare",
		"sampleCompare", "sampleCompareLevel pcf", "sampleCompareLevel fail",
	}
	forEachGatherCompareDevice(t, func(t *testing.T, device *wgpu.Device) {
		colorView, depthView := newGatherCompareViews(t, device)
		repeatView, _ := newGatherCompareViews(t, device)
		frag := wgpu.ShaderStageFragment
		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: frag, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
			}},
			gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
			gputypes.BindGroupLayoutEntry{Binding: 2, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
			gputypes.BindGroupLayoutEntry{Binding: 3, Visibility: frag, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeDepth, ViewDimension: gputypes.TextureViewDimension2D,
			}},
			gputypes.BindGroupLayoutEntry{Binding: 4, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeComparison}},
			gputypes.BindGroupLayoutEntry{Binding: 5, Visibility: frag, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
			}},
		)
		pipeline := newRebindRenderPipeline(t, device, gatherCompareShader, bgl)
		newSampler := func(desc *wgpu.SamplerDescriptor) *wgpu.Sampler {
			s, err := device.CreateSampler(desc)
			if err != nil {
				t.Fatalf("CreateSampler: %v", err)
			}
			t.Cleanup(s.Release)
			return s
		}
		clamp := newSampler(&wgpu.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
		})
		repeat := newSampler(&wgpu.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeRepeat, AddressModeV: gputypes.AddressModeRepeat,
		})
		compare := newSampler(&wgpu.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
			MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear,
			Compare: gputypes.CompareFunctionLessEqual,
		})
		group := newBindGroup(t, device, bgl,
			wgpu.BindGroupEntry{Binding: 0, TextureView: colorView},
			wgpu.BindGroupEntry{Binding: 1, Sampler: clamp},
			wgpu.BindGroupEntry{Binding: 2, Sampler: repeat},
			wgpu.BindGroupEntry{Binding: 3, TextureView: depthView},
			wgpu.BindGroupEntry{Binding: 4, Sampler: compare},
			wgpu.BindGroupEntry{Binding: 5, TextureView: repeatView},
		)
		px := renderDynamicTarget(t, device, func(pass *wgpu.RenderPassEncoder) {
			pass.SetPipeline(pipeline)
			pass.SetBindGroup(0, group, nil)
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		})
		row := featureTargetHeight / 2 * featureTargetWidth
		for col, w := range want {
			got := px[row+col*8+4]
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], got)
			if !bytesWithin(b, w, 1) {
				t.Errorf("%s: got %v, want %v", names[col], b, w)
			}
		}
	})
}

// forEachGatherCompareDevice runs fn with a device of every overrideBackends
// entry that has an adapter here, with the Vulkan validation layer and the
// DX12 debug layer on. WARP is left out: the device openWarpDevice wraps has
// no downlevel flags, so its WriteTexture of the depth texture is refused.
func forEachGatherCompareDevice(t *testing.T, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	for _, b := range overrideBackends {
		t.Run(b.name, func(t *testing.T) {
			if b.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
				t.Setenv("GOGPU_DX12_DXIL_VALIDATE", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			device, _ := openDebugBackendDevice(t, b, 0, gputypes.PowerPreferenceLowPower)
			fn(t, device)
		})
	}
}

// bytesWithin reports whether every channel of got is within tol of want:
// a linear comparison filter may round its weights differently by one unit.
func bytesWithin(got, want [4]byte, tol int) bool {
	for i := range got {
		if d := int(got[i]) - int(want[i]); d < -tol || d > tol {
			return false
		}
	}
	return true
}

// newGatherCompareViews creates the 4x4 color and Depth16Unorm textures of
// gatherCompareShader.
func newGatherCompareViews(t *testing.T, device *wgpu.Device) (color, depth *wgpu.TextureView) {
	t.Helper()
	const side = 4
	colorTexels := make([]byte, 0, side*side*4)
	depthTexels := make([]byte, 0, side*side*2)
	for k := range side * side {
		c := gatherCompareTexel(k)
		colorTexels = append(colorTexels, c[:]...)
		depthTexels = binary.LittleEndian.AppendUint16(depthTexels, uint16((2*k+1)*2048))
	}
	color = newFilledView(t, device, gputypes.TextureFormatRGBA8Unorm, colorTexels, 4)
	depth = newFilledView(t, device, gputypes.TextureFormatDepth16Unorm, depthTexels, 2)
	return color, depth
}

// newFilledView creates a 4x4 sampled texture of format, writes texels and
// returns a view of it.
func newFilledView(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, texels []byte, bpp uint32) *wgpu.TextureView {
	t.Helper()
	const side = 4
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "gather-compare",
		Size:          wgpu.Extent3D{Width: side, Height: side, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture %v: %v", format, err)
	}
	t.Cleanup(tex.Release)
	if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, texels,
		&wgpu.ImageDataLayout{BytesPerRow: side * bpp, RowsPerImage: side},
		&wgpu.Extent3D{Width: side, Height: side, DepthOrArrayLayers: 1}); err != nil {
		t.Fatalf("WriteTexture %v: %v", format, err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView %v: %v", format, err)
	}
	t.Cleanup(view.Release)
	return view
}
