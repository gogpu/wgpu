// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// samplerSwitchShader samples texture t with the named sampler at u = 1.125,
// past the right edge, and draws a full-height quad over [xLo, xHi] in NDC x.
// A clamping sampler reads the last column, a repeating one the first.
func samplerSwitchShader(sampler string, xLo, xHi float32) string {
	return fmt.Sprintf(`
@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var s_clamp: sampler;
@group(0) @binding(2) var s_repeat: sampler;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    let c = quad[v %% 6u];
    return VOut(vec4<f32>(select(%[2]f, %[3]f, c.x > 0.0), c.y, 0.0, 1.0));
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return textureSample(t, %[1]s, vec2<f32>(1.125, 0.5));
}
`, sampler, xLo, xHi)
}

// TestSamplerAcrossPipelineSwitch draws with two pipelines of one layout that
// pair the texture with different samplers, without setting the bind group
// again in between (core keeps it: the layout is compatible). GLES binds a
// sampler to the texture unit of the texture it is combined with, which
// depends on the pipeline's shader, so the backend must rebind the samplers
// when the pipeline changes (Rust wgpu-hal gles set_pipeline_inner and
// rebind_sampler_states). It used to keep the first pipeline's pairing.
func TestSamplerAcrossPipelineSwitch(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		if b.software {
			t.Skip("known defect, not the sampler rebinding: the software backend samples u = 1.125 the other way round " +
				"(the clamp-to-edge sampler reads column 0, the repeat sampler column 3), with the bind group set before each draw too")
		}
		tex, view := newUsageTexture(t, device, wgpu.TextureUsageTextureBinding|wgpu.TextureUsageCopyDst)
		// Columns 0-1 hold 10, columns 2-3 hold 200 in the red channel.
		texels := make([]byte, usageTargetSize*usageTargetSize*4)
		for i := 0; i < len(texels); i += 4 {
			red := byte(10)
			if (i/4)%usageTargetSize >= usageTargetSize/2 {
				red = 200
			}
			copy(texels[i:], []byte{red, 0, 0, 255})
		}
		if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, texels,
			&wgpu.ImageDataLayout{BytesPerRow: usageTargetSize * 4, RowsPerImage: usageTargetSize},
			&wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("WriteTexture: %v", err)
		}
		clamp := newSwitchSampler(t, device, gputypes.AddressModeClampToEdge)
		repeat := newSwitchSampler(t, device, gputypes.AddressModeRepeat)

		frag := wgpu.ShaderStageFragment
		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: frag, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
			}},
			gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
			gputypes.BindGroupLayoutEntry{Binding: 2, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		)
		group := newBindGroup(t, device, bgl,
			wgpu.BindGroupEntry{Binding: 0, TextureView: view},
			wgpu.BindGroupEntry{Binding: 1, Sampler: clamp},
			wgpu.BindGroupEntry{Binding: 2, Sampler: repeat},
		)
		left := newRebindRenderPipeline(t, device, samplerSwitchShader("s_clamp", -1, 0), bgl)
		right := newRebindRenderPipeline(t, device, samplerSwitchShader("s_repeat", 0, 1), bgl)

		draw := gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1}
		px := renderDynamicTarget(t, device, func(pass *wgpu.RenderPassEncoder) {
			pass.SetPipeline(left)
			pass.SetBindGroup(0, group, nil)
			pass.Draw(draw)
			pass.SetPipeline(right)
			pass.Draw(draw)
		})

		row := featureTargetHeight / 2 * featureTargetWidth
		if got := px[row+8] & 0xff; got != 200 {
			t.Errorf("left half (clamp sampler): red = %d, want 200", got)
		}
		if got := px[row+featureTargetWidth-8] & 0xff; got != 10 {
			t.Errorf("right half (repeat sampler, group kept across the pipeline change): red = %d, want 10", got)
		}
	})
}

func newSwitchSampler(t *testing.T, device *wgpu.Device, mode gputypes.AddressMode) *wgpu.Sampler {
	t.Helper()
	s, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		AddressModeU: mode, AddressModeV: mode, AddressModeW: mode,
		MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest,
		MipmapFilter: gputypes.FilterModeNearest, LodMaxClamp: 32,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	t.Cleanup(s.Release)
	return s
}
