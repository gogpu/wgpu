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

// The draw-arguments scene: a 128x4 target in eight 16-pixel bands. Band b
// has its own quad at vertices 6b..6b+5. The color comes from a per-instance
// buffer and alpha is instance_index/255, so every pixel shows which
// instance, and through the color which instance element, drew it.
const (
	drawArgsWidth  = 128
	drawArgsHeight = 4
	drawArgsBands  = 8
	drawArgsBandPx = drawArgsWidth / drawArgsBands

	drawArgsClear = 0xFF000000 // cleared: black, alpha 255
	drawArgsRed   = 0x000000FF // instance 0: red, alpha 0
	drawArgsGreen = 0x0100FF00 // instance 1: green, alpha 1
	drawArgsBlue  = 0x02FF0000 // instance 2: blue, alpha 2
	drawArgsGarb  = 99         // an index that must never be drawn
)

const drawArgsShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) color: vec4f,
}

@vertex
fn vs(@location(0) p: vec2f, @location(1) color: vec4f, @builtin(instance_index) inst: u32) -> VOut {
    return VOut(vec4f(p, 0.0, 1.0), vec4f(color.rgb, f32(inst) / 255.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4f {
    return in.color;
}
`

// TestDrawIndirect checks DrawIndirect and DrawIndexedIndirect, two draws
// each from nonzero offsets: bands 2 and 3 non-indexed, bands 4 and 5 indexed
// with a first index and base vertex. The record before each offset would
// draw band 0. GLES recorded no indirect draw at all (P1-GLES-016).
func TestDrawIndirect(t *testing.T) {
	required := gputypes.DownlevelFlagsIndirectExecution | gputypes.DownlevelFlagsBaseVertex
	forEachBackendAdapter(t, required, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		got := drawArgsScene(t, device, func(s *drawArgsResources, pass *wgpu.RenderPassEncoder) {
			pass.DrawIndirect(s.indirect, 16)
			pass.DrawIndirect(s.indirect, 32)
			pass.SetIndexBuffer(s.index, gputypes.IndexFormatUint32, 0)
			pass.DrawIndexedIndirect(s.indirect, 68)
			pass.DrawIndexedIndirect(s.indirect, 88)
		})
		checkDrawArgsBands(t, got, [drawArgsBands]uint32{drawArgsClear, drawArgsClear,
			drawArgsRed, drawArgsRed, drawArgsRed, drawArgsRed, drawArgsClear, drawArgsClear})
	})
}

type drawArgsResources struct {
	index    *wgpu.Buffer
	indirect *wgpu.Buffer
}

func newDrawArgsBuffer(t *testing.T, device *wgpu.Device, usage gputypes.BufferUsage, words []uint32) *wgpu.Buffer {
	t.Helper()
	data := make([]byte, len(words)*4)
	for i, w := range words {
		binary.LittleEndian.PutUint32(data[i*4:], w)
	}
	buf := newPublicBuffer(t, device, uint64(len(data)), usage|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return buf
}

// drawArgsVertices returns the quads of the eight bands, six vertices each.
func drawArgsVertices() []float32 {
	out := make([]float32, 0, drawArgsBands*12)
	for b := range drawArgsBands {
		x0 := -1 + float32(b)*2/drawArgsBands
		x1 := x0 + 2.0/drawArgsBands
		out = append(out, x0, -1, x1, -1, x0, 1, x1, -1, x1, 1, x0, 1)
	}
	return out
}

func newDrawArgsPipeline(t *testing.T, device *wgpu.Device) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "draw-args", WGSL: drawArgsShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "draw-args"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "draw-args",
		Layout: layout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs",
			Buffers: []gputypes.VertexBufferLayout{
				{ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0},
				}},
				{ArrayStride: 16, StepMode: gputypes.VertexStepModeInstance, Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x4, ShaderLocation: 1},
				}},
			},
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// drawArgsScene clears the target, binds the scene's pipeline and vertex
// buffers, records draw and returns row 1 of the target.
func drawArgsScene(t *testing.T, device *wgpu.Device, draw func(s *drawArgsResources, pass *wgpu.RenderPassEncoder)) []uint32 {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "draw-args-target",
		Size:          wgpu.Extent3D{Width: drawArgsWidth, Height: drawArgsHeight, DepthOrArrayLayers: 1},
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
	t.Cleanup(view.Release)
	pipeline := newDrawArgsPipeline(t, device)

	positions := newVertexBuffer(t, device, drawArgsVertices())
	colors := newVertexBuffer(t, device, []float32{1, 0, 0, 1, 0, 1, 0, 1, 0, 0, 1, 1, 1, 1, 0, 1})
	s := &drawArgsResources{
		index: newDrawArgsBuffer(t, device, wgpu.BufferUsageIndex,
			[]uint32{drawArgsGarb, drawArgsGarb, 0, 1, 2, 3, 4, 5}),
		indirect: newDrawArgsBuffer(t, device, wgpu.BufferUsageIndirect, []uint32{
			// DrawIndirectArgs from byte 0: vertexCount, instanceCount, firstVertex, firstInstance.
			6, 1, 0, 0, // byte 0: would draw band 0
			6, 1, 12, 0, // byte 16: band 2
			6, 1, 18, 0, // byte 32: band 3
			// DrawIndexedIndirectArgs from byte 48: indexCount, instanceCount, firstIndex, baseVertex, firstInstance.
			6, 1, 2, 0, 0, // byte 48: would draw band 0
			6, 1, 2, 24, 0, // byte 68: band 4
			6, 1, 2, 30, 0, // byte 88: band 5
		}),
	}

	staging := newPublicBuffer(t, device, drawArgsWidth*drawArgsHeight*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, positions, 0)
	pass.SetVertexBuffer(1, colors, 0)
	draw(s, pass)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: drawArgsWidth * 4, RowsPerImage: drawArgsHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: drawArgsWidth, Height: drawArgsHeight, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	words := readPublicBuffer(t, staging, drawArgsWidth*drawArgsHeight)
	return words[drawArgsWidth : 2*drawArgsWidth]
}

// checkDrawArgsBands checks every pixel of the row against its band's word.
func checkDrawArgsBands(t *testing.T, row []uint32, want [drawArgsBands]uint32) {
	t.Helper()
	for b := range drawArgsBands {
		for x := b * drawArgsBandPx; x < (b+1)*drawArgsBandPx; x++ {
			if row[x] != want[b] {
				t.Errorf("band %d: pixel %d = %#08x, want %#08x", b, x, row[x], want[b])
				break
			}
		}
	}
}
