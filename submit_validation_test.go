//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// =============================================================================
// VAL-A6: Queue.Submit resource state validation
// WebGPU spec §21.2, Rust wgpu-core device/queue.rs:1764-1828
// =============================================================================

// TestSubmitAcceptsBufferReleasedAfterRecording verifies that Release() after
// recording is a drop, not a destroy: the command buffer holds the buffer's
// reference and Submit accepts it, as Rust wgpu-core does for a dropped
// Arc<Buffer> (only a destroyed buffer fails, queue.rs check_destroyed).
// P1-CORE-020; hal/allbackends TestReleaseBeforeSubmit* reads the results back.
func TestSubmitAcceptsBufferReleasedAfterRecording(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-destroyed-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-destroyed-dst",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer dst: %v", err)
	}
	defer dstBuf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	// Use the buffer in a copy command to track it.
	enc.CopyBufferToBuffer(srcBuf, 0, dstBuf, 0, 64)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Release the source buffer AFTER encoding but BEFORE submit.
	srcBuf.Release()

	if _, err := device.Queue().Submit(cmdBuf); err != nil {
		t.Fatalf("Submit with a buffer released after recording: %v", err)
	}
}

// TestSubmitWithMappedBuffer verifies that submitting a command buffer that
// references a mapped buffer returns ErrSubmitBufferMapped.
// Matches Rust QueueSubmitError::BufferStillMapped.
func TestSubmitWithMappedBuffer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mappedBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "val-a6-mapped-buf",
		Size:             64,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer mapped: %v", err)
	}
	defer mappedBuf.Release()

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-mapped-dst",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer dst: %v", err)
	}
	defer dstBuf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	// Use the mapped buffer as copy source to track it.
	enc.CopyBufferToBuffer(mappedBuf, 0, dstBuf, 0, 64)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Buffer is still mapped (MappedAtCreation, never unmapped).
	_, err = device.Queue().Submit(cmdBuf)
	if err == nil {
		t.Fatal("Submit should fail: command buffer references mapped buffer")
	}
	if !errors.Is(err, wgpu.ErrSubmitBufferMapped) {
		t.Errorf("expected ErrSubmitBufferMapped, got: %v", err)
	}
}

// TestSubmitAcceptsTextureReleasedAfterRecording is the texture counterpart of
// TestSubmitAcceptsBufferReleasedAfterRecording (P1-CORE-020).
func TestSubmitAcceptsTextureReleasedAfterRecording(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "val-a6-destroyed-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopySrc | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-dst-buf",
		Size:  1024,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer dstBuf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	// Use the texture in a copy command to track it.
	enc.CopyTextureToBuffer(tex, dstBuf, []wgpu.BufferTextureCopy{
		{
			TextureBase: wgpu.ImageCopyTexture{
				Texture: tex,
				Origin:  wgpu.Origin3D{X: 0, Y: 0, Z: 0},
			},
			Size: wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
			BufferLayout: wgpu.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  256,
				RowsPerImage: 4,
			},
		},
	})

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Release the texture AFTER encoding but BEFORE submit.
	tex.Release()

	if _, err := device.Queue().Submit(cmdBuf); err != nil {
		t.Fatalf("Submit with a texture released after recording: %v", err)
	}
}

// TestSubmitValidResources verifies that Submit succeeds when all referenced
// resources are in valid state (not destroyed, not mapped).
func TestSubmitValidResources(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-valid-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}
	defer srcBuf.Release()

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-valid-dst",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer dst: %v", err)
	}
	defer dstBuf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToBuffer(srcBuf, 0, dstBuf, 0, 64)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// All resources valid — Submit should succeed.
	_, err = device.Queue().Submit(cmdBuf)
	if err != nil {
		t.Fatalf("Submit should succeed: %v", err)
	}
}

