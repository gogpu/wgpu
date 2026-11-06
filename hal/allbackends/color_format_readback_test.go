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

// The color format tests render a full-target quad into a small target and
// read it back with CopyTextureToBuffer. Every backend must store the
// fragment value in the target's own texel layout, as Vulkan and D3D12 do.

const colorRigSize = 4

// colorRigPitch is the copy row pitch: 4 texels of the widest format (16
// bytes) fit, and 256 is the copy row alignment.
const colorRigPitch = 256

// colorQuadShader draws the whole target with fragment value value of WGSL
// type outType. extra is WGSL placed before the entry points.
func colorQuadShader(extra, outType, value string) string {
	return fmt.Sprintf(`
%s
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(position) pos: vec4<f32>) -> @location(0) %s {
    return %s;
}
`, extra, outType, value)
}

// colorPass is one render pass of the color tests: a quad drawn with wgsl
// into view, whose pipeline has target state state.
type colorPass struct {
	view          *wgpu.TextureView
	wgsl          string
	state         gputypes.ColorTargetState
	clear         gputypes.Color
	blendConstant *gputypes.Color
	layout        *wgpu.BindGroupLayout
	group         *wgpu.BindGroup
}

func (c colorPass) run(t *testing.T, device *wgpu.Device) {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "color-quad", WGSL: c.wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	var layouts []*wgpu.BindGroupLayout
	if c.layout != nil {
		layouts = append(layouts, c.layout)
	}
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "color-quad", BindGroupLayouts: layouts})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:       "color-quad",
		Layout:      layout,
		Vertex:      wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment:    &wgpu.FragmentState{Module: shader, EntryPoint: "fs", Targets: []gputypes.ColorTargetState{c.state}},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: c.view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore, ClearValue: c.clear,
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		pass.SetPipeline(pipeline)
		if c.group != nil {
			pass.SetBindGroup(0, c.group, nil)
		}
		if c.blendConstant != nil {
			pass.SetBlendConstant(c.blendConstant)
		}
		pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	})
}

// newColorTexture creates a colorRigSize square texture of format, with
// viewFormats, and its default view.
func newColorTexture(t *testing.T, device *wgpu.Device, format gputypes.TextureFormat, viewFormats []gputypes.TextureFormat, usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "color-target",
		Size:          wgpu.Extent3D{Width: colorRigSize, Height: colorRigSize, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         usage,
		ViewFormats:   viewFormats,
	})
	if err != nil {
		t.Fatalf("CreateTexture %v: %v", format, err)
	}
	t.Cleanup(tex.Release)
	return tex, newFormatView(t, device, tex, gputypes.TextureFormatUndefined)
}

// newFormatView creates a view of tex in format (Undefined: the texture's).
func newFormatView(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, format gputypes.TextureFormat) *wgpu.TextureView {
	t.Helper()
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label: "color-view", Format: format, Dimension: gputypes.TextureViewDimension2D,
		MipLevelCount: 1, ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView %v: %v", format, err)
	}
	t.Cleanup(view.Release)
	return view
}

// readColorTexels copies tex to a buffer and returns its texels, texelSize
// bytes each, tightly packed in row order.
func readColorTexels(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, texelSize int) [][]byte {
	t.Helper()
	staging := newPublicBuffer(t, device, colorRigPitch*colorRigSize, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: colorRigPitch, RowsPerImage: colorRigSize},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
			Size:         wgpu.Extent3D{Width: colorRigSize, Height: colorRigSize, DepthOrArrayLayers: 1},
		}})
	})
	words := readPublicBuffer(t, staging, colorRigPitch*colorRigSize/4)
	raw := make([]byte, len(words)*4)
	for i, w := range words {
		binary.LittleEndian.PutUint32(raw[i*4:], w)
	}
	texels := make([][]byte, 0, colorRigSize*colorRigSize)
	for y := 0; y < colorRigSize; y++ {
		for x := 0; x < colorRigSize; x++ {
			off := y*colorRigPitch + x*texelSize
			texels = append(texels, raw[off:off+texelSize])
		}
	}
	return texels
}

