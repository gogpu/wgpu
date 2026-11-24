//go:build !(js && wasm)

package software

import (
	"math"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/software/raster"
)

// multisampleSize is the side of the square targets the multisample tests
// draw into.
const multisampleSize = 8

// standardSamplePositions are the positions of a 4-sample pixel's samples
// inside it, Vulkan's standard sample locations and D3D's standard pattern.
var standardSamplePositions = [4][2]float64{{0.375, 0.125}, {0.875, 0.375}, {0.125, 0.625}, {0.625, 0.875}}

// multisampleTriangleWGSL draws, at vertex_index 0 to 2, the triangle of
// multisampleTriangleCorners, whose edges cross the pixels at three slopes.
// Its left edge, at x = 1.6875, leaves the centers of column 1 outside its
// bounding box and samples of that column inside it. Each fragment entry
// point reaches another of the backend's draw paths and writes red.
const multisampleTriangleWGSL = `
const corners = array<vec2<f32>, 3>(vec2<f32>(-0.578125, 0.859375), vec2<f32>(0.890625, -0.359375), vec2<f32>(-0.578125, -0.953125));

@vertex
fn vs_main(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    return vec4<f32>(corners[i], 0.0, 1.0);
}

struct Interpolated {
    @builtin(position) position: vec4<f32>,
    @location(0) color: vec4<f32>,
}

@vertex
fn vs_color(@builtin(vertex_index) i: u32) -> Interpolated {
    return Interpolated(vec4<f32>(corners[i], 0.0, 1.0), vec4<f32>(1.0, 0.0, 0.0, 1.0));
}

@fragment
fn fs_solid() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}

@fragment
fn fs_per_fragment(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0 + 0.0 * position.x);
}

@fragment
fn fs_quads(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(dpdx(position.x), 0.0, 0.0, 1.0);
}

struct Targets {
    @location(0) first: vec4<f32>,
    @location(1) second: vec4<f32>,
}

@fragment
fn fs_targets(@builtin(position) position: vec4<f32>) -> Targets {
    let red = vec4<f32>(1.0, 0.0, 0.0, 1.0 + 0.0 * position.x);
    return Targets(red, red);
}
`

// multisampleTriangleCorners are the corners of multisampleTriangleWGSL's
// triangle in pixels, sixteenths that the rasterizer's fixed point holds
// exactly.
var multisampleTriangleCorners = [3][2]float64{{1.6875, 0.5625}, {7.5625, 5.4375}, {1.6875, 7.8125}}

// multisampleFullscreenWGSL covers the target with the two triangles of the
// diagonal from its top-left to its bottom-right corner, at vertex_index 0
// to 5, and writes the constant color of half red.
const multisampleFullscreenWGSL = `
@vertex
fn vs_main(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, -1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, 1.0), vec2<f32>(1.0, -1.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.5, 0.0, 0.0, 1.0);
}

@fragment
fn fs_white() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 1.0, 1.0, 1.0);
}
`

// TestAMultisampledTriangleResolvesItsEdgeToTheSamplesItCovers draws
// multisampleTriangleWGSL's triangle in red over black into a 4-sample
// texture on each draw path and resolves it: each pixel's red must be the
// quarters of its samples the triangle covers, its interior and exterior
// exact, the edge's pixels in between.
func TestAMultisampledTriangleResolvesItsEdgeToTheSamplesItCovers(t *testing.T) {
	for _, test := range []struct {
		path, vertex, fragment string
		targets                int
	}{
		{"solid", "vs_main", "fs_solid", 1},
		{"per fragment", "vs_main", "fs_per_fragment", 1},
		{"quads", "vs_main", "fs_quads", 1},
		{"several targets", "vs_main", "fs_targets", 2},
		{"interpolated without a fragment stage", "vs_color", "", 1},
	} {
		t.Run(test.path, func(t *testing.T) {
			dev, _, cleanup := createSoftwareDevice(t)
			defer cleanup()
			formats := make([]gputypes.TextureFormat, test.targets)
			for i := range formats {
				formats[i] = gputypes.TextureFormatRGBA8Unorm
			}
			pipeline := newMultisamplePipeline(t, dev, multisampleTriangleWGSL, test.vertex, test.fragment, formats,
				gputypes.MultisampleState{Count: 4}, nil)

			resolved := drawMultisampled(t, dev, pipeline, formats, gputypes.Color{A: 1}, 3)

			covered := countCoveredSamples(t)
			for _, texture := range resolved {
				checkResolved(t, dev, texture, func(x, y uint32) [4]byte {
					return [4]byte{encodeUnorm8(float64(covered[y][x]) / 4), 0, 0, 255}
				})
			}
		})
	}
}

