//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
)

func TestCoreCommandEncoderRecordTextureUsage(t *testing.T) {
	t.Parallel()

	device := NewDevice(&mockHALDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "copy-scope")
	texture := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc|gputypes.TextureUsageCopyDst,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "copy-texture",
	)
	encoder, err := device.CreateCommandEncoder("copy-scope")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	if err := encoder.RecordTextureUsage(texture, track.TextureUsesCopySrc); err != nil {
		t.Fatalf("RecordTextureUsage: %v", err)
	}
	idx := texture.TrackingData().Index()
	if got := encoder.Mutable().TextureScope().GetUsage(idx); got != track.TextureUsesCopySrc {
		t.Fatalf("usage = %v, want CopySrc", got)
	}
	if err := encoder.RecordTextureUsage(texture, track.TextureUsesCopyDst); err == nil {
		t.Fatal("incompatible CopyDst usage succeeded")
	}

	encoder.ReplaceTextureUsage(texture, track.TextureUsesCopyDst)
	if got := encoder.Mutable().TextureScope().GetUsage(idx); got != track.TextureUsesCopyDst {
		t.Fatalf("replacement usage = %v, want CopyDst", got)
	}

	if err := encoder.RecordTextureUsage(nil, track.TextureUsesCopySrc); err != nil {
		t.Fatalf("nil texture should be ignored: %v", err)
	}
	encoder.ReplaceTextureUsage(nil, track.TextureUsesCopySrc)
}
