//go:build !(js && wasm)

package software

import (
	"bytes"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func configuredReadPixelsSurface(t *testing.T, format gputypes.TextureFormat) *Surface {
	t.Helper()

	surface := &Surface{targetKind: hal.SurfaceTargetHeadless}
	if err := surface.Configure(nil, &hal.SurfaceConfiguration{
		Width:       2,
		Height:      1,
		Format:      format,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	t.Cleanup(func() { surface.Unconfigure(nil) })
	return surface
}

func TestSurfaceReadPixelsUnconfigured(t *testing.T) {
	surface := &Surface{targetKind: hal.SurfaceTargetHeadless}
	if pixels := surface.ReadPixels(); pixels != nil {
		t.Fatalf("ReadPixels = %v, want nil before Configure", pixels)
	}
}

func TestSurfaceReadPixelsFormatsAndOwnership(t *testing.T) {
	want := []byte{
		0x11, 0x22, 0x33, 0x44,
		0xaa, 0xbb, 0xcc, 0xdd,
	}
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		bgra   bool
	}{
		{name: "RGBA8Unorm", format: gputypes.TextureFormatRGBA8Unorm},
		{name: "RGBA8UnormSrgb", format: gputypes.TextureFormatRGBA8UnormSrgb},
		{name: "BGRA8Unorm", format: gputypes.TextureFormatBGRA8Unorm, bgra: true},
		{name: "BGRA8UnormSrgb", format: gputypes.TextureFormatBGRA8UnormSrgb, bgra: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			surface := configuredReadPixelsSurface(t, test.format)
			if err := surface.WritePixels(want, 2, 1); err != nil {
				t.Fatalf("WritePixels: %v", err)
			}

			if test.bgra {
				wantStored := []byte{
					0x33, 0x22, 0x11, 0x44,
					0xcc, 0xbb, 0xaa, 0xdd,
				}
				if !bytes.Equal(surface.framebuffer, wantStored) {
					t.Fatalf("stored framebuffer = %v, want BGRA %v", surface.framebuffer, wantStored)
				}
			}

			first := surface.ReadPixels()
			if !bytes.Equal(first, want) {
				t.Fatalf("ReadPixels = %v, want RGBA %v", first, want)
			}
			if len(first) != 2*1*4 {
				t.Fatalf("ReadPixels length = %d, want 8", len(first))
			}

			first[0] ^= 0xff
			second := surface.ReadPixels()
			if !bytes.Equal(second, want) {
				t.Fatalf("second ReadPixels = %v after caller mutation, want %v", second, want)
			}
		})
	}
}

func TestSurfaceGetFramebufferCompatibility(t *testing.T) {
	surface := configuredReadPixelsSurface(t, gputypes.TextureFormatBGRA8Unorm)
	want := []byte{0xf1, 0x82, 0x13, 0xff, 0x27, 0x38, 0x49, 0x5a}
	if err := surface.WritePixels(want, 2, 1); err != nil {
		t.Fatalf("WritePixels: %v", err)
	}
	if got := surface.GetFramebuffer(); !bytes.Equal(got, want) {
		t.Fatalf("GetFramebuffer = %v, want %v", got, want)
	}
}
