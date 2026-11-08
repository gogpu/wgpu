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
)

// Release() is Rust's drop, not WebGPU's destroy() (ADR-056): a resource the
// application releases after a command recorded it stays alive until that work
// has run, because the command buffer holds a reference, as Rust wgpu-core's
// trackers hold an Arc. Submit must accept such a command buffer and the work
// must see the resource's contents (P1-CORE-020, from gogpu#461: a frame
// encoder that draws a texture which the app releases before the frame's
// submit). The readbacks prove the memory was not freed early; on Vulkan the
// synchronization validation layer runs and every error fails the test.

const releaseSize = 4 // texels and words

// TestReleaseBeforeSubmitBufferCopy: a buffer written by the queue and
// recorded as a copy source is released before Submit.
func TestReleaseBeforeSubmitBufferCopy(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		src, err := device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "release-src",
			Size:  releaseSize * 4,
			Usage: wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		want := []uint32{0x11, 0x2222, 0x333333, 0x44444444}
		writeWords(t, device, src, want...)
		staging := newPublicBuffer(t, device, releaseSize*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		enc := newEncoder(t, device)
		enc.CopyBufferToBuffer(src, 0, staging, 0, releaseSize*4)
		src.Release()
		submit(t, device, enc)

		checkWords(t, "released copy source buffer", readPublicBuffer(t, staging, releaseSize), want)
	})
}

// TestReleaseBeforeSubmitTextureCopy: a texture filled by WriteTexture (a
// pending queue write) and recorded as a copy source is released before Submit.
func TestReleaseBeforeSubmitTextureCopy(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		tex := newReleaseTexture(t, device, wgpu.TextureUsageCopySrc|wgpu.TextureUsageCopyDst)
		want := writeReleaseTexels(t, device, tex)
		staging := newPublicBuffer(t, device, 256, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		enc := newEncoder(t, device)
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
			Size:         wgpu.Extent3D{Width: releaseSize, Height: 1, DepthOrArrayLayers: 1},
		}})
		tex.Release()
		submit(t, device, enc)

		checkWords(t, "released copy source texture", readPublicBuffer(t, staging, releaseSize), want)
	})
}

const releaseSampleWGSL = `
@group(0) @binding(0) var src: texture_2d<f32>;
@group(0) @binding(1) var smp: sampler;
@group(0) @binding(2) var<storage, read_write> dst: array<u32, 4>;

@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let uv = vec2<f32>((f32(id.x) + 0.5) / 4.0, 0.5);
    dst[id.x] = pack4x8unorm(textureSampleLevel(src, smp, uv, 0.0));
}
`

// TestReleaseBeforeSubmitBindGroup is gogpu#461's shape: a texture uploaded by
// WriteTexture, its view, a sampler and the bind group holding them, plus the
// storage buffer the dispatch writes, are all released after recording and
// before Submit.
func TestReleaseBeforeSubmitBindGroup(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		tex := newReleaseTexture(t, device, wgpu.TextureUsageTextureBinding|wgpu.TextureUsageCopyDst)
		want := writeReleaseTexels(t, device, tex)
		view, err := device.CreateTextureView(tex, nil)
		if err != nil {
			t.Fatalf("CreateTextureView: %v", err)
		}
		sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{Label: "release-nearest"})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		out, err := device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "release-out",
			Size:  releaseSize * 4,
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
		})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		staging := newPublicBuffer(t, device, releaseSize*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		group, enc := recordReleaseSample(t, device, view, sampler, out, staging)

		// Everything the recorded work uses, released before Submit, in the
		// order gogpu's Texture.Destroy releases its parts.
		group.Release()
		sampler.Release()
		view.Release()
		tex.Release()
		out.Release()
		submit(t, device, enc)

		checkWords(t, "texels sampled through a released bind group", readPublicBuffer(t, staging, releaseSize), want)
	})
}

