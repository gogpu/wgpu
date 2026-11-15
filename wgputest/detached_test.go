package wgputest_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest"
)

// TestDetachedTextureNil pins the nil-descriptor contract: zero metadata, and
// Release twice is a no-op rather than a panic.
func TestDetachedTextureNil(t *testing.T) {
	tex := wgputest.NewDetachedTexture(nil)
	if got := tex.Format(); got != gputypes.TextureFormatUndefined {
		t.Fatalf("Format = %v, want Undefined for a nil descriptor", got)
	}
	tex.Release()
	tex.Release()
}

// TestDetachedTextureMetadata pins that the handle carries the descriptor's
// format, which is all logic-only tests may read from it.
func TestDetachedTextureMetadata(t *testing.T) {
	tex := wgputest.NewDetachedTexture(&wgpu.TextureDescriptor{Format: gputypes.TextureFormatRGBA8Unorm})
	if got := tex.Format(); got != gputypes.TextureFormatRGBA8Unorm {
		t.Fatalf("Format = %v, want RGBA8Unorm", got)
	}
	tex.Release()
}
