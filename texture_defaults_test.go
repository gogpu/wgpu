package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestNormalizeTextureDescriptor: zero mip level count, sample count,
// dimension, height and depth take the WebGPU defaults; explicit values and
// the required width pass unchanged; the caller's descriptor is not modified.
func TestNormalizeTextureDescriptor(t *testing.T) {
	zero := &TextureDescriptor{Size: Extent3D{Width: 8}, Format: gputypes.TextureFormatRGBA8Unorm}
	got := normalizeTextureDescriptor(zero)
	want := TextureDescriptor{
		Size:          Extent3D{Width: 8, Height: 1, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D,
		Format:    gputypes.TextureFormatRGBA8Unorm,
	}
	if !textureDescEqual(got, &want) {
		t.Errorf("zero members: got %+v, want %+v", *got, want)
	}
	if zero.MipLevelCount != 0 || zero.Size.Height != 0 || zero.Dimension != gputypes.TextureDimensionUndefined {
		t.Error("normalizeTextureDescriptor changed the caller's descriptor")
	}

	explicit := &TextureDescriptor{
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 6},
		MipLevelCount: 3, SampleCount: 4,
		Dimension: gputypes.TextureDimension3D,
	}
	if got := normalizeTextureDescriptor(explicit); !textureDescEqual(got, explicit) {
		t.Errorf("explicit members: got %+v, want them unchanged %+v", *got, *explicit)
	}

	if got := normalizeTextureDescriptor(&TextureDescriptor{}); got.Size.Width != 0 {
		t.Errorf("width %d, want the required width left at 0 for validation to reject", got.Size.Width)
	}
}

func textureDescEqual(a, b *TextureDescriptor) bool {
	return a.Label == b.Label && a.Size == b.Size && a.MipLevelCount == b.MipLevelCount &&
		a.SampleCount == b.SampleCount && a.Dimension == b.Dimension && a.Format == b.Format &&
		a.Usage == b.Usage && len(a.ViewFormats) == len(b.ViewFormats)
}
