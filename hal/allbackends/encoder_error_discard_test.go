// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// An encoder that will never be submitted (Finish failed, DiscardEncoding,
// or abandoned and discarded when the device is released) must end its open
// backend recording before it drops the references its commands took: the
// last reference may destroy a resource, and a D3D12 command list must not
// outlive a resource it records while it is still open ("ID3D12CommandList::
// Close: An ID3D12Resource ... was deleted prior to closing the command
// list", then an access violation on NVIDIA). Rust wgpu-core gets this order
// from InnerCommandEncoder::drop, which discards and resets the encoding
// before the trackers holding the resources drop.
//
// Every case records real draws with a pipeline and a bind group, makes the
// pass fail, and leaves the test's cleanups to release the resources before
// the device. The DX12 debug layer and the Vulkan validation layer must stay
// silent.
func TestErroredEncoderDiscardsBeforeReleasingResources(t *testing.T) {
	cases := []struct {
		name   string
		finish func(t *testing.T, enc *wgpu.CommandEncoder)
	}{
		{"finish fails", func(t *testing.T, enc *wgpu.CommandEncoder) {
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish accepted a pass with a nil pipeline")
			}
		}},
		{"discarded", func(_ *testing.T, enc *wgpu.CommandEncoder) {
			enc.DiscardEncoding()
		}},
		// Never finished nor discarded: Device.Release discards it.
		{"abandoned", func(*testing.T, *wgpu.CommandEncoder) {}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
				c.finish(t, recordErroredRenderPass(t, device))
			})
		})
	}
}

// TestReleasedWriteDestinationLivesUntilSubmit writes a buffer through the
// queue, releases it before the write is submitted, and submits other work.
// The pending write must keep the buffer alive until its submission
// completes, as Rust wgpu-core's PendingWrites holds an Arc of each
// destination: the buffer used to be destroyed at the release while the
// recorded copy into it was still to run.
func TestReleasedWriteDestinationLivesUntilSubmit(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "released-destination",
			Size:  256,
			Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageStorage,
		})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		if err := device.Queue().WriteBuffer(buf, 0, make([]byte, 256)); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		buf.Release()
		submit(t, device, newEncoder(t, device))
		if err := device.WaitIdle(); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}
	})
}

// recordErroredRenderPass records a render pass over a fresh target that
// draws twice with a pipeline and a bind group of two buffers, then sets a
// nil pipeline, and returns the encoder in its error state. Every resource
// is released by the test's cleanups, before the device.
func recordErroredRenderPass(t *testing.T, device *wgpu.Device) *wgpu.CommandEncoder {
	t.Helper()
	stages := wgpu.ShaderStageVertex | wgpu.ShaderStageFragment
	bgl := newDynamicLayout(t, device, stages, false, 0)
	pipeline := newDynamicRenderPipeline(t, device, bgl)
	uniform := newDynamicUniformBuffer(t, device, func(slot int) []uint32 {
		return []uint32{math.Float32bits(-1), math.Float32bits(1), 0, 0, dynamicUniformTags[slot], 0, 0, 0}
	})
	storage := newDynamicStorageBuffer(t, device)
	bg := newDynamicBindGroup(t, device, bgl, uniform, storage, nil)
	view := newRenderTargetView(t, device, wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)

	enc := newEncoder(t, device)
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, []uint32{0, 0})
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	pass.SetBindGroup(0, bg, []uint32{dynamicSlot, dynamicSlot})
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	pass.SetPipeline(nil)
	_ = pass.End()
	return enc
}

// newRenderTargetView creates a featureTargetWidth x featureTargetHeight
// RGBA8 texture with usage and returns its default view; both are released
// by the test's cleanups.
func newRenderTargetView(t *testing.T, device *wgpu.Device, usage gputypes.TextureUsage) *wgpu.TextureView {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "render-target",
		Size:          wgpu.Extent3D{Width: featureTargetWidth, Height: featureTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         usage,
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
	return view
}
