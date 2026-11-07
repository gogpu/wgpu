//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// positionedQuadWGSL is gogpu's positionedQuadShaderSource (gogpu shader.go),
// the shader of its compositor blit pipeline, which draws the software
// backend's frame to the surface: a vertex-buffer-less quad positioned by a
// uniform, sampling one texture.
const positionedQuadWGSL = `
struct QuadUniforms {
    rect: vec4<f32>,
    screen: vec2<f32>,
    alpha: f32,
    premultiplied: f32,
}

@group(0) @binding(0) var<uniform> uniforms: QuadUniforms;
@group(1) @binding(0) var texSampler: sampler;
@group(1) @binding(1) var tex: texture_2d<f32>;

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

@vertex
fn vs_main(@builtin(vertex_index) vertexIndex: u32) -> VertexOutput {
    var corners = array<vec2<f32>, 6>(
        vec2<f32>(0.0, 0.0),
        vec2<f32>(0.0, 1.0),
        vec2<f32>(1.0, 1.0),
        vec2<f32>(0.0, 0.0),
        vec2<f32>(1.0, 1.0),
        vec2<f32>(1.0, 0.0)
    );
    let corner = corners[vertexIndex];
    let pixelX = uniforms.rect.x + corner.x * uniforms.rect.z;
    let pixelY = uniforms.rect.y + corner.y * uniforms.rect.w;
    let ndcX = (pixelX / uniforms.screen.x) * 2.0 - 1.0;
    let ndcY = 1.0 - (pixelY / uniforms.screen.y) * 2.0;
    var output: VertexOutput;
    output.position = vec4<f32>(ndcX, ndcY, 0.0, 1.0);
    output.uv = corner;
    return output;
}

@fragment
fn fs_main(input: VertexOutput) -> @location(0) vec4<f32> {
    let texColor = textureSample(tex, texSampler, input.uv);
    if (uniforms.premultiplied > 0.5) {
        return texColor * uniforms.alpha;
    } else {
        let a = texColor.a * uniforms.alpha;
        return vec4<f32>(texColor.rgb * a, a);
    }
}
`

// invertTriangleWGSL draws one fullscreen triangle from vertex_index and
// writes the inverted color of the sampled texture: not a plain copy.
const invertTriangleWGSL = `
@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

@vertex
fn vs_main(@builtin(vertex_index) i: u32) -> VertexOutput {
    let p = vec2<f32>(f32((i << 1u) & 2u), f32(i & 2u));
    var out: VertexOutput;
    out.position = vec4<f32>(p.x * 2.0 - 1.0, 1.0 - p.y * 2.0, 0.0, 1.0);
    out.uv = p;
    return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    let c = textureSample(tex, samp, in.uv);
    return vec4<f32>(1.0 - c.rgb, c.a);
}
`

// shaderDraw is a render setup of one pipeline whose draws have no vertex
// buffer, with a source texture bound.
type shaderDraw struct {
	dev       *Device
	pipeline  hal.RenderPipeline
	groups    []hal.BindGroup
	dstView   hal.TextureView
	vertCount uint32
}

// newShaderTexture creates a w x h texture of format and fills it with pixel.
func newShaderTexture(t testing.TB, dev *Device, w, h uint32, format gputypes.TextureFormat, pixel [4]byte) (hal.Texture, hal.TextureView) {
	t.Helper()
	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
		Format: format,
		Usage:  gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	data := make([]byte, w*h*4)
	for i := 0; i < len(data); i += 4 {
		copy(data[i:i+4], pixel[:])
	}
	tex.(*Texture).WriteData(0, data)
	view, err := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	return tex, view
}

// newShaderPipeline compiles wgsl and creates a pipeline of its vs_main and
// fs_main writing format with blend.
func newShaderPipeline(t testing.TB, dev *Device, wgsl string, format gputypes.TextureFormat, blend *gputypes.BlendState) hal.RenderPipeline {
	t.Helper()
	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "blit-shader", Source: hal.ShaderSource{WGSL: wgsl}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	p, err := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:     "blit-shader",
		Vertex:    hal.VertexState{Module: sm, EntryPoint: "vs_main"},
		Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Fragment: &hal.FragmentState{
			Module: sm, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{Format: format, WriteMask: gputypes.ColorWriteMaskAll, Blend: blend}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	return p
}

// newShaderBindGroup creates a bind group of entries.
func newShaderBindGroup(t testing.TB, dev *Device, entries ...gputypes.BindGroupEntry) hal.BindGroup {
	t.Helper()
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	return bg
}

// draw records one render pass that clears dst to black and draws.
func (s *shaderDraw) draw(t testing.TB) {
	t.Helper()
	enc, err := s.dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View: s.dstView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{A: 1},
		}},
	})
	pass.SetPipeline(s.pipeline)
	for i, g := range s.groups {
		pass.SetBindGroup(uint32(i), g, nil)
	}
	pass.Draw(gputypes.DrawArgs{VertexCount: s.vertCount, InstanceCount: 1})
	pass.End()
	submit(t, enc)
}

