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

// TestSubmitWithDestroyedBuffer verifies that submitting a command buffer that
// references a released buffer returns ErrSubmitBufferDestroyed.
// Matches Rust QueueSubmitError::DestroyedResource for buffers.
func TestSubmitWithDestroyedBuffer(t *testing.T) {
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

	_, err = device.Queue().Submit(cmdBuf)
	if err == nil {
		t.Fatal("Submit should fail: command buffer references destroyed buffer")
	}
	if !errors.Is(err, wgpu.ErrSubmitBufferDestroyed) {
		t.Errorf("expected ErrSubmitBufferDestroyed, got: %v", err)
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

// TestSubmitWithDestroyedTexture verifies that submitting a command buffer that
// references a released texture returns ErrSubmitTextureDestroyed.
// Matches Rust QueueSubmitError::DestroyedResource for textures.
func TestSubmitWithDestroyedTexture(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "val-a6-destroyed-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopySrc | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-dst-buf",
		Size:  256,
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
				BytesPerRow:  16,
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

	_, err = device.Queue().Submit(cmdBuf)
	if err == nil {
		t.Fatal("Submit should fail: command buffer references destroyed texture")
	}
	if !errors.Is(err, wgpu.ErrSubmitTextureDestroyed) {
		t.Errorf("expected ErrSubmitTextureDestroyed, got: %v", err)
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

// TestSubmitAfterBufferRelease verifies that releasing a buffer and then
// submitting a command buffer that used it returns ErrSubmitBufferDestroyed.
// This is the same as TestSubmitWithDestroyedBuffer but uses Release() which
// is the public API for destruction.
func TestSubmitAfterBufferRelease(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-release-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-release-dst",
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

	// Release (not Destroy) — should still be caught.
	srcBuf.Release()

	_, err = device.Queue().Submit(cmdBuf)
	if err == nil {
		t.Fatal("Submit should fail: command buffer references released buffer")
	}
	if !errors.Is(err, wgpu.ErrSubmitBufferDestroyed) {
		t.Errorf("expected ErrSubmitBufferDestroyed, got: %v", err)
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
// VAL-B5: BindGroup destruction tracking at Submit
// Rust wgpu-core device/queue.rs:1815-1817
// =============================================================================

// TestSubmitWithDestroyedBindGroup verifies that submitting a command buffer
// that references a released bind group returns ErrSubmitBindGroupDestroyed.
// Matches Rust wgpu-core validate_command_buffer bind_group.try_raw()
// (device/queue.rs:1815-1817).
func TestSubmitWithDestroyedBindGroup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "val-b5-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
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

	_, err = device.Queue().Submit(cmdBuf)
	if err == nil {
		t.Fatal("Submit should fail: command buffer references destroyed bind group")
	}
	if !errors.Is(err, wgpu.ErrSubmitBindGroupDestroyed) {
		t.Errorf("expected ErrSubmitBindGroupDestroyed, got: %v", err)
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
		Entries: []wgpu.BindGroupLayoutEntry{},
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

// TestSubmitWithReleasedBufferInBindGroup verifies that a buffer reachable only
// through a bind group is still validated at Submit. Passes track the bind group
// itself, not its contents — validateCommandBufferForSubmit walks
// BindGroup.boundBuffers to reach this buffer.
func TestSubmitWithReleasedBufferInBindGroup(t *testing.T) {
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
		Entries: []wgpu.BindGroupLayoutEntry{
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

	_, err = device.Queue().Submit(cmdBuf)
	if !errors.Is(err, wgpu.ErrSubmitBufferDestroyed) {
		t.Errorf("Submit with released bind group buffer = %v, want ErrSubmitBufferDestroyed", err)
	}
}

// TestSubmitWithReleasedTextureInBindGroup is the texture counterpart of
// TestSubmitWithReleasedBufferInBindGroup: a texture reachable only through a
// bind group is still validated at Submit via BindGroup.boundTextures.
func TestSubmitWithReleasedTextureInBindGroup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "val-a6-bg-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
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
		Entries: []wgpu.BindGroupLayoutEntry{
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

	_, err = device.Queue().Submit(cmdBuf)
	if !errors.Is(err, wgpu.ErrSubmitTextureDestroyed) {
		t.Errorf("Submit with released bind group texture = %v, want ErrSubmitTextureDestroyed", err)
	}
}

// TestSubmitReleasedBufferBeatsReleasedBindGroup pins the error precedence when
// a bind group and a buffer it binds are both released. The buffer error is the
// actionable one, and it is what the flat usedBuffers set reported before the
// per-draw fan-out was removed, so the bind group check runs last.
func TestSubmitReleasedBufferBeatsReleasedBindGroup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a6-precedence-buf",
		Size:  128,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "val-a6-precedence-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
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
		Label:  "val-a6-precedence-bg",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: 128},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

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

	// Release both — the buffer error must win.
	buf.Release()
	bg.Release()

	_, err = device.Queue().Submit(cmdBuf)
	if !errors.Is(err, wgpu.ErrSubmitBufferDestroyed) {
		t.Errorf("Submit with released buffer in released bind group = %v, want ErrSubmitBufferDestroyed", err)
	}
}

// TestSubmitResourceErrorWinsAcrossBindGroups pins error precedence when
// several bind groups are at fault at once: many released groups binding
// nothing, and one live group binding a released buffer. The buffer error must
// win no matter which group the map iteration reaches first.
//
// Folding the release check into the resource walk passes this only when the
// live group happens to be visited first, so the odds of catching that are
// roughly 1/(releasedGroups+1) per attempt. Binding many released groups makes
// a single attempt decisive and the repeats then make a miss negligible.
func TestSubmitResourceErrorWinsAcrossBindGroups(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	const releasedGroups = 8

	bufLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "val-a6-cross-buf-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
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
		t.Fatalf("CreateBindGroupLayout buffer: %v", err)
	}
	defer bufLayout.Release()

	emptyLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "val-a6-cross-empty-layout",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout empty: %v", err)
	}
	defer emptyLayout.Release()

	for i := 0; i < 16; i++ {
		buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "val-a6-cross-buf",
			Size:  128,
			Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}

		// Live group binding a buffer that is about to be released.
		bgLive, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  "val-a6-cross-live-bg",
			Layout: bufLayout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: buf, Offset: 0, Size: 128},
			},
		})
		if err != nil {
			t.Fatalf("CreateBindGroup live: %v", err)
		}

		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass, err := enc.BeginComputePass(nil)
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetBindGroup(0, bgLive, nil)

		// Rebind slot 0 with each released group: every one lands in
		// usedBindGroups, so this sidesteps the maxBindGroups limit.
		for g := 0; g < releasedGroups; g++ {
			bgReleased, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
				Label:   "val-a6-cross-released-bg",
				Layout:  emptyLayout,
				Entries: []wgpu.BindGroupEntry{},
			})
			if err != nil {
				t.Fatalf("CreateBindGroup released: %v", err)
			}
			pass.SetBindGroup(0, bgReleased, nil)
			bgReleased.Release()
		}
		pass.End()

		cmdBuf, err := enc.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}

		buf.Release()

		_, err = device.Queue().Submit(cmdBuf)
		if !errors.Is(err, wgpu.ErrSubmitBufferDestroyed) {
			t.Fatalf("iteration %d: Submit = %v, want ErrSubmitBufferDestroyed", i, err)
		}

		// Released here rather than deferred: over 16 iterations the deferred
		// form would hold every encoder and bind group to the end of the test.
		// The submit failed validation, so the command buffer must be released
		// to return its HAL encoder to the pool.
		cmdBuf.Release()
		bgLive.Release()
	}
}