// TestTwoTrianglesSharingAnEdgeResolveWithoutASeam covers a 4-sample target
// with two triangles sharing its diagonal, adding half red to black: a sample
// both covered would resolve its pixel above half, one neither covered below.
func TestTwoTrianglesSharingAnEdgeResolveWithoutASeam(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	formats := []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm}
	additive := &gputypes.BlendState{
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorZero, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
	}
	pipeline := newMultisamplePipeline(t, dev, multisampleFullscreenWGSL, "vs_main", "fs_main", formats, gputypes.MultisampleState{Count: 4}, additive)

	resolved := drawMultisampled(t, dev, pipeline, formats, gputypes.Color{A: 1}, 6)

	checkResolved(t, dev, resolved[0], func(_, _ uint32) [4]byte { return [4]byte{encodeUnorm8(0.5), 0, 0, 255} })
}

// TestAClearAloneResolvesToItsColor clears a 4-sample target and draws
// nothing: the resolve target must hold the clear color.
func TestAClearAloneResolvesToItsColor(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	formats := []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm}

	resolved := drawMultisampled(t, dev, nil, formats, gputypes.Color{R: 0.2, G: 0.4, B: 0.6, A: 1}, 0)

	checkResolved(t, dev, resolved[0], func(_, _ uint32) [4]byte {
		return [4]byte{encodeUnorm8(0.2), encodeUnorm8(0.4), encodeUnorm8(0.6), 255}
	})
}

// TestASampleMaskKeepsOnlyTheSamplesItNames covers a 4-sample target in white
// over black through a pipeline whose mask keeps one sample, then two in an
// sRGB target: the first resolves to a quarter of white, the second to half
// of white in linear, which an average of the sRGB bytes would make darker.
func TestASampleMaskKeepsOnlyTheSamplesItNames(t *testing.T) {
	for _, test := range []struct {
		name   string
		format gputypes.TextureFormat
		mask   uint64
		want   byte
	}{
		{"one sample", gputypes.TextureFormatRGBA8Unorm, 0b0001, encodeUnorm8(0.25)},
		{"two samples of sRGB", gputypes.TextureFormatRGBA8UnormSrgb, 0b0110, encodeUnorm8(encodeSRGB(0.5))},
	} {
		t.Run(test.name, func(t *testing.T) {
			dev, _, cleanup := createSoftwareDevice(t)
			defer cleanup()
			formats := []gputypes.TextureFormat{test.format}
			pipeline := newMultisamplePipeline(t, dev, multisampleFullscreenWGSL, "vs_main", "fs_white", formats,
				gputypes.MultisampleState{Count: 4, Mask: test.mask}, nil)

			resolved := drawMultisampled(t, dev, pipeline, formats, gputypes.Color{A: 1}, 6)

			checkResolved(t, dev, resolved[0], func(_, _ uint32) [4]byte { return [4]byte{test.want, test.want, test.want, 255} })
		})
	}
}

