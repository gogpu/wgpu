//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestCreateTextureZeroDefaults: on the native path a descriptor that leaves
// MipLevelCount, SampleCount, Dimension, Size.Height and
// Size.DepthOrArrayLayers at zero creates a 2D texture with one mip level,
// one sample, height 1 and one layer (the WebGPU defaults), instead of
// failing validation. A zero width is still rejected.
func TestCreateTextureZeroDefaults(t *testing.T) {
	inst, _, device := newTestDevice(t)
	defer inst.Release()
	defer device.Release()

	desc := &TextureDescriptor{
		Label:  "zero-defaults",
		Size:   Extent3D{Width: 16},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageTextureBinding,
	}
	tex, err := device.CreateTexture(desc)
	if err != nil {
		t.Fatalf("CreateTexture with zero defaults: %v", err)
	}
	defer tex.Release()

	ct := tex.coreTexture
	if ct == nil {
		t.Fatal("texture has no core texture")
	}
	if got, want := ct.Size(), (gputypes.Extent3D{Width: 16, Height: 1, DepthOrArrayLayers: 1}); got != want {
		t.Errorf("size %+v, want %+v", got, want)
	}
	if ct.Dimension() != gputypes.TextureDimension2D || ct.MipLevelCount() != 1 || ct.SampleCount() != 1 {
		t.Errorf("dimension %v, mips %d, samples %d, want 2d, 1, 1", ct.Dimension(), ct.MipLevelCount(), ct.SampleCount())
	}
	if desc.MipLevelCount != 0 || desc.Size.Height != 0 {
		t.Error("CreateTexture changed the caller's descriptor")
	}

	if _, err := device.CreateTexture(&TextureDescriptor{Format: gputypes.TextureFormatRGBA8Unorm, Usage: gputypes.TextureUsageTextureBinding}); err == nil {
		t.Error("CreateTexture with zero width succeeded, want a validation error")
	}
}
