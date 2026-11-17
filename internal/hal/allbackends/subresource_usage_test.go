// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/core/track"
)

// WebGPU checks a usage scope per subresource: a render pass may sample one
// mip level or array layer of a texture while drawing into another, as a
// mipmap chain is generated level by level. The texture tracker keeps each
// mip level and layer's state of its own, so such a pass is accepted, and the
// barriers between passes move each subresource from its own state.
//
// Each case uploads subresource 0, draws subresource 1 from it, then draws
// subresource 0 from subresource 1 swizzled, all in one command buffer, and
// reads both back. The second pass reads what the first drew only if the
// barrier between them moved subresource 1 from the color target state to the
// sampled one while subresource 0 moved the other way. The Vulkan validation
// layer and the DX12 debug layer must stay silent.

// subresourceUpload is the color uploaded into subresource 0, and
// subresourceSwizzled the one the second pass draws from it (.zyxw).
var (
	subresourceUpload   = [4]byte{10, 200, 30, 255}
	subresourceSwizzled = [4]byte{30, 200, 10, 255}
)

// subresourceCase is a texture of two subresources: two mip levels of one
// layer, or two layers of one mip level.
type subresourceCase struct {
	name      string
	mipLevels uint32
	layers    uint32
}

// newView returns a new 2D view of subresource i of tex.
func (c subresourceCase) newView(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, i uint32) *wgpu.TextureView {
	t.Helper()
	if c.mipLevels > 1 {
		return newSubresourceView(t, device, tex, i, 0)
	}
	return newSubresourceView(t, device, tex, 0, i)
}

// makeCopyTexture returns where subresource i of tex starts.
func (c subresourceCase) makeCopyTexture(tex *wgpu.Texture, i uint32) wgpu.ImageCopyTexture {
	if c.mipLevels > 1 {
		return wgpu.ImageCopyTexture{Texture: tex, MipLevel: i}
	}
	return wgpu.ImageCopyTexture{Texture: tex, Origin: wgpu.Origin3D{Z: i}}
}

var subresourceCases = []subresourceCase{
	{name: "mip levels", mipLevels: 2, layers: 1},
	{name: "array layers", mipLevels: 1, layers: 2},
}

func TestRenderPassSamplesOneSubresourceWhileDrawingAnother(t *testing.T) {
	for _, c := range subresourceCases {
		t.Run(c.name, func(t *testing.T) {
			forEachDynamicOffsetDevice(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
				if b.name == backendGLES && c.layers > 1 {
					t.Skip("GLES has no texture views: one layer of an array texture cannot be sampled alone")
				}
				tex := newSubresourceTexture(t, device, c)
				views := [2]*wgpu.TextureView{c.newView(t, device, tex, 0), c.newView(t, device, tex, 1)}
				texels := make([]byte, 4*usageTargetSize*usageTargetSize)
				for i := 0; i < len(texels); i += 4 {
					copy(texels[i:], subresourceUpload[:])
				}
				upload := c.makeCopyTexture(tex, 0)
				if err := device.Queue().WriteTexture(&upload, texels,
					&wgpu.ImageDataLayout{BytesPerRow: 4 * usageTargetSize, RowsPerImage: usageTargetSize},
					&wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: 1}); err != nil {
					t.Fatalf("WriteTexture: %v", err)
				}

				enc := newEncoder(t, device)
				drawSampled(t, device, enc, views[0], views[1], "c")
				drawSampled(t, device, enc, views[1], views[0], "c.zyxw")
				staging := [2]*wgpu.Buffer{copySubresource(t, device, enc, c, tex, 0), copySubresource(t, device, enc, c, tex, 1)}
				submit(t, device, enc)

				checkFirstTexel(t, "subresource 1, drawn from the upload", staging[1], subresourceUpload)
				checkFirstTexel(t, "subresource 0, drawn from subresource 1", staging[0], subresourceSwizzled)
			})
		})
	}
}

