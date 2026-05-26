//go:build !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// =============================================================================
// CommandEncoder copy and barrier operations
// Covers encoder_native.go missed lines (CopyTextureToTexture,
// CopyTextureToBuffer, TransitionTextures, trackBuffer/trackTexture/
// trackBindGroup transfer to CommandBuffer)
// =============================================================================

func TestCopyTextureToTextureNilSrc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "copy-dst-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// nil src should record a deferred error.
	enc.CopyTextureToTexture(nil, tex, nil)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail: CopyTextureToTexture with nil src")
	}
}

func TestCopyTextureToTextureNilDst(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "copy-src-tex",
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

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// nil dst should record a deferred error.
	enc.CopyTextureToTexture(tex, nil, nil)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail: CopyTextureToTexture with nil dst")
	}
}

func TestCopyTextureToTextureValid(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcTex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "copy-t2t-src",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
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
		Label:         "copy-t2t-dst",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
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

	// Command buffer should not be nil.
	if cmdBuf == nil {
		t.Fatal("Finish returned nil CommandBuffer")
	}
	cmdBuf.Release()
}

func TestCopyTextureToBufferNilSrc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "copy-t2b-dst",
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

	// nil src texture should record a deferred error.
	enc.CopyTextureToBuffer(nil, buf, nil)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail: CopyTextureToBuffer with nil src")
	}
}

func TestCopyTextureToBufferNilDst(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "copy-t2b-src",
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

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// nil dst buffer should record a deferred error.
	enc.CopyTextureToBuffer(tex, nil, nil)

	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish should fail: CopyTextureToBuffer with nil dst")
	}
}

func TestTransitionTexturesEmpty(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Empty barriers should be a no-op.
	enc.TransitionTextures(nil)
	enc.TransitionTextures([]wgpu.TextureBarrier{})

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if cmdBuf == nil {
		t.Fatal("Finish returned nil")
	}
	cmdBuf.Release()
}

func TestTransitionTexturesReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Consume the encoder.
	_, err = enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// TransitionTextures on a consumed encoder should be a no-op (released=true).
	enc.TransitionTextures(nil)
}

// =============================================================================
// CommandEncoder resource tracking — trackBuffer, trackTexture, trackBindGroup
// Covers encoder_native.go lines 70-104 (tracking map initialization + transfer)
// =============================================================================

func TestEncoderTrackedRefsTransferOnFinish(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	srcBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "tracked-src",
		Size:  64,
		Usage: wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer src: %v", err)
	}
	defer srcBuf.Release()

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "tracked-dst",
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

	// After encoding, encoder should have tracked refs.
	refs := enc.TestTrackedRefs()
	if len(refs) == 0 {
		t.Log("encoder tracked refs may be nil on mock device (no core.Buffer.Ref)")
	}

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// After Finish, encoder refs should be transferred to CommandBuffer.
	if enc.TestTrackedRefs() != nil {
		t.Error("encoder tracked refs should be nil after Finish")
	}

	// CommandBuffer should carry the refs.
	cbRefs := cmdBuf.TestTrackedRefs()
	if len(refs) > 0 && len(cbRefs) == 0 {
		t.Error("command buffer should carry tracked refs from encoder")
	}

	cmdBuf.Release()
}

func TestCommandBufferReleaseFull(t *testing.T) {
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

	// Release should free the HAL encoder and tracked refs.
	cmdBuf.Release()

	// After Release, halEncoder should be nil.
	if cmdBuf.TestHALEncoder() != nil {
		t.Error("halEncoder should be nil after Release")
	}
	if cmdBuf.TestTrackedRefs() != nil {
		t.Error("trackedRefs should be nil after Release")
	}
}

// TestComputePass_DirectWriteTrackedRefs verifies that ComputePassEncoder.trackRef
// writes directly to the parent CommandEncoder's trackedRefs (not a pass-level
// intermediate slice). This eliminates per-pass backing array allocations and
// the copy at End(). Matches Rust wgpu tracker architecture.
func TestComputePass_DirectWriteTrackedRefs(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Before compute pass, encoder has no tracked refs.
	if refs := enc.TestTrackedRefs(); len(refs) != 0 {
		t.Fatalf("encoder should have 0 tracked refs before pass, got %d", len(refs))
	}

	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}

	// Create a bind group with a buffer to trigger trackRef in SetBindGroup.
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "direct-write-buf",
		Size:  64,
		Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "direct-write-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "direct-write-bg",
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Size: 64},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	pass.SetBindGroup(0, bg, nil)

	// After SetBindGroup INSIDE the pass, encoder should already have tracked refs
	// (direct write, no intermediate pass-level slice).
	refsBeforeEnd := enc.TestTrackedRefs()
	if len(refsBeforeEnd) == 0 {
		t.Log("encoder tracked refs empty — may be mock device without ResourceRef")
	} else {
		t.Logf("encoder has %d tracked refs after SetBindGroup (before End)", len(refsBeforeEnd))
	}

	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	// After End(), encoder refs should be the SAME count (no additional copy).
	refsAfterEnd := enc.TestTrackedRefs()
	if len(refsAfterEnd) != len(refsBeforeEnd) {
		t.Errorf("encoder refs changed at End(): before=%d, after=%d (should be same — direct write)",
			len(refsBeforeEnd), len(refsAfterEnd))
	}

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// After Finish, encoder should be nil (transferred to CB).
	if enc.TestTrackedRefs() != nil {
		t.Error("encoder should have nil trackedRefs after Finish")
	}

	// CommandBuffer should carry all refs.
	if cbRefs := cmdBuf.TestTrackedRefs(); len(refsBeforeEnd) > 0 && len(cbRefs) == 0 {
		t.Error("command buffer should carry tracked refs")
	}

	cmdBuf.Release()
}