// TestReleaseTextureWhileViewAlive: a view keeps its texture alive, as Rust
// wgpu-core's TextureView holds an Arc of its parent. The texture is released
// with no work pending on it, and only then is a bind group created from the
// view and dispatched; the texture must not be destroyed under that work
// (it was: VUID-vkDestroyImage-image-01000).
func TestReleaseTextureWhileViewAlive(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		tex := newReleaseTexture(t, device, wgpu.TextureUsageTextureBinding|wgpu.TextureUsageCopyDst)
		want := writeReleaseTexels(t, device, tex)
		// Run the upload so no pending write holds the texture any more.
		if _, err := device.Queue().Submit(); err != nil {
			t.Fatalf("Submit (flush upload): %v", err)
		}
		if err := device.WaitIdle(); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}
		view, err := device.CreateTextureView(tex, nil)
		if err != nil {
			t.Fatalf("CreateTextureView: %v", err)
		}
		t.Cleanup(view.Release)
		tex.Release()

		sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{Label: "release-nearest"})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		t.Cleanup(sampler.Release)
		out := newPublicBuffer(t, device, releaseSize*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc)
		staging := newPublicBuffer(t, device, releaseSize*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		group, enc := recordReleaseSample(t, device, view, sampler, out, staging)
		t.Cleanup(group.Release)
		submit(t, device, enc)

		checkWords(t, "texels sampled through a view of a released texture", readPublicBuffer(t, staging, releaseSize), want)
	})
}

// TestReleasedSamplerCannotBeBound: a released sampler may already be queued
// for destruction, so a new bind group must not name it.
func TestReleasedSamplerCannotBeBound(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{Label: "released"})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		sampler.Release()
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageCompute,
			Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}})
		group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl,
			Entries: []wgpu.BindGroupEntry{{Binding: 0, Sampler: sampler}}})
		if !errors.Is(err, wgpu.ErrReleased) {
			if group != nil {
				group.Release()
			}
			t.Fatalf("CreateBindGroup with a released sampler = %v, want ErrReleased", err)
		}
	})
}

// recordReleaseSample records a dispatch of releaseSampleWGSL that samples
// view into out, and a copy of out into staging; it returns the bind group and
// the unfinished encoder.
func recordReleaseSample(t *testing.T, device *wgpu.Device, view *wgpu.TextureView, sampler *wgpu.Sampler,
	out, staging *wgpu.Buffer) (*wgpu.BindGroup, *wgpu.CommandEncoder) {
	t.Helper()
	bgl := newBindGroupLayout(t, device,
		gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageCompute, Texture: &gputypes.TextureBindingLayout{
			SampleType:    gputypes.TextureSampleTypeFloat,
			ViewDimension: gputypes.TextureViewDimension2D,
		}},
		gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: wgpu.ShaderStageCompute,
			Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		gputypes.BindGroupLayoutEntry{Binding: 2, Visibility: wgpu.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
	)
	group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl, Entries: []wgpu.BindGroupEntry{
		{Binding: 0, TextureView: view},
		{Binding: 1, Sampler: sampler},
		{Binding: 2, Buffer: out, Size: releaseSize * 4},
	}})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	pipeline := newComputePipeline(t, device, releaseSampleWGSL, bgl)

	enc := newEncoder(t, device)
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("ComputePass.End: %v", err)
	}
	enc.CopyBufferToBuffer(out, 0, staging, 0, releaseSize*4)
	return group, enc
}

// newReleaseTexture creates a releaseSize×1 RGBA8 texture the caller releases.
func newReleaseTexture(t *testing.T, device *wgpu.Device, usage gputypes.TextureUsage) *wgpu.Texture {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "release-tex",
		Size:          wgpu.Extent3D{Width: releaseSize, Height: 1, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	return tex
}

// writeReleaseTexels fills the texture through a pending queue write and
// returns each texel as the little-endian word a copy or pack4x8unorm yields.
func writeReleaseTexels(t *testing.T, device *wgpu.Device, tex *wgpu.Texture) []uint32 {
	t.Helper()
	texels := make([]byte, releaseSize*4)
	want := make([]uint32, releaseSize)
	for i := range releaseSize {
		px := texels[i*4 : i*4+4]
		px[0], px[1], px[2], px[3] = byte(10*i+1), byte(10*i+2), byte(10*i+3), 255
		want[i] = binary.LittleEndian.Uint32(px)
	}
	if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, texels,
		&wgpu.ImageDataLayout{BytesPerRow: releaseSize * 4, RowsPerImage: 1},
		&wgpu.Extent3D{Width: releaseSize, Height: 1, DepthOrArrayLayers: 1}); err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	return want
}
