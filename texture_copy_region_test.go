//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestCopyTextureToTextureRegionWithoutTexture: a region whose source or
// destination texture is nil is an encoder error reported by Finish. It used
// to dereference nil while converting the region for the backend.
func TestCopyTextureToTextureRegionWithoutTexture(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newTexture := func(usage gputypes.TextureUsage) *wgpu.Texture {
		t.Helper()
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     wgpu.TextureDimension2D,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         usage,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	src := newTexture(wgpu.TextureUsageCopySrc)
	dst := newTexture(wgpu.TextureUsageCopyDst)
	size := wgpu.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}

	for _, tc := range []struct {
		name   string
		region wgpu.TextureCopy
	}{
		{"nil source", wgpu.TextureCopy{Destination: wgpu.ImageCopyTexture{Texture: dst}, Size: size}},
		{"nil destination", wgpu.TextureCopy{Source: wgpu.ImageCopyTexture{Texture: src}, Size: size}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			enc.CopyTextureToTexture(src, dst, []wgpu.TextureCopy{tc.region})
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish should fail for a region without a texture")
			}
		})
	}
}
