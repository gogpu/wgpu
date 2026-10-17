//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// copyBarrierCaptureEncoder records the barriers a copy command emits.
type copyBarrierCaptureEncoder struct {
	noop.CommandEncoder
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
		{texture: &Texture{}, usage: track.TextureUsesCopySrc},
		{texture: &Texture{coreTexture: &core.Texture{}}, usage: track.TextureUsesCopySrc},
		{texture: &Texture{coreTexture: invalidCoreTexture}, usage: track.TextureUsesCopySrc},
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
		{buffer: &Buffer{}, usage: track.BufferUsesCopySrc},
		{buffer: &Buffer{core: &core.Buffer{}}, usage: track.BufferUsesCopySrc},
		{buffer: &Buffer{core: invalidCoreBuffer}, usage: track.BufferUsesCopySrc},
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

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	texture := createCopyScopeTexture(t, device, "per-command-texture", TextureUsageCopySrc|TextureUsageCopyDst)
	defer texture.Release()
	buffer := createCopyScopeBuffer(t, device, "per-command-buffer")
	defer buffer.Release()

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

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	texture := createCopyScopeTexture(t, device, "transition-texture", TextureUsageCopySrc)
	defer texture.Release()
	buffer := createCopyScopeBuffer(t, device, "transition-buffer")
	defer buffer.Release()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	defer encoder.DiscardEncoding()

	texIndex := texture.coreTexture.TrackingData().Index()
	bufIndex := buffer.core.TrackingData().Index()
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
	encoder.core.Mutable().TextureScope().SetSingle(texIndex, track.TextureUsesColorTarget)
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

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	src := createCopyScopeBuffer(t, device, "released-copy-src")
	dst := createCopyScopeBuffer(t, device, "released-copy-dst")
	defer dst.Release()
	src.Release()

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
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

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	texture := createCopyScopeTexture(t, device, "explicit-transition", TextureUsageCopySrc|TextureUsageCopyDst)
	defer texture.Release()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	encoder.TransitionTextures([]TextureBarrier{{
		Texture: texture,
		Usage: TextureUsageTransition{
			OldUsage: TextureUsageRenderAttachment,
			NewUsage: TextureUsageCopySrc,
		},
	}})
	idx := texture.coreTexture.TrackingData().Index()
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
			OldUsage: TextureUsageCopySrc,
			NewUsage: TextureUsageRenderAttachment,
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