// TestDrawRunsFragmentShaderNotBlit draws a fullscreen triangle whose
// fragment shader inverts the sampled color. With no vertex buffer and a
// texture bound, the software backend used to copy the texture instead of
// running the shaders, so the target got the source color.
func TestDrawRunsFragmentShaderNotBlit(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	const size = 8
	// 0 and 255 invert exactly, whatever the float-to-unorm rounding.
	_, srcView := newShaderTexture(t, dev, size, size, gputypes.TextureFormatRGBA8Unorm, [4]byte{255, 0, 255, 255})
	dst, dstView := newShaderTexture(t, dev, size, size, gputypes.TextureFormatRGBA8Unorm, [4]byte{})
	samp, err := dev.CreateSampler(&hal.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	s := &shaderDraw{
		dev:      dev,
		pipeline: newShaderPipeline(t, dev, invertTriangleWGSL, gputypes.TextureFormatRGBA8Unorm, nil),
		groups: []hal.BindGroup{newShaderBindGroup(t, dev,
			gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
			gputypes.BindGroupEntry{Binding: 1, Resource: gputypes.SamplerBinding{Sampler: samp.NativeHandle()}},
		)},
		dstView:   dstView,
		vertCount: 3,
	}
	s.draw(t)

	data := dst.(*Texture).GetData()
	want := [4]byte{0, 255, 0, 255}
	for _, px := range []int{0, size*size/2 + size/2, size*size - 1} {
		got := [4]byte(data[px*4 : px*4+4])
		if got != want {
			t.Errorf("pixel %d = %v, want %v (the inverted source)", px, got, want)
		}
	}
}

// BenchmarkFullscreenTexturedDraw measures the shape of gogpu's software
// present: its compositor blit pipeline (positionedQuadWGSL, premultiplied
// source-over blend) draws a 1280x720 RGBA8 texture over a 1280x720 BGRA8
// target with no vertex buffer, the quad positioned by a uniform.
func BenchmarkFullscreenTexturedDraw(b *testing.B) {
	backend := NewBackend()
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	openDev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()
	dev := openDev.Device.(*Device)

	const w, h = 1280, 720
	_, srcView := newShaderTexture(b, dev, w, h, gputypes.TextureFormatRGBA8Unorm, [4]byte{200, 50, 10, 255})
	_, dstView := newShaderTexture(b, dev, w, h, gputypes.TextureFormatBGRA8Unorm, [4]byte{})

	uniforms := make([]byte, 32)
	for i, f := range []float32{0, 0, w, h, w, h, 1, 1} { // rect, screen, alpha, premultiplied
		binary.LittleEndian.PutUint32(uniforms[i*4:], math.Float32bits(f))
	}
	ubuf, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 32, Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst})
	if err != nil {
		b.Fatalf("CreateBuffer: %v", err)
	}
	ubuf.(*Buffer).WriteData(0, uniforms)
	samp, err := dev.CreateSampler(&hal.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
		MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear,
	})
	if err != nil {
		b.Fatalf("CreateSampler: %v", err)
	}
	srcOver := &gputypes.BlendState{
		Color: gputypes.BlendComponent{Operation: gputypes.BlendOperationAdd, SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha},
		Alpha: gputypes.BlendComponent{Operation: gputypes.BlendOperationAdd, SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha},
	}
	s := &shaderDraw{
		dev:      dev,
		pipeline: newShaderPipeline(b, dev, positionedQuadWGSL, gputypes.TextureFormatBGRA8Unorm, srcOver),
		groups: []hal.BindGroup{
			newShaderBindGroup(b, dev, gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.BufferBinding{Buffer: ubuf.NativeHandle(), Size: 32}}),
			newShaderBindGroup(b, dev,
				gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.SamplerBinding{Sampler: samp.NativeHandle()}},
				gputypes.BindGroupEntry{Binding: 1, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
			),
		},
		dstView:   dstView,
		vertCount: 6,
	}

	b.ResetTimer()
	start := shader.ExecutedInstructions()
	for range b.N {
		s.draw(b)
	}
	// The executed SPIR-V instructions, counted only with -tags swinstrcount.
	if n := shader.ExecutedInstructions() - start; n > 0 {
		b.ReportMetric(float64(n)/float64(b.N), "instrs/op")
	}
}
