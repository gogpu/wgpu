//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"context"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// sampleShader renders a single full-screen triangle that samples a texture.
// The fragment maps gl_FragCoord.x across the output width to U in [0,1], V=0.5,
// and writes the sampled .r value to all RGBA channels. This lets the test read
// back the U->texel mapping directly: output column x shows the texel sampled at
// U = x/width.
const sampleShader = `
struct VSOut { @builtin(position) pos: vec4<f32>, @location(0) uv: vec2<f32> };

@vertex
fn vs_main(@builtin(vertex_index) vid: u32) -> VSOut {
    // Full-screen triangle.
    var p = array<vec2<f32>,3>(vec2<f32>(-1.0,-1.0), vec2<f32>(3.0,-1.0), vec2<f32>(-1.0,3.0));
    var o: VSOut;
    o.pos = vec4<f32>(p[vid], 0.0, 1.0);
    // Map clip space to UV: x in [-1,3]->[0,2], y similarly. We only use x here.
    o.uv = (p[vid] + vec2<f32>(1.0,1.0)) * 0.5;
    return o;
}

@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var s: sampler;

@fragment
fn fs_main(in: VSOut) -> @location(0) vec4<f32> {
    let v = textureSample(t, s, vec2<f32>(in.uv.x, 0.5)).r;
    return vec4<f32>(v, v, v, 1.0);
}
`

// TestR8SampleCoordRepro verifies that sampling an R8Unorm texture maps the U
// texture coordinate 1:1. It uploads a vertical-stripe pattern (one 255 stripe
// near texel 0.75*width) and checks that sampling U=0.75 hits it.
//
// BUG: on the pure-Go Metal HAL, the R8 sample coordinate U was scaled ~4x,
// so U=0.75 sampled near the texture edge and the stripe appeared at U~0.1875.
func TestR8SampleCoordRepro(t *testing.T) {
	runSampleCoordTest(t, gputypes.TextureFormatR8Unorm, 1)
}

// TestRGBA8SampleCoordControl is the control: RGBA8 sampling must also be 1:1.
func TestRGBA8SampleCoordControl(t *testing.T) {
	runSampleCoordTest(t, gputypes.TextureFormatRGBA8Unorm, 4)
}

func runSampleCoordTest(t *testing.T, format wgpu.TextureFormat, bytesPerPixel int) {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: wgpu.PowerPreferenceHighPerformance})
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	defer adapter.Release()
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{Label: "r8repro"})
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	defer device.Release()
	q := device.Queue()

	const texW, texH = 256, 4
	const stripeTexel = 192 // 0.75 * 256
	// Build texture data: a 255 stripe at columns [stripeTexel, stripeTexel+8).
	data := make([]byte, texW*texH*bytesPerPixel)
	for y := 0; y < texH; y++ {
		for x := stripeTexel; x < stripeTexel+8; x++ {
			o := (y*texW + x) * bytesPerPixel
			data[o] = 255 // R channel
			if bytesPerPixel == 4 {
				data[o+3] = 255 // A
			}
		}
	}

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "sample-coord-tex",
		Size:          wgpu.Extent3D{Width: texW, Height: texH, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()
	if err := q.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0},
		data,
		&wgpu.ImageDataLayout{Offset: 0, BytesPerRow: uint32(texW * bytesPerPixel), RowsPerImage: texH},
		&wgpu.Extent3D{Width: texW, Height: texH, DepthOrArrayLayers: 1},
	); err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	defer view.Release()

	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "nearest",
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeNearest,
		MinFilter:    gputypes.FilterModeNearest,
		MipmapFilter: gputypes.FilterModeNearest,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	defer sampler.Release()

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "sample", WGSL: sampleShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageFragment, Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D}},
			{Binding: 1, Visibility: gputypes.ShaderStageFragment, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()
	pl, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer pl.Release()
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, TextureView: view},
			{Binding: 1, Sampler: sampler},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	const outW, outH = 256, 1
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "sample-pipe",
		Layout: pl,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs_main"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	outTex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "out",
		Size:          wgpu.Extent3D{Width: outW, Height: outH, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture(out): %v", err)
	}
	defer outTex.Release()
	outView, err := device.CreateTextureView(outTex, nil)
	if err != nil {
		t.Fatalf("CreateView(out): %v", err)
	}
	defer outView.Release()

	enc, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	rp, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: outView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	rp.SetPipeline(pipeline)
	rp.SetBindGroup(0, bg, nil)
	rp.Draw(3, 1, 0, 0)
	_ = rp.End()

	enc.TransitionTextures([]wgpu.TextureBarrier{{Texture: outTex, Usage: wgpu.TextureUsageTransition{OldUsage: gputypes.TextureUsageRenderAttachment, NewUsage: gputypes.TextureUsageCopySrc}}})

	staging, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: outW * 4, Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer(staging): %v", err)
	}
	defer staging.Release()
	enc.CopyTextureToBuffer(outTex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{Offset: 0, BytesPerRow: outW * 4, RowsPerImage: outH},
		TextureBase:  wgpu.ImageCopyTexture{Texture: outTex, MipLevel: 0},
		Size:         wgpu.Extent3D{Width: outW, Height: outH, DepthOrArrayLayers: 1},
	}})
	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := q.Submit(cb); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, outW*4); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := staging.MappedRange(0, outW*4)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	px := rng.Bytes()

	// Find the output column where the stripe shows up (the bright band).
	firstBright := -1
	for x := 0; x < outW; x++ {
		if px[x*4] > 128 {
			firstBright = x
			break
		}
	}
	staging.Unmap()

	expected := stripeTexel // output column x maps to U=x/outW; texture texel = U*texW = x (since outW==texW)
	t.Logf("format=%v: stripe at texel %d sampled bright at output col %d (expected ~%d)", format, stripeTexel, firstBright, expected)
	if firstBright < 0 {
		t.Fatalf("format=%v: stripe not found in output — U coordinate maps off-texture", format)
	}
	if firstBright < expected-12 || firstBright > expected+12 {
		t.Fatalf("format=%v: stripe sampled at output col %d but expected ~%d — U coordinate is mis-scaled (ratio %.2f)",
			format, firstBright, expected, float64(expected)/float64(firstBright+1))
	}
}