func le16(v ...uint16) []byte {
	b := make([]byte, 2*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint16(b[2*i:], x)
	}
	return b
}

func le32(v ...uint32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], x)
	}
	return b
}

func f32bits(v ...float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func pack1010102(r, g, b, a uint32) []byte { return le32(r | g<<10 | b<<20 | a<<30) }

// srgbEncode8 is the 8-bit sRGB encoding of linear value v (IEC 61966-2-1).
func srgbEncode8(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v * 255
	}
	return (1.055*math.Pow(v, 1/2.4) - 0.055) * 255
}

// srgbDecode is the linear value of 8-bit sRGB code c.
func srgbDecode(c byte) float64 {
	v := float64(c) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

type colorFormatCase struct {
	format  gputypes.TextureFormat
	outType string
	value   string
	want    []byte
	// tolerance is the allowed difference per byte: the sRGB conversion is
	// approximate on GPUs.
	tolerance int
}

const (
	unormValue = "vec4<f32>(0.2, 0.4, 0.6, 1.0)"
	floatValue = "vec4<f32>(0.25, 0.5, 0.75, 1.0)"
	u8Value    = "vec4<u32>(7u, 200u, 3u, 255u)"
	s8Value    = "vec4<i32>(-7, 100, -128, 127)"
	u16Value   = "vec4<u32>(65000u, 1234u, 7u, 1u)"
	s16Value   = "vec4<i32>(-30000, 1234, -7, 1)"
	u32Value   = "vec4<u32>(16000000u, 7u, 3u, 1u)"
	s32Value   = "vec4<i32>(-16000000, 7, -3, 1)"
)

// colorFormatCases are the color formats WebGPU renders without a feature.
// The 32-bit integer vectors stay below 2^24 here; colorWideIntCases hold the
// values above.
var colorFormatCases = []colorFormatCase{
	{gputypes.TextureFormatR8Unorm, "vec4<f32>", unormValue, []byte{51}, 0},
	{gputypes.TextureFormatRG8Unorm, "vec4<f32>", unormValue, []byte{51, 102}, 0},
	{gputypes.TextureFormatRGBA8Unorm, "vec4<f32>", unormValue, []byte{51, 102, 153, 255}, 0},
	{gputypes.TextureFormatBGRA8Unorm, "vec4<f32>", unormValue, []byte{153, 102, 51, 255}, 0},
	{gputypes.TextureFormatRGBA8UnormSrgb, "vec4<f32>", unormValue, []byte{124, 170, 203, 255}, 1},
	{gputypes.TextureFormatBGRA8UnormSrgb, "vec4<f32>", unormValue, []byte{203, 170, 124, 255}, 1},
	{gputypes.TextureFormatRGB10A2Unorm, "vec4<f32>", unormValue, pack1010102(205, 409, 614, 3), 0},
	{gputypes.TextureFormatR16Float, "vec4<f32>", floatValue, le16(0x3400), 0},
	{gputypes.TextureFormatRG16Float, "vec4<f32>", floatValue, le16(0x3400, 0x3800), 0},
	{gputypes.TextureFormatRGBA16Float, "vec4<f32>", floatValue, le16(0x3400, 0x3800, 0x3A00, 0x3C00), 0},
	{gputypes.TextureFormatR32Float, "vec4<f32>", floatValue, f32bits(0.25), 0},
	{gputypes.TextureFormatRG32Float, "vec4<f32>", floatValue, f32bits(0.25, 0.5), 0},
	{gputypes.TextureFormatRGBA32Float, "vec4<f32>", floatValue, f32bits(0.25, 0.5, 0.75, 1), 0},
	{gputypes.TextureFormatR8Uint, "vec4<u32>", u8Value, []byte{7}, 0},
	{gputypes.TextureFormatRG8Uint, "vec4<u32>", u8Value, []byte{7, 200}, 0},
	{gputypes.TextureFormatRGBA8Uint, "vec4<u32>", u8Value, []byte{7, 200, 3, 255}, 0},
	{gputypes.TextureFormatR8Sint, "vec4<i32>", s8Value, []byte{0xF9}, 0},
	{gputypes.TextureFormatRG8Sint, "vec4<i32>", s8Value, []byte{0xF9, 100}, 0},
	{gputypes.TextureFormatRGBA8Sint, "vec4<i32>", s8Value, []byte{0xF9, 100, 0x80, 127}, 0},
	{gputypes.TextureFormatR16Uint, "vec4<u32>", u16Value, le16(65000), 0},
	{gputypes.TextureFormatRG16Uint, "vec4<u32>", u16Value, le16(65000, 1234), 0},
	{gputypes.TextureFormatRGBA16Uint, "vec4<u32>", u16Value, le16(65000, 1234, 7, 1), 0},
	{gputypes.TextureFormatR16Sint, "vec4<i32>", s16Value, le16(0x8AD0), 0},
	{gputypes.TextureFormatRG16Sint, "vec4<i32>", s16Value, le16(0x8AD0, 1234), 0},
	{gputypes.TextureFormatRGBA16Sint, "vec4<i32>", s16Value, le16(0x8AD0, 1234, 0xFFF9, 1), 0},
	{gputypes.TextureFormatR32Uint, "u32", "0xDEADBEEFu", le32(0xDEADBEEF), 0},
	{gputypes.TextureFormatR32Sint, "i32", "-123456789", le32(0xF8A432EB), 0},
	{gputypes.TextureFormatRG32Uint, "vec4<u32>", u32Value, le32(16000000, 7), 0},
	{gputypes.TextureFormatRGBA32Uint, "vec4<u32>", u32Value, le32(16000000, 7, 3, 1), 0},
	{gputypes.TextureFormatRG32Sint, "vec4<i32>", s32Value, le32(0xFF0BDC00, 7), 0},
	{gputypes.TextureFormatRGBA32Sint, "vec4<i32>", s32Value, le32(0xFF0BDC00, 7, 0xFFFFFFFD, 1), 0},
	{gputypes.TextureFormatRGB10A2Uint, "vec4<u32>", "vec4<u32>(1000u, 7u, 512u, 2u)", pack1010102(1000, 7, 512, 2), 0},
}

// colorWideIntCases write 32-bit integer vectors above 2^24.
var colorWideIntCases = []colorFormatCase{
	{gputypes.TextureFormatRGBA32Uint, "vec4<u32>", "vec4<u32>(0xDEADBEEFu, 16777217u, 4000000000u, 1u)",
		le32(0xDEADBEEF, 16777217, 4000000000, 1), 0},
	{gputypes.TextureFormatRGBA32Sint, "vec4<i32>", "vec4<i32>(-123456789, 16777217, 2147483647, -2147483648)",
		le32(0xF8A432EB, 16777217, 0x7FFFFFFF, 0x80000000), 0},
}

// swWideIntSkip: the software interpreter holds integer vectors as float32
// (shader.Value TagVec*), exact only up to 2^24.
const swWideIntSkip = "software: integer vectors above 2^24 are float-held in the interpreter, P1-SW-019"

// TestRenderTargetFormats draws a constant into a target of every color
// format and checks the stored texels. The software backend stored every
// target as 8-bit RGBA, so other formats received bytes of the wrong layout,
// and an sRGB target got the linear value.
func TestRenderTargetFormats(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		for _, c := range colorFormatCases {
			t.Run(c.format.String(), func(t *testing.T) {
				checkColorFormat(t, b, device, c)
			})
		}
		for _, c := range colorWideIntCases {
			t.Run(c.format.String()+"-wide", func(t *testing.T) {
				if b.software {
					t.Skip(swWideIntSkip)
				}
				checkColorFormat(t, b, device, c)
			})
		}
	})
}

