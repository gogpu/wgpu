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

// broadcastShader takes its position per vertex (location 0), one color per
// vertex buffer at arrayStride 0 stepped per vertex (location 1) and one at
// arrayStride 0 stepped per instance (location 2), and outputs
// (a.r, a.g, b.b, 1).
const broadcastShader = `
struct VOut {
    @builtin(position) pos: vec4<f32>,
    @location(0) color: vec4<f32>,
}

@vertex
fn vs(@location(0) p: vec2<f32>, @location(1) a: vec4<f32>, @location(2) b: vec4<f32>) -> VOut {
    return VOut(vec4<f32>(p, 0.0, 1.0), vec4<f32>(a.r, a.g, b.b, 1.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    return in.color;
}
`

// TestVertexBufferStrideZero checks arrayStride 0 (WebGPU's broadcast: every
// vertex or instance reads element 0) on every backend and on each adapter the
// power preferences reach. Two instances of one full-target triangle read
// their color from two arrayStride 0 buffers, one stepped per vertex and one
// per instance, whose element 0 holds the expected color and whose elements 1
// and 2 hold a wrong one. Every pixel must be (0, 255, 255, 255): a backend
// that reads the buffers as tightly packed picks up the wrong elements for
// vertex 1 and 2 or for instance 1, and one that drops the draw leaves the
// black clear color.
func TestVertexBufferStrideZero(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		checkVertexBroadcast(t, device, false)
	})
}

// TestVertexBufferBeforePipeline draws the same scene as
// TestVertexBufferStrideZero with the vertex buffers bound before the
// pipeline, which WebGPU allows. DX12 took the stride and GLES the attribute
// layout from the pipeline current at SetVertexBuffer time, so this order drew
// nothing on either until they kept the bindings and set them up at the draw.
func TestVertexBufferBeforePipeline(t *testing.T) {
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		checkVertexBroadcast(t, device, true)
	})
}

// forEachBackendAdapter runs fn with a device of every backend on each
// adapter the low-power and high-performance preferences select, so a hybrid
// machine checks both GPUs. A backend without such an adapter, or whose
// adapter lacks a required downlevel flag, is skipped.
func forEachBackendAdapter(t *testing.T, required gputypes.DownlevelFlags, fn func(t *testing.T, b overrideBackend, device *wgpu.Device)) {
	t.Helper()
	for _, b := range overrideBackends {
		t.Run(string(b.name), func(t *testing.T) {
			if b.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			seen := map[string]bool{}
			for _, p := range []struct {
				name string
				pref gputypes.PowerPreference
			}{
				{"low-power", gputypes.PowerPreferenceLowPower},
				{"high-performance", gputypes.PowerPreferenceHighPerformance},
			} {
				t.Run(p.name, func(t *testing.T) {
					device, adapter := openBackendDevice(t, b, required, p.pref)
					if seen[adapter] {
						t.Skipf("%s already checked", adapter)
					}
					seen[adapter] = true
					fn(t, b, device)
				})
			}
		})
	}
}

// newVertexBuffer creates a vertex buffer holding the float32 values data.
func newVertexBuffer(t *testing.T, device *wgpu.Device, data []float32) *wgpu.Buffer {
	t.Helper()
	bytes := make([]byte, len(data)*4)
	for i, f := range data {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(f))
	}
	buf := newPublicBuffer(t, device, uint64(len(bytes)), wgpu.BufferUsageVertex|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(buf, 0, bytes); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return buf
}

func newBroadcastPipeline(t *testing.T, device *wgpu.Device) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "broadcast", WGSL: broadcastShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "broadcast"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	color := func(loc uint32) []gputypes.VertexAttribute {
		return []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x4, ShaderLocation: loc}}
	}
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "broadcast",
		Layout: layout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs",
			Buffers: []gputypes.VertexBufferLayout{
				{ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0},
				}},
				{ArrayStride: 0, StepMode: gputypes.VertexStepModeVertex, Attributes: color(1)},
				{ArrayStride: 0, StepMode: gputypes.VertexStepModeInstance, Attributes: color(2)},
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

// checkVertexBroadcast draws and checks the broadcast scene; buffersFirst binds
// the vertex buffers before the pipeline.
func checkVertexBroadcast(t *testing.T, device *wgpu.Device, buffersFirst bool) {
	const width, height = 64, 4 // 256 bytes per row, the copy alignment
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "broadcast-target",
		Size:          wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
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
	pipeline := newBroadcastPipeline(t, device)

	positions := newVertexBuffer(t, device, []float32{-1, -1, 3, -1, -1, 3})
	// Element 0 is the color every vertex and instance must read; elements 1
	// and 2 are what a tightly packed read would return.
	perVertex := newVertexBuffer(t, device, []float32{0, 1, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1})
	perInstance := newVertexBuffer(t, device, []float32{0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 1})

	staging := newPublicBuffer(t, device, width*height*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
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
	if !buffersFirst {
		pass.SetPipeline(pipeline)
	}
	pass.SetVertexBuffer(0, positions, 0)
	pass.SetVertexBuffer(1, perVertex, 0)
	pass.SetVertexBuffer(2, perInstance, 0)
	if buffersFirst {
		pass.SetPipeline(pipeline)
	}
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 2})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: width * 4, RowsPerImage: height},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	const want = 0xFFFFFF00 // RGBA8 (0, 255, 255, 255) read as a little-endian word
	words := readPublicBuffer(t, staging, width*height)
	bad := 0
	for i, w := range words {
		if w != want {
			if bad == 0 {
				t.Errorf("pixel (%d, %d) = %#08x, want %#08x", i%width, i/width, w, uint32(want))
			}
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d pixels wrong; RGBA words at (0,0), (32,1), (63,3): %#08x %#08x %#08x",
			bad, len(words), words[0], words[width+32], words[len(words)-1])
	}
}