// indexedShader draws solid green; vertex positions come from a vertex buffer
// (location 0). It exists to verify indexed drawing actually rasterizes.
const indexedShader = `
@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
    return vec4<f32>(pos, 0.0, 1.0);
}
@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// TestDrawIndexedRenders verifies that DrawIndexed actually rasterizes geometry.
// A full-screen quad is drawn as two triangles via a uint16 index buffer
// (0,1,2, 2,3,0). Previously DrawIndexed was a no-op on the software backend,
// so glyph-mask and MSDF text (the only indexed-draw tiers) rendered nothing.
func TestDrawIndexedRenders(t *testing.T) {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	defer adapter.Release()
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	defer device.Release()
	q := device.Queue()

	// Full-screen quad: 4 vertices (vec2 NDC), 6 indices.
	verts := []float32{-1, -1, 1, -1, 1, 1, -1, 1}
	vbytes := make([]byte, len(verts)*4)
	for i, f := range verts {
		binaryLEPutFloat32(vbytes[i*4:], f)
	}
	indices := []uint16{0, 1, 2, 2, 3, 0}
	ibytes := make([]byte, len(indices)*2)
	for i, v := range indices {
		ibytes[i*2] = byte(v)
		ibytes[i*2+1] = byte(v >> 8)
	}

	vbuf, _ := device.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(len(vbytes)), Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst})
	defer vbuf.Release()
	q.WriteBuffer(vbuf, 0, vbytes)
	ibuf, _ := device.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(len(ibytes)), Usage: gputypes.BufferUsageIndex | gputypes.BufferUsageCopyDst})
	defer ibuf.Release()
	q.WriteBuffer(ibuf, 0, ibytes)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: indexedShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()
	pl, _ := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{})
	defer pl.Release()

	const W, H = 16, 16
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout: pl,
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: "vs_main",
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex,
				Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0}},
			}},
		},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	outTex, _ := device.CreateTexture(&wgpu.TextureDescriptor{
		Size: wgpu.Extent3D{Width: W, Height: H, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	defer outTex.Release()
	outView, _ := device.CreateTextureView(outTex, nil)
	defer outView.Release()

	enc, _ := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	rp, _ := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: outView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
		}},
	})
	rp.SetPipeline(pipeline)
	rp.SetVertexBuffer(0, vbuf, 0)
	rp.SetIndexBuffer(ibuf, gputypes.IndexFormatUint16, 0)
	rp.DrawIndexed(6, 1, 0, 0, 0)
	_ = rp.End()
	enc.TransitionTextures([]wgpu.TextureBarrier{{Texture: outTex, Usage: wgpu.TextureUsageTransition{OldUsage: gputypes.TextureUsageRenderAttachment, NewUsage: gputypes.TextureUsageCopySrc}}})
	staging, _ := device.CreateBuffer(&wgpu.BufferDescriptor{Size: W * H * 4, Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst})
	defer staging.Release()
	enc.CopyTextureToBuffer(outTex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{Offset: 0, BytesPerRow: W * 4, RowsPerImage: H},
		TextureBase:  wgpu.ImageCopyTexture{Texture: outTex, MipLevel: 0},
		Size:         wgpu.Extent3D{Width: W, Height: H, DepthOrArrayLayers: 1},
	}})
	cb, _ := enc.Finish()
	q.Submit(cb)
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, W*H*4); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, _ := staging.MappedRange(0, W*H*4)
	px := rng.Bytes()
	// The full-screen quad fills over a black clear. A no-op DrawIndexed leaves
	// the frame black. Count any non-black pixel (the software backend's default
	// path fills white; backends that run the fragment shader fill green).
	filled := 0
	for i := 0; i < W*H; i++ {
		if px[i*4] > 128 || px[i*4+1] > 128 || px[i*4+2] > 128 {
			filled++
		}
	}
	staging.Unmap()
	if filled < W*H/2 {
		t.Fatalf("DrawIndexed produced %d/%d filled pixels — indexed draw did not rasterize the quad", filled, W*H)
	}
}

func binaryLEPutFloat32(b []byte, f float32) {
	bits := math.Float32bits(f)
	b[0] = byte(bits)
	b[1] = byte(bits >> 8)
	b[2] = byte(bits >> 16)
	b[3] = byte(bits >> 24)
}
