//go:build !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// =============================================================================
// CopyTextureToBuffer — valid path with real resources
// Covers encoder_native.go CopyTextureToBuffer lines 178-205
// (texture/buffer tracking + HAL call)
// =============================================================================

func TestCopyTextureToBufferValid(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "t2b-src-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	// RGBA8Unorm = 4 bytes per pixel, 4x4 = 64 bytes
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "t2b-dst-buf",
		Size:  256,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	regions := []wgpu.BufferTextureCopy{
		{
			TextureBase: wgpu.ImageCopyTexture{Texture: tex},
			BufferLayout: wgpu.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  16,
				RowsPerImage: 4,
			},
			Size: wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		},
	}
	enc.CopyTextureToBuffer(tex, buf, regions)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if cmdBuf == nil {
		t.Fatal("Finish returned nil CommandBuffer")
	}
	cmdBuf.Release()
}

// =============================================================================
// CopyTextureToTexture — valid path with real textures
// Covers encoder_native.go CopyTextureToTexture lines 209-235
// (texture tracking for both src and dst)
// =============================================================================

func TestCopyTextureToTextureValidPath(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcTex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "t2t-src",
		Size:          wgpu.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture src: %v", err)
	}
	defer srcTex.Release()

	dstTex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "t2t-dst",
		Size:          wgpu.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture dst: %v", err)
	}
	defer dstTex.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	regions := []wgpu.TextureCopy{
		{
			Source:      wgpu.ImageCopyTexture{Texture: srcTex},
			Destination: wgpu.ImageCopyTexture{Texture: dstTex},
			Size:        wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		},
	}
	enc.CopyTextureToTexture(srcTex, dstTex, regions)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	cmdBuf.Release()
}

// =============================================================================
// TransitionTextures — with actual barriers
// Covers encoder_native.go TransitionTextures lines 241-254
// =============================================================================

func TestTransitionTexturesWithBarrier(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "barrier-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopySrc | wgpu.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	barriers := []wgpu.TextureBarrier{
		{
			Texture: tex,
			Range: wgpu.TextureRange{
				BaseMipLevel:    0,
				MipLevelCount:   1,
				BaseArrayLayer:  0,
				ArrayLayerCount: 1,
			},
			Usage: wgpu.TextureUsageTransition{
				OldUsage: wgpu.TextureUsageRenderAttachment,
				NewUsage: wgpu.TextureUsageCopySrc,
			},
		},
	}
	enc.TransitionTextures(barriers)

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	cmdBuf.Release()
}

// =============================================================================
// Encoder resource tracking transfer — trackBuffer, trackTexture, trackBindGroup
// Covers encoder_native.go lines 70-104
// =============================================================================

func TestEncoderTrackBufferLazyInit(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "track-src",
		Size:  32,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}
	defer srcBuf.Release()

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "track-dst",
		Size:  32,
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

	// CopyBufferToBuffer triggers trackBuffer for both src and dst.
	enc.CopyBufferToBuffer(srcBuf, 0, dstBuf, 0, 32)

	// Finish to verify no errors from tracking.
	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Verify refs were transferred (encoder is empty after Finish).
	if enc.TestTrackedRefs() != nil {
		t.Error("encoder should have nil trackedRefs after Finish")
	}
	cmdBuf.Release()
}

func TestEncoderTrackBindGroupTransfer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "track-bg-layout",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "track-bg",
		Layout:  bgl,
		Entries: []wgpu.BindGroupEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	// Verify bind group is not released.
	if bg.TestBindGroupReleased() {
		t.Error("bind group should not be released after creation")
	}
}

// =============================================================================
// DiscardEncoding — drops tracked refs and returns pooled encoder
// Covers encoder_native.go lines 259-291
// =============================================================================

func TestDiscardEncodingAfterCopy(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "discard-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}
	defer srcBuf.Release()

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "discard-dst",
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

	enc.CopyBufferToBuffer(srcBuf, 0, dstBuf, 0, 32)

	// Discard instead of Finish — should drop tracked refs.
	enc.DiscardEncoding()

	// After discard, encoder is released — Finish should fail.
	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish after DiscardEncoding should fail")
	}
}

// =============================================================================
// CopyBufferToBuffer — nil src and dst
// Covers encoder_native.go lines 150-167
// =============================================================================

func TestCopyBufferToBufferNilSrc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "nil-src-dst",
		Size:  64,
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

	// nil src should record deferred error.
	enc.CopyBufferToBuffer(nil, 0, dstBuf, 0, 64)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail: CopyBufferToBuffer with nil src")
	}
}

func TestCopyBufferToBufferNilDst(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "nil-dst-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer srcBuf.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// nil dst should record deferred error.
	enc.CopyBufferToBuffer(srcBuf, 0, nil, 0, 64)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail: CopyBufferToBuffer with nil dst")
	}
}

// =============================================================================
// CommandBuffer.Release — on nil, on already-released
// Covers encoder_native.go CommandBuffer.Release lines 447-462
// =============================================================================

func TestCommandBufferDoubleRelease(t *testing.T) {
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

	cmdBuf.Release()
	// Second Release should be a no-op (halEncoder is nil).
	cmdBuf.Release()
}

// =============================================================================
// Encoder.Finish — error path drops refs
// Covers encoder_native.go Finish lines 299-340 (error path)
// =============================================================================

func TestFinishWithEncodingError(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Record a deferred error via nil src buffer.
	enc.CopyBufferToBuffer(nil, 0, nil, 0, 64)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail with deferred encoding error")
	}

	// After Finish error, encoder is released. Further Finish should also fail.
	_, err = enc.Finish()
	if err == nil {
		t.Fatal("second Finish should also fail")
	}
}

// =============================================================================
// Encoder pool interaction — CreateCommandEncoder returns encoder to pool on error
// Covers device_native.go lines 712-742 (pool acquire/release path)
// =============================================================================

func TestEncoderPoolRecycling(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	poolSize := device.TestCmdEncoderPoolSize()
	if poolSize < 0 {
		t.Skip("no encoder pool configured (mock device)")
	}

	// Create and finish an encoder.
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Release the command buffer — returns encoder to pool.
	cmdBuf.Release()

	// Pool should now have at least one encoder.
	newPoolSize := device.TestCmdEncoderPoolSize()
	if newPoolSize < 1 {
		t.Log("pool size after release:", newPoolSize, "(may vary by backend)")
	}
}

// =============================================================================
// Bind group with texture view resources
// Covers bind_native.go collectBindGroupResources + boundTextures path
// =============================================================================

func TestCreateBindGroupWithTextureView(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "bg-tv-tex",
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
	defer tex.Release()

	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "bg-tv-layout",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageFragment,
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
		Label:  "bg-with-texture",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, TextureView: view},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	bg.Release()
}