// TestWhatTheBackendDoesNotMultisampleIsRefused creates a 4-sample depth
// texture, a pipeline enabling alpha to coverage and a bind group of a
// 4-sample texture: each must fail, naming what is not multisampled.
func TestWhatTheBackendDoesNotMultisampleIsRefused(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	_, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 4,
		Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatDepth24Plus, Usage: gputypes.TextureUsageRenderAttachment,
	})
	if err == nil || !strings.Contains(err.Error(), "depth") {
		t.Errorf("a 4-sample Depth24Plus texture answered %v, want an error naming depth", err)
	}

	module, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Source: hal.ShaderSource{WGSL: multisampleFullscreenWGSL}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	_, err = dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Vertex:      hal.VertexState{Module: module, EntryPoint: "vs_main"},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 4, AlphaToCoverageEnabled: true},
		Fragment: &hal.FragmentState{Module: module, EntryPoint: "fs_main", Targets: []gputypes.ColorTargetState{
			{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "alpha to coverage") {
		t.Errorf("a pipeline enabling alpha to coverage answered %v, want an error naming alpha to coverage", err)
	}

	multisampled, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 4,
		Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture of 4 samples: %v", err)
	}
	view := newSubView(t, dev, multisampled, hal.TextureViewDescriptor{})
	_, err = dev.CreateBindGroup(&hal.BindGroupDescriptor{Entries: []gputypes.BindGroupEntry{
		{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()}},
	}})
	if err == nil || !strings.Contains(err.Error(), "multisampled") {
		t.Errorf("a bind group of a 4-sample texture answered %v, want an error naming it multisampled", err)
	}
}

// TestTheAdapterReportsMultisamplingOfColorFormatsAlone reads the adapter's
// capabilities of a float, an integer, a depth and a compressed format: a
// format the backend draws color into multisamples, and one it can average
// resolves too.
func TestTheAdapterReportsMultisamplingOfColorFormatsAlone(t *testing.T) {
	both := hal.TextureFormatCapabilityMultisample | hal.TextureFormatCapabilityMultisampleResolve
	for _, test := range []struct {
		format gputypes.TextureFormat
		want   hal.TextureFormatCapabilityFlags
	}{
		{gputypes.TextureFormatRGBA8Unorm, both},
		{gputypes.TextureFormatBGRA8UnormSrgb, both},
		{gputypes.TextureFormatR32Uint, hal.TextureFormatCapabilityMultisample},
		{gputypes.TextureFormatDepth24Plus, 0},
		{gputypes.TextureFormatBC1RGBAUnorm, 0},
	} {
		if got := (&Adapter{}).TextureFormatCapabilities(test.format).Flags & both; got != test.want {
			t.Errorf("%v: multisample flags %b, want %b", test.format, got, test.want)
		}
	}
}

// newMultisamplePipeline returns the pipeline of wgsl's vertex and fragment
// entry points, no fragment stage when fragment is empty, drawing into one
// target of each of formats with blend.
func newMultisamplePipeline(t *testing.T, dev *Device, wgsl, vertex, fragment string, formats []gputypes.TextureFormat,
	multisample gputypes.MultisampleState, blend *gputypes.BlendState) hal.RenderPipeline {
	t.Helper()
	module, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Source: hal.ShaderSource{WGSL: wgsl}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	desc := &hal.RenderPipelineDescriptor{
		Vertex:      hal.VertexState{Module: module, EntryPoint: vertex},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: multisample,
	}
	if fragment != "" {
		targets := make([]gputypes.ColorTargetState, len(formats))
		for i, format := range formats {
			targets[i] = gputypes.ColorTargetState{Format: format, WriteMask: gputypes.ColorWriteMaskAll, Blend: blend}
		}
		desc.Fragment = &hal.FragmentState{Module: module, EntryPoint: fragment, Targets: targets}
	}
	pipeline, err := dev.CreateRenderPipeline(desc)
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	return pipeline
}

