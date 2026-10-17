//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
)

// TestCoreCommandEncoderRecordTextureTransition verifies that an explicit
// barrier updates the command buffer tracker: the first explicit barrier on a
// texture defines its start state (the barrier's old usage), every barrier
// moves its end state.
func TestCoreCommandEncoderRecordTextureTransition(t *testing.T) {
	t.Parallel()

	device := NewDevice(&mockHALDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "explicit-transition")
	color := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc|gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "color",
	)
	depth := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatDepth32Float, gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc|gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "depth",
	)
	encoder, err := device.CreateCommandEncoder("explicit-transition")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	tracker := encoder.Mutable().TextureScope()

	encoder.RecordTextureTransition(color, gputypes.TextureUsageRenderAttachment, gputypes.TextureUsageCopySrc)
	colorIdx := color.TrackingData().Index()
	if got := tracker.StartUsage(colorIdx); got != track.TextureUsesColorTarget {
		t.Errorf("color start = %v, want ColorTarget", got)
	}
	if got := tracker.EndUsage(colorIdx); got != track.TextureUsesCopySrc {
		t.Errorf("color end = %v, want CopySrc", got)
	}
	encoder.RecordTextureTransition(color, gputypes.TextureUsageCopySrc, gputypes.TextureUsageRenderAttachment)
	if got := tracker.StartUsage(colorIdx); got != track.TextureUsesColorTarget {
		t.Errorf("color start after second barrier = %v, want unchanged ColorTarget", got)
	}
	if got := tracker.EndUsage(colorIdx); got != track.TextureUsesColorTarget {
		t.Errorf("color end after second barrier = %v, want ColorTarget", got)
	}

	// RenderAttachment on a depth format is a depth/stencil target.
	encoder.RecordTextureTransition(depth, 0, gputypes.TextureUsageRenderAttachment)
	depthIdx := depth.TrackingData().Index()
	if got := tracker.StartUsage(depthIdx); got != track.TextureUsesDepthStencilWrite {
		t.Errorf("depth start = %v, want DepthStencilWrite (undefined old usage falls back to new)", got)
	}
}

// TestCoreCommandEncoderRecordTextureTransitionGuards verifies that missing
// state or tracking data is ignored instead of panicking.
func TestCoreCommandEncoderRecordTextureTransitionGuards(t *testing.T) {
	t.Parallel()

	encoders := []*CoreCommandEncoder{
		{},
		{mutable: &CommandBufferMutable{}},
		{mutable: &CommandBufferMutable{textureTracker: track.NewEncoderTextureTracker()}},
	}
	for _, encoder := range encoders {
		encoder.RecordTextureTransition(nil, gputypes.TextureUsageCopySrc, gputypes.TextureUsageCopyDst)
	}

	encoder := &CoreCommandEncoder{
		mutable: &CommandBufferMutable{textureTracker: track.NewEncoderTextureTracker()},
	}
	encoder.RecordTextureTransition(&Texture{}, gputypes.TextureUsageCopySrc, gputypes.TextureUsageCopyDst)

	invalidTrackingData := NewTexture(
		nil, nil,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "invalid-tracking",
	)
	t.Cleanup(invalidTrackingData.Destroy)
	encoder.RecordTextureTransition(invalidTrackingData, gputypes.TextureUsageCopySrc, gputypes.TextureUsageCopyDst)

	if !encoder.mutable.textureTracker.IsEmpty() {
		t.Fatal("guarded texture transition unexpectedly mutated the tracker")
	}
}