// TestRenderPassRefusesSampledAndDrawnSubresource checks that a pass drawing
// into a subresource still may not sample it: a view sampling both
// subresources conflicts with the attachment of one.
func TestRenderPassRefusesSampledAndDrawnSubresource(t *testing.T) {
	for _, c := range subresourceCases {
		t.Run(c.name, func(t *testing.T) {
			forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
				tex := newSubresourceTexture(t, device, c)
				dimension := gputypes.TextureViewDimension2DArray
				if c.mipLevels > 1 {
					dimension = gputypes.TextureViewDimension2D
				}
				both, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{Dimension: dimension})
				if err != nil {
					t.Fatalf("CreateTextureView: %v", err)
				}
				t.Cleanup(both.Release)
				enc := newEncoder(t, device)
				pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
					ColorAttachments: []wgpu.RenderPassColorAttachment{{
						View: c.newView(t, device, tex, 1), LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					}},
				})
				if err != nil {
					t.Fatalf("BeginRenderPass: %v", err)
				}
				layout := newBindGroupLayout(t, device, makeSampledTextureEntry(dimension))
				group := newBindGroup(t, device, layout, wgpu.BindGroupEntry{Binding: 0, TextureView: both})
				pass.SetBindGroup(0, group, nil)
				_ = pass.End()
				_, err = enc.Finish()
				var conflict *track.TextureUsageConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("Finish error = %v, want a *track.TextureUsageConflictError from SetBindGroup", err)
				}
				wantMipLevel, wantArrayLayer := uint32(0), uint32(1)
				if c.mipLevels > 1 {
					wantMipLevel, wantArrayLayer = 1, 0
				}
				if conflict.Selector.BaseMipLevel != wantMipLevel || conflict.Selector.BaseArrayLayer != wantArrayLayer {
					t.Errorf("conflict on mip level %d, array layer %d, want the drawn subresource, mip level %d, array layer %d",
						conflict.Selector.BaseMipLevel, conflict.Selector.BaseArrayLayer, wantMipLevel, wantArrayLayer)
				}
			})
		})
	}
}

// newSubresourceTexture returns a usageTargetSize-square RGBA8 texture of
// c's mip levels and layers, which passes draw into and sample.
func newSubresourceTexture(t *testing.T, device *wgpu.Device, c subresourceCase) *wgpu.Texture {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "subresources",
		Size:          wgpu.Extent3D{Width: usageTargetSize, Height: usageTargetSize, DepthOrArrayLayers: c.layers},
		MipLevelCount: c.mipLevels,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage: wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageTextureBinding |
			wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	return tex
}

func makeSampledTextureEntry(dimension gputypes.TextureViewDimension) gputypes.BindGroupLayoutEntry {
	return gputypes.BindGroupLayoutEntry{
		Binding:    0,
		Visibility: gputypes.ShaderStageFragment,
		Texture: &gputypes.TextureBindingLayout{
			SampleType:    gputypes.TextureSampleTypeFloat,
			ViewDimension: dimension,
		},
	}
}

// drawSampled records a pass over target that fills it with expression of
// c, the first texel of source.
func drawSampled(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, source, target *wgpu.TextureView, expression string) {
	t.Helper()
	bindGroupLayout := newBindGroupLayout(t, device, makeSampledTextureEntry(gputypes.TextureViewDimension2D))
	pipeline := newSamplePipeline(t, device, `
@group(0) @binding(0) var source: texture_2d<f32>;

@vertex
fn vs_main(@builtin(vertex_index) v: u32) -> @builtin(position) vec4<f32> {
    var triangle = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
    return vec4<f32>(triangle[v], 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    let c = textureLoad(source, vec2<i32>(0, 0), 0);
    return `+expression+`;
}
`, bindGroupLayout)
	group := newBindGroup(t, device, bindGroupLayout, wgpu.BindGroupEntry{Binding: 0, TextureView: source})

	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: target, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
}

// copySubresource records a copy of the first row of subresource i of tex
// into a new staging buffer.
func copySubresource(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, c subresourceCase, tex *wgpu.Texture, i uint32) *wgpu.Buffer {
	t.Helper()
	staging := newPublicBuffer(t, device, 256, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
		TextureBase:  c.makeCopyTexture(tex, i),
		Size:         wgpu.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	}})
	return staging
}

// checkFirstTexel checks the texel a copySubresource staging buffer holds.
func checkFirstTexel(t *testing.T, what string, staging *wgpu.Buffer, want [4]byte) {
	t.Helper()
	var got [4]byte
	binary.LittleEndian.PutUint32(got[:], readPublicBuffer(t, staging, 1)[0])
	if got != want {
		t.Errorf("%s: texel (0, 0) = %v, want %v", what, got, want)
	}
}
