//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestDetachedTextureRelease guards the nil-device branch of Texture.Release:
// a texture created without a device and without a HAL object (the wgputest
// NewDetachedTexture handle) must release cleanly, twice, and must not reach
// destroyTextureDeferred.
func TestDetachedTextureRelease(t *testing.T) {
	tex := newDetachedTexture(nil)
	if got := tex.Format(); got != gputypes.TextureFormatUndefined {
		t.Fatalf("Format = %v, want Undefined", got)
	}
	tex.Release()
	tex.Release()
}

// TestDetachedTextureKeepsMetadata pins that the constructor copies the
// descriptor's format and view formats into the handle.
func TestDetachedTextureKeepsMetadata(t *testing.T) {
	view := []gputypes.TextureFormat{gputypes.TextureFormatRGBA8UnormSrgb}
	desc := &TextureDescriptor{
		Format:      gputypes.TextureFormatRGBA8Unorm,
		ViewFormats: view,
	}
	tex := newDetachedTexture(desc)
	defer tex.Release()

	if got := tex.Format(); got != gputypes.TextureFormatRGBA8Unorm {
		t.Fatalf("Format = %v, want RGBA8Unorm", got)
	}
	got := tex.impl.ViewFormatList()
	if len(got) != 1 || got[0] != gputypes.TextureFormatRGBA8UnormSrgb {
		t.Fatalf("viewFormats = %v, want [RGBA8UnormSrgb]", got)
	}

	// The handle owns its copy: mutating the caller's slice changes nothing.
	view[0] = gputypes.TextureFormatBGRA8Unorm
	if got := tex.impl.ViewFormatList(); got[0] != gputypes.TextureFormatRGBA8UnormSrgb {
		t.Fatalf("viewFormats aliased the caller's slice: %v", got)
	}
}
