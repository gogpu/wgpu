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

// discardShader covers the target with one triangle and discards two regions:
// x < 16 by a constant and x > params.hi by a uniform, so the second discard
// cannot be folded away at compile time.
const discardShader = `
struct Params {
    hi: f32,
    pad0: f32,
    pad1: f32,
    pad2: f32,
}

@group(0) @binding(0) var<uniform> params: Params;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
    if pos.x < 16.0 {
        discard;
    }
    if pos.x > params.hi {
        discard;
    }
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// discardHi is the uniform bound: pixel centers x+0.5 above it are discarded.
const discardHi = 48.0

// TestFragmentDiscard checks that a discarded fragment writes nothing, on every
// backend and on each adapter the power preferences reach (integrated and
// discrete). The target is cleared to red; columns 16..47 must turn green and
// all others stay red. naga's DXIL backend emitted no dx.op.discard before
// naga P0-DXIL-068, so every column turned green under GOGPU_DX12_DXIL=1.
func TestFragmentDiscard(t *testing.T) {
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
					device, adapter := openBackendDevice(t, b, 0, p.pref)
					if seen[adapter] {
						t.Skipf("%s already checked", adapter)
					}
					seen[adapter] = true
					checkDiscardColumns(t, device)
				})
			}
		})
	}
}

func checkDiscardColumns(t *testing.T, device *wgpu.Device) {
	t.Helper()
	const width, height = 64, 4 // 256 bytes per row, the copy alignment
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "discard-target",
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

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "discard", WGSL: discardShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "discard",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageFragment, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeUniform,
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(bgl.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "discard", BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "discard",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
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

	params := newPublicBuffer(t, device, 16, wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	var data [16]byte
	binary.LittleEndian.PutUint32(data[:], math.Float32bits(discardHi))
	if err := device.Queue().WriteBuffer(params, 0, data[:]); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "discard", Layout: bgl,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: params, Size: 16}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(bg.Release)

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
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
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

	words := readPublicBuffer(t, staging, width*height)
	var got, want [width]byte
	for x := 0; x < width; x++ {
		var px [4]byte
		binary.LittleEndian.PutUint32(px[:], words[1*width+x])
		switch {
		case px[0] > 128 && px[1] < 128:
			got[x] = 'r'
		case px[0] < 128 && px[1] > 128:
			got[x] = 'g'
		default:
			got[x] = '?'
		}
		want[x] = 'r'
		if x >= 16 && float64(x)+0.5 <= discardHi {
			want[x] = 'g'
		}
	}
	if got != want {
		t.Errorf("row 1 columns (r = red/discarded, g = green):\n got  %s\n want %s\n RGBA words at x = 8, 32, 56: %#08x %#08x %#08x",
			got[:], want[:], words[width+8], words[width+32], words[width+56])
	}
}
