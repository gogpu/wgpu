package browser

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestCompositeAlphaModeToJS verifies all composite alpha mode mappings.
func TestCompositeAlphaModeToJS(t *testing.T) {
	tests := []struct {
		mode gputypes.CompositeAlphaMode
		want string
	}{
		{gputypes.CompositeAlphaModeAuto, "opaque"},
		{gputypes.CompositeAlphaModeOpaque, "opaque"},
		{gputypes.CompositeAlphaModePremultiplied, "premultiplied"},
		{gputypes.CompositeAlphaModeUnpremultiplied, "opaque"}, // not supported on web, falls back
		{gputypes.CompositeAlphaModeInherit, "opaque"},         // not supported on web, falls back
	}
	for _, tc := range tests {
		got := CompositeAlphaModeToJS(tc.mode)
		if got != tc.want {
			t.Errorf("CompositeAlphaModeToJS(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestPresentModeToJS verifies that all present modes return "fifo" on browser.
func TestPresentModeToJS(t *testing.T) {
	tests := []struct {
		mode gputypes.PresentMode
		want string
	}{
		{gputypes.PresentModeFifo, "fifo"},
		{gputypes.PresentModeFifoRelaxed, "fifo"},
		{gputypes.PresentModeImmediate, "fifo"},
		{gputypes.PresentModeMailbox, "fifo"},
		{gputypes.PresentModeUndefined, "fifo"},
	}
	for _, tc := range tests {
		got := PresentModeToJS(tc.mode)
		if got != tc.want {
			t.Errorf("PresentModeToJS(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestTextureFormatFromJS verifies the reverse mapping from JS strings to Go format constants.
func TestTextureFormatFromJS(t *testing.T) {
	tests := []struct {
		jsStr string
		want  gputypes.TextureFormat
	}{
		// Canvas preferred formats (the primary use case)
		{"bgra8unorm", gputypes.TextureFormatBGRA8Unorm},
		{"rgba8unorm", gputypes.TextureFormatRGBA8Unorm},
		{"rgba16float", gputypes.TextureFormatRGBA16Float},

		// Spot check other common formats
		{"r8unorm", gputypes.TextureFormatR8Unorm},
		{"depth32float", gputypes.TextureFormatDepth32Float},
		{"bc1-rgba-unorm", gputypes.TextureFormatBC1RGBAUnorm},

		// Unknown returns Undefined
		{"", gputypes.TextureFormatUndefined},
		{"nonexistent-format", gputypes.TextureFormatUndefined},
	}
	for _, tc := range tests {
		got := TextureFormatFromJS(tc.jsStr)
		if got != tc.want {
			t.Errorf("TextureFormatFromJS(%q) = %v, want %v", tc.jsStr, got, tc.want)
		}
	}
}

// TestTextureFormatRoundTrip verifies that every format in the forward map
// can be recovered by the reverse map, and vice versa.
func TestTextureFormatRoundTrip(t *testing.T) {
	for goFmt, jsStr := range textureFormatMap {
		recovered := TextureFormatFromJS(jsStr)
		if recovered != goFmt {
			t.Errorf("round trip failed: TextureFormatToJS(%v) = %q, TextureFormatFromJS(%q) = %v, want %v",
				goFmt, jsStr, jsStr, recovered, goFmt)
		}
	}
}
