//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// These are the copy-usage coverage tests of the root package, moved with the
// declarations they exercise (S4's Move). They build their objects through the
// internal constructors instead of the public wrappers.

// copyBarrierCaptureEncoder records the barriers a copy command emits.
type copyBarrierCaptureEncoder struct {
	hal.CommandEncoder
	textureBarriers []hal.TextureBarrier
	bufferBarriers  []hal.BufferBarrier
}

func (e *copyBarrierCaptureEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	e.textureBarriers = append(e.textureBarriers, barriers...)
}

func (e *copyBarrierCaptureEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	e.bufferBarriers = append(e.bufferBarriers, barriers...)
}

func TestCopyUsageHelpersSkipUntrackedResources(t *testing.T) {
	t.Parallel()

	encoder := &CommandEncoder{}
	if !encoder.recordCopyUsages(nil, nil, []copyBufferUsage{{usage: track.BufferUsesCopySrc}}) {
		t.Fatal("encoder without core state should ignore copy usage")
	}

	invalidCoreTexture := core.NewTexture(
		nil, nil,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "invalid-tracking",
	)
	t.Cleanup(invalidCoreTexture.Destroy)

	textures, err := prepareCopyTextureUsages([]copyTextureUsage{
		{texture: nil, usage: track.TextureUsesCopySrc},
		{texture: resource.NewTexture(nil, nil, gputypes.TextureFormatRGBA8Unorm, nil, nil), usage: track.TextureUsesCopySrc},
		{texture: resource.NewTexture(nil, nil, gputypes.TextureFormatRGBA8Unorm, nil, &core.Texture{}), usage: track.TextureUsesCopySrc},
		{texture: resource.NewTexture(nil, nil, gputypes.TextureFormatRGBA8Unorm, nil, invalidCoreTexture), usage: track.TextureUsesCopySrc},
	})
	if err != nil {
		t.Fatalf("prepareCopyTextureUsages: %v", err)
	}
	if len(textures) != 0 {
		t.Fatalf("prepared %d untracked textures, want 0", len(textures))
	}

	invalidCoreBuffer := core.NewBuffer(nil, nil, gputypes.BufferUsageCopySrc, 4, "invalid-tracking")
	t.Cleanup(invalidCoreBuffer.Destroy)
	buffers, err := prepareCopyBufferUsages([]copyBufferUsage{
		{buffer: nil, usage: track.BufferUsesCopySrc},
		{buffer: resource.NewBufferFromCore(nil), usage: track.BufferUsesCopySrc},
		{buffer: resource.NewBufferFromCore(&core.Buffer{}), usage: track.BufferUsesCopySrc},
		{buffer: resource.NewBufferFromCore(invalidCoreBuffer), usage: track.BufferUsesCopySrc},
	})
	if err != nil {
		t.Fatalf("prepareCopyBufferUsages: %v", err)
	}
	if len(buffers) != 0 {
		t.Fatalf("prepared %d untracked buffers, want 0", len(buffers))
	}
}

