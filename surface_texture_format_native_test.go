//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// The Texture wrappers derived from a SurfaceTexture must report the surface
// format: copy and upload validation reads it from the wrapper.
func TestSurfaceTextureWrappersCarryFormat(t *testing.T) {
	_, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	want := gputypes.TextureFormatRGBA8Unorm

	texture := surfaceTexture.AsTexture()
	if texture == nil {
		t.Fatal("AsTexture returned nil for acquired texture")
	}
	if got := texture.Format(); got != want {
		t.Errorf("AsTexture().Format() = %v, want %v", got, want)
	}

	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	if got := view.texture.Format(); got != want {
		t.Errorf("CreateView().texture.Format() = %v, want %v", got, want)
	}
}
