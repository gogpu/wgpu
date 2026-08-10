//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
)

func TestCoreCommandEncoderTextureUsageGuards(t *testing.T) {
	t.Parallel()

	encoders := []*CoreCommandEncoder{
		{},
		{mutable: &CommandBufferMutable{}},
		{mutable: &CommandBufferMutable{textureScope: track.NewTextureUsageScope()}},
	}
	for i, encoder := range encoders {
		if err := encoder.RecordTextureUsage(nil, track.TextureUsesCopySrc); err != nil {
			t.Fatalf("encoder %d RecordTextureUsage(nil): %v", i, err)
		}
		encoder.ReplaceTextureUsage(nil, track.TextureUsesCopySrc)
	}

	encoder := &CoreCommandEncoder{
		mutable: &CommandBufferMutable{textureScope: track.NewTextureUsageScope()},
	}
	withoutTrackingData := &Texture{}
	if err := encoder.RecordTextureUsage(withoutTrackingData, track.TextureUsesCopySrc); err != nil {
		t.Fatalf("RecordTextureUsage without tracking data: %v", err)
	}
	encoder.ReplaceTextureUsage(withoutTrackingData, track.TextureUsesCopySrc)

	invalidTrackingData := NewTexture(
		nil, nil,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "invalid-tracking",
	)
	t.Cleanup(invalidTrackingData.Destroy)
	if err := encoder.RecordTextureUsage(invalidTrackingData, track.TextureUsesCopySrc); err != nil {
		t.Fatalf("RecordTextureUsage with invalid tracking data: %v", err)
	}
	encoder.ReplaceTextureUsage(invalidTrackingData, track.TextureUsesCopySrc)

	if !encoder.mutable.textureScope.IsEmpty() {
		t.Fatal("guarded texture usage unexpectedly mutated the scope")
	}
}