// TestSubmitDoubleSubmit verifies that submitting the same command buffer
// twice returns ErrSubmitCommandBufferInvalid.
// Matches Rust wgpu-core CommandBuffer::take_finished() which consumes
// the buffer, preventing reuse.
func TestSubmitDoubleSubmit(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-double-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}
	defer srcBuf.Release()

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-double-dst",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer dst: %v", err)
	}
	defer dstBuf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToBuffer(srcBuf, 0, dstBuf, 0, 64)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// First submit — should succeed.
	_, err = device.Queue().Submit(cmdBuf)
	if err != nil {
		t.Fatalf("first Submit should succeed: %v", err)
	}

	// Second submit — should fail.
	_, err = device.Queue().Submit(cmdBuf)
	if err == nil {
		t.Fatal("second Submit should fail: command buffer already submitted")
	}
	if !errors.Is(err, wgpu.ErrSubmitCommandBufferInvalid) {
		t.Errorf("expected ErrSubmitCommandBufferInvalid, got: %v", err)
	}
}

// TestSubmitEmptyCommandBuffer verifies that submitting an empty command buffer
// (no resources referenced) succeeds.
func TestSubmitEmptyCommandBuffer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// No resources — should succeed.
	_, err = device.Queue().Submit(cmdBuf)
	if err != nil {
		t.Fatalf("Submit empty command buffer should succeed: %v", err)
	}
}

// TestSubmitNoCommandBuffers verifies that submitting zero command buffers
// succeeds (flushes pending writes only).
func TestSubmitNoCommandBuffers(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	_, err := device.Queue().Submit()
	if err != nil {
		t.Fatalf("Submit() with no command buffers should succeed: %v", err)
	}
}

// =============================================================================
// VAL-B5: bind groups at Submit
// Rust wgpu-core device/queue.rs:2171-2180 (bind_group.try_raw)
// =============================================================================

// TestSubmitAcceptsBindGroupReleasedAfterRecording: a bind group released after
// a pass bound it is still held by the command buffer, so Submit accepts it
// (P1-CORE-020).
func TestSubmitAcceptsBindGroupReleasedAfterRecording(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "val-b5-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "val-b5-destroyed-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	// Use a compute pass to bind the group so it gets tracked.
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetBindGroup(0, bg, nil)
	_ = pass.End()

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Release the bind group AFTER encoding but BEFORE submit.
	bg.Release()

	if _, err := device.Queue().Submit(cmdBuf); err != nil {
		t.Fatalf("Submit with a bind group released after recording: %v", err)
	}
}

// TestSubmitWithValidBindGroup verifies that Submit succeeds when a command
// buffer references a bind group that is still alive.
func TestSubmitWithValidBindGroup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "val-b5-valid-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "val-b5-valid-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetBindGroup(0, bg, nil)
	_ = pass.End()

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Bind group is still alive -- Submit should succeed.
	_, err = device.Queue().Submit(cmdBuf)
	if err != nil {
		t.Fatalf("Submit should succeed: %v", err)
	}
}

// TestSubmitAcceptsReleasedBufferInBindGroup: a buffer reachable only through
// a bind group, released after recording, is a drop; the walk over
// BindGroup.boundBuffers still validates it and accepts it (P1-CORE-020).
func TestSubmitAcceptsReleasedBufferInBindGroup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-bg-buf",
		Size:  128,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "val-a6-bg-layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 128,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "val-a6-bg",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: 128},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetBindGroup(0, bg, nil)
	pass.End()

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Release the buffer — but not the bind group — after encoding.
	buf.Release()

	if _, err := device.Queue().Submit(cmdBuf); err != nil {
		t.Fatalf("Submit with a bind group buffer released after recording: %v", err)
	}
}

// TestSubmitAcceptsReleasedTextureInBindGroup is the texture counterpart of
// TestSubmitAcceptsReleasedBufferInBindGroup, via BindGroup.boundTextures.
func TestSubmitAcceptsReleasedTextureInBindGroup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "val-a6-bg-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}

	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "val-a6-bg-tex-layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Texture: &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeFloat,
					ViewDimension: gputypes.TextureViewDimension2D,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "val-a6-bg-tex",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, TextureView: view},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetBindGroup(0, bg, nil)
	pass.End()

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Release the texture — but not the bind group — after encoding.
	tex.Release()

	if _, err := device.Queue().Submit(cmdBuf); err != nil {
		t.Fatalf("Submit with a bind group texture released after recording: %v", err)
	}
}
