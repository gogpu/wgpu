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

// unormShader fills rows 0-2 of a 64-pixel-wide target with a gradient whose
// value at column i is (k + offset)/255 for an integer k, with offset 0.75,
// 0.25 and 0.6 in rows 0, 1 and 2. Rounding to nearest stores k+1, k and k+1;
// truncation stores k in every row.
const unormShader = `
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    // Pixel rows 0-2 of a 4-pixel-high target: NDC y from 1 down to -0.5.
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, 1.0), vec2<f32>(1.0, -0.5),
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, -0.5), vec2<f32>(-1.0, -0.5),
    );
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(position) pos: vec4<f32>) -> @location(0) vec4<f32> {
    let x = floor(pos.x);
    let row = u32(pos.y);
    var off = 0.75;
    if row == 1u { off = 0.25; }
    if row == 2u { off = 0.6; }
    return vec4<f32>(x + off, x + 128.0 + off, 190.0 - x + off, 255.0) / 255.0;
}
`

// The clear value of row 3: 60.7/255, 100.75/255 and 200.25/255 (61, 101 and
// 200 rounded to nearest; truncation gives 60 and 100). The exact tie 0.5
// (127.5, the only float32 that lands on a half for 8 bits) is not used: the
// D3D11 tolerance of 0.6 ULP admits either neighbor, and the GPUs differ
// there (Intel Iris Xe clears 0.5 to 128, an NVIDIA GPU to 127, on Vulkan and
// DX12 alike).
var unormClear = gputypes.Color{R: 60.7 / 255, G: 100.75 / 255, B: 200.25 / 255, A: 1}

// TestUnormWriteRounding checks the float to unorm8 conversion of a clear
// value, of an unblended fragment output and of a blended one on every
// backend. The expected bytes are the spec's (D3D11 §3.2.3.6: clamp, scale
// by 255, add 0.5 and drop the fraction; Vulkan "Conversion from
// Floating-Point to Normalized Fixed-Point": nearest integer), the same for
// every backend; the hardware
// backends are the reference. The software backend truncated, one lower on
// rows 0 and 2 and on the clear.
func TestUnormWriteRounding(t *testing.T) {
	copyBlend := gputypes.BlendComponent{
		SrcFactor: gputypes.BlendFactorOne,
		DstFactor: gputypes.BlendFactorZero,
		Operation: gputypes.BlendOperationAdd,
	}
	blends := map[string]*gputypes.BlendState{
		"unblended": nil,
		"blended":   {Color: copyBlend, Alpha: copyBlend},
	}
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		for name, blend := range blends {
			t.Run(name, func(t *testing.T) {
				words := drawUnormGradient(t, device, blend)
				bad := 0
				for y := 0; y < 4; y++ {
					for x := 0; x < 64; x++ {
						var got [4]byte
						binary.LittleEndian.PutUint32(got[:], words[y*64+x])
						if want := unormExpected(x, y); got != want {
							if bad < 8 {
								t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
							}
							bad++
						}
					}
				}
				if bad > 8 {
					t.Errorf("... %d pixels wrong in total", bad)
				}
			})
		}
	})
}

// unormExpected returns the RGBA8 bytes the spec gives for pixel (x, y).
func unormExpected(x, y int) [4]byte {
	if y == 3 {
		return [4]byte{61, 101, 200, 255}
	}
	up := 1 // offsets 0.75 and 0.6 round up
	if y == 1 {
		up = 0 // offset 0.25 rounds down
	}
	return [4]byte{byte(x + up), byte(x + 128 + up), byte(190 - x + up), 255}
}

// drawUnormGradient renders unormShader into a 64x4 RGBA8 target cleared to
// unormClear and returns the pixels.
func drawUnormGradient(t *testing.T, device *wgpu.Device, blend *gputypes.BlendState) []uint32 {
	t.Helper()
	const width, height = 64, 4 // 256 bytes per row, the copy alignment
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "unorm-target",
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

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "unorm", WGSL: unormShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "unorm-pl"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "unorm",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{{
				Format:    gputypes.TextureFormatRGBA8Unorm,
				Blend:     blend,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

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
			ClearValue: unormClear,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
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
	return readPublicBuffer(t, staging, width*height)
}