// TestCopyUsageScopeIsPerCommand checks the conflict rules inside ONE copy
// command: compatible roles of the same resource merge, incompatible roles
// (copy onto itself) are rejected.
func TestCopyUsageScopeIsPerCommand(t *testing.T) {
	t.Parallel()

	device, view := newTestCoreDevice(t)
	texture := newTrackedTexture(t, device, view, "per-command-texture", gputypes.TextureUsageCopySrc|gputypes.TextureUsageCopyDst)
	t.Cleanup(texture.Release)
	buffer := newTrackedBuffer(t, device, view, "per-command-buffer")
	t.Cleanup(buffer.Release)

	textures, err := prepareCopyTextureUsages([]copyTextureUsage{
		{texture: texture, usage: track.TextureUsesCopySrc},
		{texture: texture, usage: track.TextureUsesResource},
	})
	if err != nil {
		t.Fatalf("compatible texture roles rejected: %v", err)
	}
	if len(textures) != 1 || textures[0].usage != track.TextureUsesCopySrc|track.TextureUsesResource {
		t.Fatalf("prepared texture roles = %+v, want one CopySrc|Resource entry", textures)
	}
	_, err = prepareCopyTextureUsages([]copyTextureUsage{
		{texture: texture, usage: track.TextureUsesCopySrc},
		{texture: texture, usage: track.TextureUsesCopyDst},
	})
	var textureConflict *track.TextureUsageConflictError
	if !errors.As(err, &textureConflict) {
		t.Fatalf("texture copied onto itself = %v, want TextureUsageConflictError", err)
	}
	if textureConflict.Label != "per-command-texture" {
		t.Errorf("conflict label = %q, want the texture's, %q", textureConflict.Label, "per-command-texture")
	}

	buffers, err := prepareCopyBufferUsages([]copyBufferUsage{
		{buffer: buffer, usage: track.BufferUsesCopySrc},
		{buffer: buffer, usage: track.BufferUsesUniform},
	})
	if err != nil {
		t.Fatalf("compatible buffer roles rejected: %v", err)
	}
	if len(buffers) != 1 || buffers[0].usage != track.BufferUsesCopySrc|track.BufferUsesUniform {
		t.Fatalf("prepared buffer roles = %+v, want one CopySrc|Uniform entry", buffers)
	}
	_, err = prepareCopyBufferUsages([]copyBufferUsage{
		{buffer: buffer, usage: track.BufferUsesCopySrc},
		{buffer: buffer, usage: track.BufferUsesCopyDst},
	})
	var bufferConflict *track.UsageConflictError
	if !errors.As(err, &bufferConflict) {
		t.Fatalf("buffer copied onto itself = %v, want UsageConflictError", err)
	}
}

// TestCopyRecordsTransitionsFromEarlierCommands checks that a copy following
// another command on the same resources records the barriers from the
// earlier usage before the copy, and that a first use records none (the
// submit-time barrier covers it).
func TestCopyRecordsTransitionsFromEarlierCommands(t *testing.T) {
	t.Parallel()

	device, view := newTestCoreDevice(t)
	texture := newTrackedTexture(t, device, view, "transition-texture", gputypes.TextureUsageCopySrc)
	t.Cleanup(texture.Release)
	buffer := newTrackedBuffer(t, device, view, "transition-buffer")
	t.Cleanup(buffer.Release)
	encoder := &CommandEncoder{core: newTestCoreEncoder(t, device)}
	t.Cleanup(encoder.DiscardEncoding)

	texIndex := texture.CoreTexture().TrackingData().Index()
	bufIndex := buffer.CoreBuffer().TrackingData().Index()
	request := func(raw hal.CommandEncoder) bool {
		return encoder.recordCopyUsages(raw,
			[]copyTextureUsage{{texture: texture, usage: track.TextureUsesCopySrc}},
			[]copyBufferUsage{{buffer: buffer, usage: track.BufferUsesCopyDst}})
	}

	first := &copyBarrierCaptureEncoder{}
	if !request(first) {
		t.Fatal("first copy rejected")
	}
	if len(first.textureBarriers) != 0 || len(first.bufferBarriers) != 0 {
		t.Fatalf("first use recorded barriers: textures=%d buffers=%d, want 0",
			len(first.textureBarriers), len(first.bufferBarriers))
	}

	// An earlier command (a render pass) left the texture as a color target
	// and the buffer as a vertex buffer.
	encoder.core.Mutable().TextureScope().AppendSingle(nil, texIndex, track.TextureSelector{}, track.TextureUsesColorTarget)
	encoder.core.Mutable().BufferScope().SetSingle(bufIndex, track.BufferUsesVertex)

	second := &copyBarrierCaptureEncoder{}
	if !request(second) {
		t.Fatal("copy after an earlier command was rejected")
	}
	if len(second.textureBarriers) != 1 {
		t.Fatalf("texture barriers = %d, want 1", len(second.textureBarriers))
	}
	if got := second.textureBarriers[0].Usage; got.OldUsage != gputypes.TextureUsageRenderAttachment ||
		got.NewUsage != gputypes.TextureUsageCopySrc {
		t.Errorf("texture barrier = %v -> %v, want RenderAttachment -> CopySrc", got.OldUsage, got.NewUsage)
	}
	if len(second.bufferBarriers) != 1 {
		t.Fatalf("buffer barriers = %d, want 1", len(second.bufferBarriers))
	}
	if got := second.bufferBarriers[0].Usage; got.OldUsage != gputypes.BufferUsageVertex ||
		got.NewUsage != gputypes.BufferUsageCopyDst {
		t.Errorf("buffer barrier = %v -> %v, want Vertex -> CopyDst", got.OldUsage, got.NewUsage)
	}
	if got := encoder.core.Mutable().TextureScope().StartUsage(texIndex); got != track.TextureUsesCopySrc {
		t.Errorf("texture start usage = %v, want CopySrc (first use)", got)
	}
}

