//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
)

// These tests pin the WebGPU usage-scope rule: a usage conflict is only
// possible inside ONE usage scope (one render pass, one compute dispatch, one
// copy command). Different commands of the same encoder may use a resource
// with different usages; the encoder transitions between them.
//
// Reference: W3C WebGPU spec "usage scope"; wgpu-core track/mod.rs
// (UsageScope = merge with conflict check, command buffer Tracker =
// barrier + update, no conflict).

func newUsageScopeTexture(t *testing.T, device *Device, label string, usage gputypes.TextureUsage) (*Texture, *TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         label,
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture(%s): %v", label, err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		tex.Release()
		t.Fatalf("CreateView(%s): %v", label, err)
	}
	t.Cleanup(func() {
		view.Release()
		tex.Release()
	})
	return tex, view
}

func newUsageScopeBuffer(t *testing.T, device *Device, label string, usage gputypes.BufferUsage) *Buffer {
	t.Helper()
	buf, err := device.CreateBuffer(&BufferDescriptor{Label: label, Size: 256, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer(%s): %v", label, err)
	}
	t.Cleanup(buf.Release)
	return buf
}

func runColorPass(t *testing.T, enc *CommandEncoder, view *TextureView, load gputypes.LoadOp, record func(*RenderPassEncoder)) {
	t.Helper()
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{
		ColorAttachments: []RenderPassColorAttachment{{
			View:    view,
			LoadOp:  load,
			StoreOp: gputypes.StoreOpStore,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if record != nil {
		record(pass)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
}

func mustFinish(t *testing.T, enc *CommandEncoder) *CommandBuffer {
	t.Helper()
	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	t.Cleanup(cb.Release)
	return cb
}

// TestRenderPassThenCopyTextureToBufferSameEncoder is the triangle-headless
// readback pattern: render into T, end the pass, copy T to a staging buffer,
// all in one encoder. ColorTarget and CopySrc belong to different usage
// scopes, so this is valid WebGPU.
func TestRenderPassThenCopyTextureToBufferSameEncoder(t *testing.T) {
	t.Parallel()
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	tex, view := newUsageScopeTexture(t, device, "readback-target",
		TextureUsageRenderAttachment|TextureUsageCopySrc)
	staging := newUsageScopeBuffer(t, device, "readback-staging", BufferUsageCopyDst|BufferUsageMapRead)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	runColorPass(t, enc, view, gputypes.LoadOpClear, nil)
	enc.CopyTextureToBuffer(tex, staging, []BufferTextureCopy{{
		TextureBase:  ImageCopyTexture{Texture: tex},
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		Size:         Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	}})
	cb := mustFinish(t, enc)

	idx := tex.coreTexture.TrackingData().Index()
	tracker := cb.core.TextureScope()
	if got := tracker.StartUsage(idx); got != track.TextureUsesColorTarget {
		t.Errorf("first usage in command buffer = %v, want ColorTarget", got)
	}
	if got := tracker.EndUsage(idx); got != track.TextureUsesCopySrc {
		t.Errorf("last usage in command buffer = %v, want CopySrc", got)
	}
}

// TestCopyBufferToTextureThenRenderAttachmentSameEncoder uploads into T and
// then renders on top of it (LoadOp Load) in the same encoder.
func TestCopyBufferToTextureThenRenderAttachmentSameEncoder(t *testing.T) {
	t.Parallel()
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	tex, view := newUsageScopeTexture(t, device, "upload-then-draw",
		TextureUsageRenderAttachment|TextureUsageCopyDst)
	upload := newUsageScopeBuffer(t, device, "upload-src", BufferUsageCopySrc)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToTexture(upload, tex, []BufferTextureCopy{{
		TextureBase:  ImageCopyTexture{Texture: tex},
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		Size:         Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	}})
	runColorPass(t, enc, view, gputypes.LoadOpLoad, nil)
	cb := mustFinish(t, enc)

	idx := tex.coreTexture.TrackingData().Index()
	tracker := cb.core.TextureScope()
	if got := tracker.StartUsage(idx); got != track.TextureUsesCopyDst {
		t.Errorf("first usage in command buffer = %v, want CopyDst", got)
	}
	if got := tracker.EndUsage(idx); got != track.TextureUsesColorTarget {
		t.Errorf("last usage in command buffer = %v, want ColorTarget", got)
	}
}

// TestCopyBufferToTextureThenSampledInRenderPassSameEncoder uploads into T and
// samples T (TextureBinding) from a later render pass of the same encoder.
func TestCopyBufferToTextureThenSampledInRenderPassSameEncoder(t *testing.T) {
	t.Parallel()
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	sampled, sampledView := newUsageScopeTexture(t, device, "upload-then-sample",
		TextureUsageTextureBinding|TextureUsageCopyDst)
	_, targetView := newUsageScopeTexture(t, device, "sample-target", TextureUsageRenderAttachment)
	upload := newUsageScopeBuffer(t, device, "upload-src", BufferUsageCopySrc)

	layout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: ShaderStageFragment,
			Texture: &gputypes.TextureBindingLayout{
				SampleType:    gputypes.TextureSampleTypeFloat,
				ViewDimension: gputypes.TextureViewDimension2D,
			},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()
	group, err := device.CreateBindGroup(&BindGroupDescriptor{
		Layout:  layout,
		Entries: []BindGroupEntry{{Binding: 0, TextureView: sampledView}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer group.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToTexture(upload, sampled, []BufferTextureCopy{{
		TextureBase:  ImageCopyTexture{Texture: sampled},
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		Size:         Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	}})
	runColorPass(t, enc, targetView, gputypes.LoadOpClear, func(pass *RenderPassEncoder) {
		pass.SetBindGroup(0, group, nil)
	})
	mustFinish(t, enc)
}

// TestCopyBufferToBufferThenVertexBufferSameEncoder writes vertex data with a
// copy and then binds that buffer as a vertex buffer in a later render pass.
func TestCopyBufferToBufferThenVertexBufferSameEncoder(t *testing.T) {
	t.Parallel()
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	staging := newUsageScopeBuffer(t, device, "vertex-staging", BufferUsageCopySrc)
	vertices := newUsageScopeBuffer(t, device, "vertices", BufferUsageVertex|BufferUsageCopyDst)
	_, targetView := newUsageScopeTexture(t, device, "vertex-target", TextureUsageRenderAttachment)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToBuffer(staging, 0, vertices, 0, 64)
	runColorPass(t, enc, targetView, gputypes.LoadOpClear, func(pass *RenderPassEncoder) {
		pass.SetVertexBuffer(0, vertices, 0)
	})
	cb := mustFinish(t, enc)

	idx := vertices.core.TrackingData().Index()
	tracker := cb.core.BufferScope()
	if got := tracker.StartUsage(idx); got != track.BufferUsesCopyDst {
		t.Errorf("first usage in command buffer = %v, want CopyDst", got)
	}
	if got := tracker.EndUsage(idx); got != track.BufferUsesVertex {
		t.Errorf("last usage in command buffer = %v, want Vertex", got)
	}
}

// TestRenderCopyRenderSameTextureSameEncoder renders into T, copies T to U and
// renders into T again — three usage scopes, each valid on its own.
func TestRenderCopyRenderSameTextureSameEncoder(t *testing.T) {
	t.Parallel()
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	tex, view := newUsageScopeTexture(t, device, "render-copy-render",
		TextureUsageRenderAttachment|TextureUsageCopySrc)
	dst, _ := newUsageScopeTexture(t, device, "render-copy-dst", TextureUsageCopyDst)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	runColorPass(t, enc, view, gputypes.LoadOpClear, nil)
	enc.CopyTextureToTexture(tex, dst, []TextureCopy{{
		Source:      ImageCopyTexture{Texture: tex},
		Destination: ImageCopyTexture{Texture: dst},
		Size:        Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	}})
	runColorPass(t, enc, view, gputypes.LoadOpLoad, nil)
	cb := mustFinish(t, enc)

	idx := tex.coreTexture.TrackingData().Index()
	tracker := cb.core.TextureScope()
	if got := tracker.StartUsage(idx); got != track.TextureUsesColorTarget {
		t.Errorf("first usage = %v, want ColorTarget", got)
	}
	if got := tracker.EndUsage(idx); got != track.TextureUsesColorTarget {
		t.Errorf("last usage = %v, want ColorTarget", got)
	}
}
