// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The fill rule readback tests draw tilings with premultiplied alpha 0.5
// source-over blending. A pixel on an edge shared by two triangles must be
// shaded by exactly one of them (Vulkan "Basic Polygon Rasterization", D3D
// rasterization rules); if both shade it, it is blended twice. The hardware
// backends are the reference: the expected image is the same for every
// backend. Reported on the software adapter as a brighter diagonal across a
// half-transparent textured quad (gogpu examples/texture).

const fillTarget = 64 // 64x64 RGBA8: 256 bytes per row, the copy alignment

// Clear 40, source 100 premultiplied with alpha 0.5: one blend gives
// 100 + 0.5*40 = 120, a second blend over that 100 + 0.5*120 = 160.
const (
	fillClear  = 40
	fillSource = 100
	fillOnce   = 120
)

// fillPoint is a vertex position in framebuffer pixels (origin top-left).
type fillPoint struct{ x, y float32 }

// fillRuleShader returns a shader that draws the triangle list tris (three
// points each) from vertex_index in a constant color.
func fillRuleShader(tris []fillPoint) string {
	var b strings.Builder
	ndc := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	fmt.Fprintf(&b, "@vertex\nfn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {\n")
	fmt.Fprintf(&b, "    var p = array<vec2<f32>, %d>(\n", len(tris))
	for _, p := range tris {
		// Pixel to NDC: exact for these coordinates (multiples of 1/2 over 32).
		fmt.Fprintf(&b, "        vec2<f32>(%s, %s),\n", ndc(p.x/(fillTarget/2)-1), ndc(1-p.y/(fillTarget/2)))
	}
	fmt.Fprintf(&b, "    );\n    return vec4<f32>(p[i], 0.0, 1.0);\n}\n\n")
	fmt.Fprintf(&b, "@fragment\nfn fs() -> @location(0) vec4<f32> {\n")
	fmt.Fprintf(&b, "    let c = %d.0 / 255.0;\n    return vec4<f32>(c, c, c, 0.5);\n}\n", fillSource)
	return b.String()
}

// fillWindings returns the triangle list as given and with every other
// triangle reversed; coverage must not depend on winding.
func fillWindings(tris []fillPoint) map[string][]fillPoint {
	mixed := append([]fillPoint(nil), tris...)
	for i := 3; i+2 < len(mixed); i += 6 {
		mixed[i+1], mixed[i+2] = mixed[i+2], mixed[i+1]
	}
	reversed := append([]fillPoint(nil), tris...)
	for i := 0; i+2 < len(reversed); i += 3 {
		reversed[i+1], reversed[i+2] = reversed[i+2], reversed[i+1]
	}
	return map[string][]fillPoint{"given": tris, "mixed": mixed, "reversed": reversed}
}

// TestFillRuleQuadDiagonal draws the pixel rectangle [8,56)x[8,56) as two
// triangles sharing a diagonal. The diagonal runs through pixel centers; the
// outer edges are on pixel boundaries, so only the shared edge needs the fill
// rule.
func TestFillRuleQuadDiagonal(t *testing.T) {
	quads := map[string][]fillPoint{
		"tlbr": {{8, 8}, {56, 8}, {56, 56}, {8, 8}, {56, 56}, {8, 56}},
		"trbl": {{8, 8}, {56, 8}, {8, 56}, {56, 8}, {56, 56}, {8, 56}},
	}
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		for qname, quad := range quads {
			for wname, tris := range fillWindings(quad) {
				t.Run(qname+"/"+wname, func(t *testing.T) {
					checkFillImage(t, drawFillTriangles(t, device, tris))
				})
			}
		}
	})
}

// TestFillRuleTriangleFan draws [8,56)x[8,56) as a fan of eight triangles
// around a center vertex at the pixel center (32.5, 32.5). The horizontal,
// vertical and main-diagonal spokes run through pixel centers, and the center
// pixel lies on all eight triangles.
func TestFillRuleTriangleFan(t *testing.T) {
	c := fillPoint{32.5, 32.5}
	ring := []fillPoint{{8, 8}, {32.5, 8}, {56, 8}, {56, 32.5}, {56, 56}, {32.5, 56}, {8, 56}, {8, 32.5}}
	fan := make([]fillPoint, 0, 3*len(ring))
	for i := range ring {
		fan = append(fan, c, ring[i], ring[(i+1)%len(ring)])
	}
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		for wname, tris := range fillWindings(fan) {
			t.Run(wname, func(t *testing.T) {
				checkFillImage(t, drawFillTriangles(t, device, tris))
			})
		}
	})
}

// checkFillImage expects every pixel of [8,56)x[8,56) blended exactly once
// and every other pixel left at the clear color. One step of tolerance covers
// the backends' unorm8 rounding.
func checkFillImage(t *testing.T, words []uint32) {
	t.Helper()
	near := func(got byte, want int) bool { d := int(got) - want; return d >= -1 && d <= 1 }
	bad := 0
	for y := 0; y < fillTarget; y++ {
		for x := 0; x < fillTarget; x++ {
			want := fillClear
			if x >= 8 && x < 56 && y >= 8 && y < 56 {
				want = fillOnce
			}
			var px [4]byte
			binary.LittleEndian.PutUint32(px[:], words[y*fillTarget+x])
			if near(px[0], want) && near(px[1], want) && near(px[2], want) && near(px[3], 255) {
				continue
			}
			if bad < 8 {
				t.Errorf("pixel (%d,%d) = %v, want (%d, %d, %d, 255) ±1", x, y, px, want, want, want)
			}
			bad++
		}
	}
	if bad > 8 {
		t.Errorf("... %d pixels wrong in total", bad)
	}
}

// drawFillTriangles renders tris (pixel coordinates) into a cleared 64x64
// RGBA8 target with premultiplied source-over blending and returns the
// pixels.
func drawFillTriangles(t *testing.T, device *wgpu.Device, tris []fillPoint) []uint32 {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "fill-target",
		Size:          wgpu.Extent3D{Width: fillTarget, Height: fillTarget, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "fill", WGSL: fillRuleShader(tris)})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "fill-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	premultiplied := gputypes.BlendComponent{
		SrcFactor: gputypes.BlendFactorOne,
		DstFactor: gputypes.BlendFactorOneMinusSrcAlpha,
		Operation: gputypes.BlendOperationAdd,
	}
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "fill",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{{
				Format:    gputypes.TextureFormatRGBA8Unorm,
				Blend:     &gputypes.BlendState{Color: premultiplied, Alpha: premultiplied},
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	staging := newPublicBuffer(t, device, fillTarget*fillTarget*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	const clearValue = float64(fillClear) / 255
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: clearValue, G: clearValue, B: clearValue, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: uint32(len(tris)), InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: fillTarget * 4, RowsPerImage: fillTarget},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: fillTarget, Height: fillTarget, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return readPublicBuffer(t, staging, fillTarget*fillTarget)
}