func TestCopyBufferToBufferRejectsReleasedEndpoint(t *testing.T) {
	t.Parallel()

	device, view := newTestCoreDevice(t)
	src := newTrackedBuffer(t, device, view, "released-copy-src")
	dst := newTrackedBuffer(t, device, view, "released-copy-dst")
	t.Cleanup(dst.Release)
	src.Release()

	encoder := &CommandEncoder{core: newTestCoreEncoder(t, device)}
	encoder.CopyBufferToBuffer(src, 0, dst, 0, 4)
	if _, err := encoder.Finish(); err == nil {
		t.Fatal("Finish succeeded after copying from a released buffer")
	}
}

// TestTransitionTexturesUpdatesEncoderTracker checks that an explicit barrier
// moves the tracker to the barrier's new state, so a following copy does not
// repeat the transition (gg's MSAA-resolve readback pattern).
func TestTransitionTexturesUpdatesEncoderTracker(t *testing.T) {
	t.Parallel()

	device, view := newTestCoreDevice(t)
	texture := newTrackedTexture(t, device, view, "explicit-transition", gputypes.TextureUsageCopySrc|gputypes.TextureUsageCopyDst)
	t.Cleanup(texture.Release)
	encoder := &CommandEncoder{core: newTestCoreEncoder(t, device)}

	encoder.TransitionTextures([]TextureBarrier{{
		Texture: texture,
		Usage: TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})
	idx := texture.CoreTexture().TrackingData().Index()
	tracker := encoder.core.Mutable().TextureScope()
	if got := tracker.StartUsage(idx); got != track.TextureUsesColorTarget {
		t.Errorf("start usage = %v, want ColorTarget (explicit barrier's old usage)", got)
	}
	if got := tracker.EndUsage(idx); got != track.TextureUsesCopySrc {
		t.Errorf("end usage = %v, want CopySrc", got)
	}

	capture := &copyBarrierCaptureEncoder{}
	if !encoder.recordCopyUsages(capture,
		[]copyTextureUsage{{texture: texture, usage: track.TextureUsesCopySrc}}, nil) {
		t.Fatal("copy after explicit transition rejected")
	}
	if len(capture.textureBarriers) != 0 {
		t.Errorf("copy repeated the explicit transition: %d barriers, want 0", len(capture.textureBarriers))
	}

	encoder.TransitionTextures([]TextureBarrier{{
		Texture: texture,
		Usage: TextureUsageTransition{
			OldUsage: gputypes.TextureUsageCopySrc,
			NewUsage: gputypes.TextureUsageRenderAttachment,
		},
	}})
	if got := tracker.EndUsage(idx); got != track.TextureUsesColorTarget {
		t.Errorf("end usage after transition back = %v, want ColorTarget", got)
	}
	commandBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	commandBuffer.Release()
}

// TestTransitionTexturesRecordsItsSubresources checks that an explicit
// barrier over one mip level moves that level alone in the encoder tracker:
// a following copy into the whole texture transitions that level from the
// barrier's new usage, and records no barrier for the level the explicit
// one left alone, used here for the first time.
func TestTransitionTexturesRecordsItsSubresources(t *testing.T) {
	t.Parallel()

	device, view := newTestCoreDevice(t)
	texture := newTrackedTextureWithMipLevels(t, device, view, "explicit-subresource",
		gputypes.TextureUsageCopySrc|gputypes.TextureUsageCopyDst, 2)
	t.Cleanup(texture.Release)
	encoder := &CommandEncoder{core: newTestCoreEncoder(t, device)}
	t.Cleanup(encoder.DiscardEncoding)

	encoder.TransitionTextures([]TextureBarrier{{
		Texture: texture,
		Range:   TextureRange{BaseMipLevel: 1, MipLevelCount: 1},
		Usage: TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

	capture := &copyBarrierCaptureEncoder{}
	if !encoder.recordCopyUsages(capture,
		[]copyTextureUsage{{texture: texture, usage: track.TextureUsesCopyDst}}, nil) {
		t.Fatal("copy after the explicit transition rejected")
	}
	if len(capture.textureBarriers) != 1 {
		t.Fatalf("copy recorded %d texture barriers, want 1, for the level the explicit barrier moved",
			len(capture.textureBarriers))
	}
	barrier := capture.textureBarriers[0]
	if barrier.Range.BaseMipLevel != 1 || barrier.Range.MipLevelCount != 1 {
		t.Errorf("barrier range = %+v, want mip level 1 alone", barrier.Range)
	}
	if barrier.Usage.OldUsage != gputypes.TextureUsageCopySrc || barrier.Usage.NewUsage != gputypes.TextureUsageCopyDst {
		t.Errorf("barrier = %v -> %v, want CopySrc -> CopyDst", barrier.Usage.OldUsage, barrier.Usage.NewUsage)
	}
}

// TestTransitionTexturesRefusesRangeOutsideTexture checks that a barrier
// whose range selects a mip level or array layer its texture does not have
// fails the encoder instead of being narrowed to the texture.
func TestTransitionTexturesRefusesRangeOutsideTexture(t *testing.T) {
	t.Parallel()

	for _, r := range []TextureRange{
		{BaseMipLevel: 1, MipLevelCount: 5},
		{BaseMipLevel: 2},
		{BaseArrayLayer: 1},
		{ArrayLayerCount: 2},
	} {
		device, view := newTestCoreDevice(t)
		texture := newTrackedTextureWithMipLevels(t, device, view, "out-of-range",
			gputypes.TextureUsageCopySrc|gputypes.TextureUsageCopyDst, 2)
		t.Cleanup(texture.Release)
		encoder := &CommandEncoder{core: newTestCoreEncoder(t, device)}

		encoder.TransitionTextures([]TextureBarrier{{
			Texture: texture,
			Range:   r,
			Usage: TextureUsageTransition{
				OldUsage: gputypes.TextureUsageRenderAttachment,
				NewUsage: gputypes.TextureUsageCopySrc,
			},
		}})
		if _, err := encoder.Finish(); err == nil {
			t.Errorf("range %+v of a 2-level, 1-layer texture: Finish succeeded, want the barrier refused", r)
		}
	}
}

// TestClearBufferTracksCopyDstUsage: ClearBuffer writes the buffer as a copy
// destination, so the encoder tracker moves it to COPY_DST (ordering it after
// the earlier vertex use) and the encoder keeps the buffer alive until the
// submission completes (Rust wgpu-core clear.rs clear_buffer).
func TestClearBufferTracksCopyDstUsage(t *testing.T) {
	t.Parallel()

	device, view := newTestCoreDevice(t)
	buffer := newTrackedBuffer(t, device, view, "clear-tracked")
	t.Cleanup(buffer.Release)
	encoder := &CommandEncoder{core: newTestCoreEncoder(t, device)}
	t.Cleanup(encoder.DiscardEncoding)

	index := buffer.CoreBuffer().TrackingData().Index()
	encoder.core.Mutable().BufferScope().SetSingle(index, track.BufferUsesVertex)

	encoder.ClearBuffer(buffer, 0, 16)

	if got := encoder.core.Mutable().BufferScope().EndUsage(index); got != track.BufferUsesCopyDst {
		t.Errorf("buffer usage after ClearBuffer = %v, want CopyDst", got)
	}
	if len(encoder.TrackedRefs()) != 1 {
		t.Errorf("tracked refs = %d, want 1", len(encoder.TrackedRefs()))
	}
	if _, ok := encoder.UsedBuffers()[buffer]; !ok {
		t.Error("buffer is not recorded as used by the encoder")
	}
}