func checkColorFormat(t *testing.T, b overrideBackend, device *wgpu.Device, c colorFormatCase) {
	t.Helper()
	tex, view := newColorTexture(t, device, c.format, nil, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	colorPass{
		view:  view,
		wgsl:  colorQuadShader("", c.outType, c.value),
		state: gputypes.ColorTargetState{Format: c.format, WriteMask: gputypes.ColorWriteMaskAll},
	}.run(t, device)
	for i, got := range readColorTexels(t, device, tex, len(c.want)) {
		if !bytesNear(got, c.want, c.tolerance) {
			t.Fatalf("texel %d = %x, want %x", i, got, c.want)
		}
	}
}

func bytesNear(got, want []byte, tolerance int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if d := int(got[i]) - int(want[i]); d > tolerance || d < -tolerance {
			return false
		}
	}
	return true
}

// TestSrgbBlend blends a half-transparent color over an sRGB target. WebGPU
// blends sRGB targets in linear space: the stored value is decoded, blended
// and encoded again. The software backend blended the encoded bytes.
func TestSrgbBlend(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		format := gputypes.TextureFormatRGBA8UnormSrgb
		tex, view := newColorTexture(t, device, format, nil, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		over := gputypes.BlendComponent{
			SrcFactor: gputypes.BlendFactorSrcAlpha, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha,
			Operation: gputypes.BlendOperationAdd,
		}
		colorPass{
			view:  view,
			wgsl:  colorQuadShader("", "vec4<f32>", "vec4<f32>(1.0, 0.0, 0.5, 0.5)"),
			state: gputypes.ColorTargetState{Format: format, WriteMask: gputypes.ColorWriteMaskAll, Blend: &gputypes.BlendState{Color: over, Alpha: over}},
			clear: gputypes.Color{R: 0.2, G: 0.2, B: 0.2, A: 1},
		}.run(t, device)

		// The clear stores encode(0.2); blending decodes that code.
		dst := srgbDecode(byte(math.Round(srgbEncode8(0.2))))
		want := []byte{
			byte(math.Round(srgbEncode8(0.5*1 + 0.5*dst))),
			byte(math.Round(srgbEncode8(0.5 * dst))),
			byte(math.Round(srgbEncode8(0.5*0.5 + 0.5*dst))),
			byte(math.Round((0.5*0.5 + 0.5*1) * 255)),
		}
		for i, got := range readColorTexels(t, device, tex, 4) {
			if !bytesNear(got, want, 1) {
				t.Fatalf("texel %d = %v, want %v (linear blend)", i, got, want)
			}
		}
	})
}