// drawMultisampled runs one pass over a 4-sample texture of each of formats,
// each cleared to clearColor and resolved into a texture of one sample, and
// answers the resolve targets. It draws vertexCount vertices through
// pipeline, or nothing when pipeline is nil.
func drawMultisampled(t *testing.T, dev *Device, pipeline hal.RenderPipeline, formats []gputypes.TextureFormat,
	clearColor gputypes.Color, vertexCount uint32) []hal.Texture {
	t.Helper()
	resolved := make([]hal.Texture, len(formats))
	attachments := make([]hal.RenderPassColorAttachment, len(formats))
	for i, format := range formats {
		multisampled, err := dev.CreateTexture(&hal.TextureDescriptor{
			Size: hal.Extent3D{Width: multisampleSize, Height: multisampleSize, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 4,
			Dimension: gputypes.TextureDimension2D, Format: format, Usage: gputypes.TextureUsageRenderAttachment,
		})
		if err != nil {
			t.Fatalf("CreateTexture of 4 samples: %v", err)
		}
		resolved[i] = newSubTexture(t, dev, gputypes.TextureDimension2D, multisampleSize, multisampleSize, 1, 1, format)
		attachments[i] = hal.RenderPassColorAttachment{
			View:          newSubView(t, dev, multisampled, hal.TextureViewDescriptor{}),
			ResolveTarget: newSubView(t, dev, resolved[i], hal.TextureViewDescriptor{}),
			LoadOp:        gputypes.LoadOpClear, StoreOp: gputypes.StoreOpDiscard, ClearValue: clearColor,
		}
	}
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: attachments})
	if pipeline != nil {
		pass.SetPipeline(pipeline)
		pass.Draw(gputypes.DrawArgs{VertexCount: vertexCount, InstanceCount: 1})
	}
	pass.End()
	submit(t, enc)
	return resolved
}

// countCoveredSamples answers, for each pixel of a multisampleSize square
// target, how many of its standard samples lie inside the triangle of
// multisampleTriangleCorners. It fails the test when a sample lies on an
// edge, where the fill rule rather than this count decides.
func countCoveredSamples(t *testing.T) [multisampleSize][multisampleSize]int {
	t.Helper()
	corners := multisampleTriangleCorners
	var counts [multisampleSize][multisampleSize]int
	edge := func(a, b [2]float64, x, y float64) float64 {
		return (b[0]-a[0])*(y-a[1]) - (b[1]-a[1])*(x-a[0])
	}
	for y := range multisampleSize {
		for x := range multisampleSize {
			for _, position := range standardSamplePositions {
				sampleX := float64(x) + position[0]
				sampleY := float64(y) + position[1]
				e0 := edge(corners[0], corners[1], sampleX, sampleY)
				e1 := edge(corners[1], corners[2], sampleX, sampleY)
				e2 := edge(corners[2], corners[0], sampleX, sampleY)
				if math.Abs(e0) < 1e-9 || math.Abs(e1) < 1e-9 || math.Abs(e2) < 1e-9 {
					t.Fatalf("sample (%v, %v) lies on an edge of the triangle", sampleX, sampleY)
				}
				if (e0 > 0) == (e1 > 0) && (e1 > 0) == (e2 > 0) {
					counts[y][x]++
				}
			}
		}
	}
	return counts
}

// checkResolved compares the multisampleSize square RGBA8 resolve target
// texture with want(x, y).
func checkResolved(t *testing.T, dev *Device, texture hal.Texture, want func(x, y uint32) [4]byte) {
	t.Helper()
	checkImage(t, readImage(t, dev, texture, 0, 0, multisampleSize, multisampleSize, 4), 0, 0, multisampleSize, multisampleSize, want)
}

// encodeUnorm8 answers v in [0, 1] as an 8-bit unorm, as the backend rounds it.
func encodeUnorm8(v float64) byte {
	return raster.FloatToUnorm8(float32(v))
}

// encodeSRGB answers the sRGB encoding of the linear value v.
func encodeSRGB(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}
