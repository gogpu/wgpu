// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The raster rig draws one scene into RGBA32Float color targets and a
// Depth32Float attachment and reads every texel back, so a test can check
// where the rasterizer put each fragment and which position, depth and
// varyings it gave it. The color targets are cleared to rigClearColor and the
// depth to 1.

// rigClearColor is the clear value of every color target: no fragment
// shader output in these tests is negative.
const rigClearColor = -1

// rigScene describes a draw for the raster rig.
type rigScene struct {
	size      int    // the targets' width and height in pixels
	wgsl      string // the shader module
	vsEntry   string
	fsEntry   string // "" for a pipeline without a fragment stage
	targets   int    // number of RGBA32Float color targets, @location(0) up
	noDepth   bool   // no depth attachment
	depthTest gputypes.CompareFunction
	primitive gputypes.PrimitiveState
	buffers   []gputypes.VertexBufferLayout
	layouts   []*wgpu.BindGroupLayout // the pipeline layout's bind groups
	// draw records the scene's state (viewport, scissor, buffers) and draws
	// into a pass that has the scene's pipeline set.
	draw func(t *testing.T, device *wgpu.Device, pass *wgpu.RenderPassEncoder)
}

// rigImage is the read-back result of a scene: color[target][y*size+x] and
// depth[y*size+x] (nil without a depth attachment).
type rigImage struct {
	size  int
	color [][][4]float64
	depth []float64
}

func (img rigImage) at(target, x, y int) [4]float64 { return img.color[target][y*img.size+x] }

func (img rigImage) depthAt(x, y int) float64 { return img.depth[y*img.size+x] }

// rigPitch is the bytesPerRow of a size-pixel row of texelSize-byte texels:
// CopyTextureToBuffer needs a multiple of 256.
func rigPitch(size, texelSize int) int { return (size*texelSize + 255) &^ 255 }

func (s rigScene) run(t *testing.T, device *wgpu.Device) rigImage {
	t.Helper()
	pipeline := s.pipeline(t, device)
	extent := wgpu.Extent3D{Width: uint32(s.size), Height: uint32(s.size), DepthOrArrayLayers: 1}
	newTarget := func(format gputypes.TextureFormat) (*wgpu.Texture, *wgpu.TextureView) {
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label: "rig-target", Size: extent, MipLevelCount: 1, SampleCount: 1,
			Dimension: gputypes.TextureDimension2D, Format: format,
			Usage: wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		})
		if err != nil {
			t.Fatalf("CreateTexture %v: %v", format, err)
		}
		t.Cleanup(tex.Release)
		view, err := device.CreateTextureView(tex, nil)
		if err != nil {
			t.Fatalf("CreateTextureView %v: %v", format, err)
		}
		t.Cleanup(view.Release)
		return tex, view
	}

	desc := &wgpu.RenderPassDescriptor{}
	colorTex := make([]*wgpu.Texture, s.targets)
	for i := range colorTex {
		tex, view := newTarget(gputypes.TextureFormatRGBA32Float)
		colorTex[i] = tex
		desc.ColorAttachments = append(desc.ColorAttachments, wgpu.RenderPassColorAttachment{
			View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: rigClearColor, G: rigClearColor, B: rigClearColor, A: rigClearColor},
		})
	}
	var depthTex *wgpu.Texture
	if !s.noDepth {
		tex, view := newTarget(gputypes.TextureFormatDepth32Float)
		depthTex = tex
		desc.DepthStencilAttachment = &wgpu.RenderPassDepthStencilAttachment{
			View: view, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
		}
	}

	colorPitch, depthPitch := rigPitch(s.size, 16), rigPitch(s.size, 4)
	colorStaging := make([]*wgpu.Buffer, s.targets)
	for i := range colorStaging {
		colorStaging[i] = newPublicBuffer(t, device, uint64(colorPitch*s.size), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	}
	var depthStaging *wgpu.Buffer
	if depthTex != nil {
		depthStaging = newPublicBuffer(t, device, uint64(depthPitch*s.size), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	}

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	s.draw(t, device, pass)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	for i, tex := range colorTex {
		enc.CopyTextureToBuffer(tex, colorStaging[i], []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: uint32(colorPitch), RowsPerImage: uint32(s.size)},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
			Size:         extent,
		}})
	}
	if depthTex != nil {
		enc.CopyTextureToBuffer(depthTex, depthStaging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: uint32(depthPitch), RowsPerImage: uint32(s.size)},
			TextureBase:  wgpu.ImageCopyTexture{Texture: depthTex, Aspect: gputypes.TextureAspectDepthOnly},
			Size:         extent,
		}})
	}
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	img := rigImage{size: s.size}
	for _, staging := range colorStaging {
		raw := mapStaging(t, staging, uint64(colorPitch*s.size))
		texels := make([][4]float64, 0, s.size*s.size)
		for y := range s.size {
			for x := range s.size {
				texels = append(texels, decodeRGBA32F(raw[y*colorPitch+x*16:]))
			}
		}
		img.color = append(img.color, texels)
	}
	if depthStaging != nil {
		raw := mapStaging(t, depthStaging, uint64(depthPitch*s.size))
		img.depth = make([]float64, 0, s.size*s.size)
		for y := range s.size {
			for x := range s.size {
				bits := binary.LittleEndian.Uint32(raw[y*depthPitch+x*4:])
				img.depth = append(img.depth, float64(math.Float32frombits(bits)))
			}
		}
	}
	return img
}

func (s rigScene) pipeline(t *testing.T, device *wgpu.Device) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "rig", WGSL: s.wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "rig", BindGroupLayouts: s.layouts})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	var fragment *wgpu.FragmentState
	if s.fsEntry != "" {
		fragment = &wgpu.FragmentState{Module: shader, EntryPoint: s.fsEntry}
		for range s.targets {
			fragment.Targets = append(fragment.Targets,
				gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA32Float, WriteMask: gputypes.ColorWriteMaskAll})
		}
	}
	var depth *wgpu.DepthStencilState
	if !s.noDepth {
		compare := s.depthTest
		if compare == 0 {
			compare = gputypes.CompareFunctionAlways
		}
		depth = &wgpu.DepthStencilState{
			Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: true, DepthCompare: compare,
			StencilFront: wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
			StencilBack:  wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways},
		}
	}
	primitive := s.primitive
	if primitive.Topology == 0 {
		primitive.Topology = gputypes.PrimitiveTopologyTriangleList
	}
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:        "rig",
		Layout:       layout,
		Vertex:       wgpu.VertexState{Module: shader, EntryPoint: s.vsEntry, Buffers: s.buffers},
		Fragment:     fragment,
		DepthStencil: depth,
		Primitive:    primitive,
		Multisample:  gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// checkTexel reports every channel of got that differs from want by more
// than tol.
func checkTexel(t *testing.T, what string, x, y int, got, want [4]float64, tol float64) {
	t.Helper()
	for ch := range 4 {
		if !(math.Abs(got[ch]-want[ch]) <= tol) {
			t.Errorf("%s pixel (%d, %d) = %v, want %v (±%g)", what, x, y, got, want, tol)
			return
		}
	}
}

// checkDepth reports a depth that differs from want by more than tol.
func checkDepth(t *testing.T, x, y int, got, want, tol float64) {
	t.Helper()
	if !(math.Abs(got-want) <= tol) {
		t.Errorf("depth pixel (%d, %d) = %g, want %g (±%g)", x, y, got, want, tol)
	}
}