// TestBlendConstantAndWriteMask blends with the pass's blend constant and
// writes red, green and alpha only: blue keeps the cleared value.
func TestBlendConstantAndWriteMask(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		format := gputypes.TextureFormatRGBA8Unorm
		tex, view := newColorTexture(t, device, format, nil, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		byConstant := gputypes.BlendComponent{
			SrcFactor: gputypes.BlendFactorConstant, DstFactor: gputypes.BlendFactorOneMinusConstant,
			Operation: gputypes.BlendOperationAdd,
		}
		colorPass{
			view: view,
			wgsl: colorQuadShader("", "vec4<f32>", "vec4<f32>(1.0, 1.0, 1.0, 1.0)"),
			state: gputypes.ColorTargetState{
				Format:    format,
				WriteMask: gputypes.ColorWriteMaskRed | gputypes.ColorWriteMaskGreen | gputypes.ColorWriteMaskAlpha,
				Blend:     &gputypes.BlendState{Color: byConstant, Alpha: byConstant},
			},
			clear:         gputypes.Color{R: 0.2, G: 0.4, B: 0.6, A: 0.2},
			blendConstant: &gputypes.Color{R: 0.5, G: 0.25, B: 0.75, A: 1},
		}.run(t, device)

		// red 0.5 + 0.5*0.2, green 0.25 + 0.75*0.4, blue kept, alpha 1.
		want := []byte{153, 140, 153, 255}
		for i, got := range readColorTexels(t, device, tex, 4) {
			if !bytesNear(got, want, 1) {
				t.Fatalf("texel %d = %v, want %v", i, got, want)
			}
		}
	})
}
