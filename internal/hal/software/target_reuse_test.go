//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package software

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// quadDraw is one positioned-quad render setup: the pipeline, the shared
// uniform bind group, and one texture bind group per solid-color source
// texture. A draw's viewport clips the fullscreen quad to its cell.
type quadDraw struct {
	pipeline hal.RenderPipeline
	uniform  hal.BindGroup
	textures []hal.BindGroup
}

// newQuadDraw builds a positioned-quad pipeline of a w x h target with the
// depth-stencil state ds (nil for none) and one source texture per color.
func newQuadDraw(t testing.TB, dev *Device, w, h int, format gputypes.TextureFormat, ds *hal.DepthStencilState, colors ...[4]byte) *quadDraw {
	t.Helper()
	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "quad-shader", Source: hal.ShaderSource{WGSL: positionedQuadWGSL}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	pipeline, err := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:        "quad-shader",
		Vertex:       hal.VertexState{Module: sm, EntryPoint: "vs_main"},
		Primitive:    gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		DepthStencil: ds,
		Fragment: &hal.FragmentState{
			Module: sm, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{Format: format, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}

	// One quad covering the whole screen; each draw's viewport clips it.
	uniforms := make([]byte, 32)
	for i, f := range []float32{0, 0, float32(w), float32(h), float32(w), float32(h), 1, 1} { // rect, screen, alpha, premultiplied
		binary.LittleEndian.PutUint32(uniforms[i*4:], math.Float32bits(f))
	}
	ubuf, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 32, Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	ubuf.(*Buffer).WriteData(0, uniforms)
	samp, err := dev.CreateSampler(&hal.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}

	q := &quadDraw{
		pipeline: pipeline,
		uniform:  newShaderBindGroup(t, dev, gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.BufferBinding{Buffer: ubuf.NativeHandle(), Size: 32}}),
	}
	for _, c := range colors {
		_, srcView := newShaderTexture(t, dev, uint32(w), uint32(h), gputypes.TextureFormatRGBA8Unorm, c)
		q.textures = append(q.textures, newShaderBindGroup(t, dev,
			gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.SamplerBinding{Sampler: samp.NativeHandle()}},
			gputypes.BindGroupEntry{Binding: 1, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
		))
	}
	return q
}

// TestPassDrawsReuseOneRasterPipeline proves that the pipeline a pass reuses
// for all its draws gives the same image as a fresh one per draw (#419): a
// 16x16 draw keeps its own viewport and state whatever the draws before it
// did. Each cell of the shared pass must equal the same draw alone in a pass.
func TestPassDrawsReuseOneRasterPipeline(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	const w, h = 128, 96
	q := newQuadDraw(t, dev, w, h, gputypes.TextureFormatBGRA8Unorm, nil, [4]byte{200, 50, 10, 255})
	dst, dstView := newShaderTexture(t, dev, w, h, gputypes.TextureFormatBGRA8Unorm, [4]byte{})
	cells := []gputypes.Viewport{
		{X: 0, Y: 0, Width: 16, Height: 16, MinDepth: 0, MaxDepth: 1},
		{X: 32, Y: 0, Width: 16, Height: 16, MinDepth: 0, MaxDepth: 1},
		{X: 64, Y: 32, Width: 16, Height: 16, MinDepth: 0, MaxDepth: 1},
	}

	// draw records one command buffer with one pass per group of cells, the
	// cells of a group drawn by the same pass, and returns the target's image.
	draw := func(groups ...[]gputypes.Viewport) []byte {
		enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		execs := make([]*RenderPassEncoder, 0, len(groups))
		for _, group := range groups {
			pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
				ColorAttachments: []hal.RenderPassColorAttachment{{
					View: dstView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					ClearValue: gputypes.Color{A: 1},
				}},
			})
			pass.SetPipeline(q.pipeline)
			pass.SetBindGroup(0, q.uniform, nil)
			pass.SetBindGroup(1, q.textures[0], nil)
			for _, cell := range group {
				pass.SetViewport(cell)
				pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
			}
			pass.End()
			execs = append(execs, pass.(*RenderPassEncoder))
		}
		submit(t, enc)
		for _, exec := range execs {
			if exec.drawPipe == nil {
				t.Fatal("the pass did not keep a raster pipeline for its draws")
			}
		}
		return dst.(*Texture).GetData()
	}

	shared := draw(cells)
	for _, cell := range cells {
		alone := draw([]gputypes.Viewport{cell})
		x0, y0 := int(cell.X), int(cell.Y)
		row := 16 * 4 // the quad's width in bytes
		for y := y0; y < y0+16; y++ {
			off := (y*w + x0) * 4
			if !bytes.Equal(shared[off:off+row], alone[off:off+row]) {
				t.Fatalf("the 16x16 cell at %d,%d differs from the same draw in a pass of its own", x0, y0)
			}
		}
	}
}

