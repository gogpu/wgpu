//go:build !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

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