func TestCommandBufferReleaseNil(t *testing.T) {
	// Release on nil CommandBuffer should not panic.
	var cb *wgpu.CommandBuffer
	cb.Release()
}

// =============================================================================
// DiscardEncoding tests
// Covers encoder_native.go lines 259-291
// =============================================================================

func TestDiscardEncodingReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Finish consumes the encoder.
	_, err = enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// DiscardEncoding on a consumed encoder should be a no-op.
	enc.DiscardEncoding()
}

func TestDiscardEncodingDropsTrackedRefs(t *testing.T) {
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

	enc.CopyBufferToBuffer(srcBuf, 0, dstBuf, 0, 64)

	enc.DiscardEncoding()

	// After discard, Finish should fail (encoder is released).
	_, err = enc.Finish()
	if err == nil {
		t.Fatal("Finish after DiscardEncoding should fail")
	}
}

// =============================================================================
// BeginRenderPass / BeginComputePass on released encoder
// Covers encoder_native.go released guard clauses
// =============================================================================

func TestBeginRenderPassReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	_, _ = enc.Finish()

	_, err = enc.BeginRenderPass(nil)
	if err == nil {
		t.Fatal("BeginRenderPass on consumed encoder should fail")
	}
}

func TestBeginComputePassReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	_, _ = enc.Finish()

	_, err = enc.BeginComputePass(nil)
	if err == nil {
		t.Fatal("BeginComputePass on consumed encoder should fail")
	}
}

// =============================================================================
// CopyBufferToBuffer on released encoder — no-op
// Covers encoder_native.go line 147
// =============================================================================

func TestCopyBufferToBufferReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Consume the encoder.
	_, _ = enc.Finish()

	// CopyBufferToBuffer on released encoder is a no-op.
	enc.CopyBufferToBuffer(nil, 0, nil, 0, 0)
}

// =============================================================================
// CopyTextureToTexture on released encoder — no-op
// Covers encoder_native.go line 211
// =============================================================================

func TestCopyTextureToTextureReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	_, _ = enc.Finish()

	// CopyTextureToTexture on released encoder is a no-op.
	enc.CopyTextureToTexture(nil, nil, nil)
}

// =============================================================================
// CopyTextureToBuffer on released encoder — no-op
// Covers encoder_native.go line 181
// =============================================================================

func TestCopyTextureToBufferReleasedEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	_, _ = enc.Finish()

	// CopyTextureToBuffer on released encoder is a no-op.
	enc.CopyTextureToBuffer(nil, nil, nil)
}

// BenchmarkComputePassTrackedRefs measures allocation overhead of Phase 2
// resource tracking in a Born ML-like workload: N dispatches per pass,
// each SetBindGroup tracking ~2 refs (BindGroup + Buffer).
func BenchmarkComputePassTrackedRefs(b *testing.B) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		b.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		b.Fatalf("RequestAdapter: %v", err)
	}
	defer adapter.Release()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		b.Fatalf("RequestDevice: %v", err)
	}
	defer device.Release()

	q := device.Queue()
	if q == nil {
		b.Skip("skipping: device has no HAL integration")
	}

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "bench-buf",
		Size:  64,
		Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		b.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "bench-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	})
	if err != nil {
		b.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bench-bg",
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Size: 64},
		},
	})
	if err != nil {
		b.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	// Baseline: encoder lifecycle without SetBindGroup.
	b.Run("baseline-no-bindgroup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			enc, _ := device.CreateCommandEncoder(nil)
			pass, _ := enc.BeginComputePass(nil)
			pass.End()
			cb, _ := enc.Finish()
			cb.Release()
		}
	})

	// 100 SetBindGroup calls (Born ML: ~100 dispatches per encoder).
	b.Run("100-setbindgroup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			enc, _ := device.CreateCommandEncoder(nil)
			pass, _ := enc.BeginComputePass(nil)
			for j := 0; j < 100; j++ {
				pass.SetBindGroup(0, bg, nil)
			}
			pass.End()
			cb, _ := enc.Finish()
			cb.Release()
		}
	})

	// 1000 SetBindGroup calls (stress test).
	b.Run("1000-setbindgroup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			enc, _ := device.CreateCommandEncoder(nil)
			pass, _ := enc.BeginComputePass(nil)
			for j := 0; j < 1000; j++ {
				pass.SetBindGroup(0, bg, nil)
			}
			pass.End()
			cb, _ := enc.Finish()
			cb.Release()
		}
	})
}