// TestPassDrawStateDoesNotLeakBetweenDraws proves the reused pipeline is reset
// for every draw of the pass: a pipeline with a depth-stencil state draws
// first, and the pipeline without one that follows in the same pass must not
// inherit its depth test. Both draw the same cell, so the second one's color
// must be the one left on screen.
func TestPassDrawStateDoesNotLeakBetweenDraws(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	const w, h = 64, 64
	ds := &hal.DepthStencilState{
		Format:            gputypes.TextureFormatDepth24PlusStencil8,
		DepthWriteEnabled: true,
		DepthCompare:      gputypes.CompareFunctionLess,
	}
	depthDraw := newQuadDraw(t, dev, w, h, gputypes.TextureFormatBGRA8Unorm, ds, [4]byte{255, 0, 0, 255})
	plainDraw := newQuadDraw(t, dev, w, h, gputypes.TextureFormatBGRA8Unorm, nil, [4]byte{0, 255, 0, 255})
	dst, dstView := newShaderTexture(t, dev, w, h, gputypes.TextureFormatBGRA8Unorm, [4]byte{})
	depthTex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatDepth24PlusStencil8,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	depthView, err := dev.CreateTextureView(depthTex, &hal.TextureViewDescriptor{})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	cell := gputypes.Viewport{X: 0, Y: 0, Width: 16, Height: 16, MinDepth: 0, MaxDepth: 1}

	// draw runs one pass that draws each setup in order into the same cell.
	draw := func(setups ...*quadDraw) []byte {
		enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
			ColorAttachments: []hal.RenderPassColorAttachment{{
				View: dstView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{A: 1},
			}},
			DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
				View:            depthView,
				DepthLoadOp:     gputypes.LoadOpClear,
				DepthClearValue: 1.0,
				StencilLoadOp:   gputypes.LoadOpClear,
			},
		})
		for _, s := range setups {
			pass.SetPipeline(s.pipeline)
			pass.SetBindGroup(0, s.uniform, nil)
			pass.SetBindGroup(1, s.textures[0], nil)
			pass.SetViewport(cell)
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		}
		pass.End()
		submit(t, enc)
		return dst.(*Texture).GetData()
	}

	after := draw(depthDraw, plainDraw)
	alone := draw(plainDraw)
	row := 16 * 4
	for y := 0; y < 16; y++ {
		off := y * w * 4
		if !bytes.Equal(after[off:off+row], alone[off:off+row]) {
			t.Fatalf("row %d of the draw after a depth-tested one differs from the same draw in a pass of its own: it inherited the render state", y)
		}
	}
	// BGRA8: the second draw's solid green, not the first one's red.
	if b, g, r, _ := after[0], after[1], after[2], after[3]; b != 0 || g != 255 || r != 0 {
		t.Errorf("cell pixel = (B %d, G %d, R %d), want the second draw's green (0, 255, 0)", b, g, r)
	}
}
