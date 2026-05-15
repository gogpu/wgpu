package browser

import "github.com/gogpu/gputypes"

// CompositeAlphaModeToJS converts a gputypes.CompositeAlphaMode to the WebGPU JS
// canvas alpha mode string.
//
// Browser WebGPU only supports "opaque" and "premultiplied". PostMultiplied and
// Inherit are not valid on the web (Rust wgpu panics on those). Auto and Opaque
// both map to "opaque".
//
// See: https://www.w3.org/TR/webgpu/#enumdef-gpucanvasalphamode
func CompositeAlphaModeToJS(mode gputypes.CompositeAlphaMode) string {
	switch mode {
	case gputypes.CompositeAlphaModePremultiplied:
		return "premultiplied"
	default:
		// Auto, Opaque, Unpremultiplied, Inherit all fall back to opaque.
		// Rust wgpu panics on PostMultiplied/Inherit; we gracefully default.
		return "opaque" //nolint:goconst // intentional literal in enum-to-string conversion
	}
}

// PresentModeToJS converts a gputypes.PresentMode to the WebGPU JS present mode string.
//
// Browser WebGPU does not expose present mode control; the browser always uses
// FIFO (VSync). Rust wgpu panics on Mailbox/Immediate on the web.
// We return "fifo" for all modes since the browser ignores it anyway.
func PresentModeToJS(mode gputypes.PresentMode) string {
	// Browser WebGPU only supports FIFO. The configure() call does not even
	// accept a presentMode field -- the browser auto-presents with VSync.
	_ = mode
	return "fifo" //nolint:goconst // intentional literal; browser only supports FIFO
}

// TextureFormatFromJS converts a WebGPU JS texture format string to the
// corresponding gputypes.TextureFormat. Returns TextureFormatUndefined if
// the string is not recognized.
//
// This is the reverse of TextureFormatToJS and is needed for parsing the
// preferred canvas format returned by navigator.gpu.getPreferredCanvasFormat().
func TextureFormatFromJS(s string) gputypes.TextureFormat {
	f, ok := textureFormatFromJSMap[s]
	if ok {
		return f
	}
	return gputypes.TextureFormatUndefined
}

// textureFormatFromJSMap is the reverse mapping of textureFormatMap.
// Built at init time from textureFormatMap.
var textureFormatFromJSMap map[string]gputypes.TextureFormat

func init() {
	textureFormatFromJSMap = make(map[string]gputypes.TextureFormat, len(textureFormatMap))
	for k, v := range textureFormatMap {
		textureFormatFromJSMap[v] = k
	}
}
