//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/gogpu/gputypes"
)

// BuildSurfaceConfiguration constructs a JS GPUCanvasConfiguration object.
//
// The returned object is passed to GPUCanvasContext.configure(). Fields match
// the WebGPU spec GPUCanvasConfiguration dictionary:
//   - device: GPUDevice
//   - format: GPUTextureFormat string
//   - usage: GPUTextureUsageFlags (default: RENDER_ATTACHMENT)
//   - alphaMode: GPUCanvasAlphaMode ("opaque" or "premultiplied")
//   - viewFormats: sequence<GPUTextureFormat>
//
// Note: the spec does not include width/height/presentMode in the configure()
// call. Canvas dimensions are set separately via canvas.width/canvas.height.
//
// Matches Rust wgpu SurfaceInterface::configure for WebSurface which builds
// GpuCanvasConfiguration with device, format, usage, alpha_mode, view_formats.
func BuildSurfaceConfiguration(
	deviceRef js.Value,
	format gputypes.TextureFormat,
	usage gputypes.TextureUsage,
	alphaMode gputypes.CompositeAlphaMode,
	viewFormats []gputypes.TextureFormat,
) js.Value {
	config := newJSObject()
	config.Set("device", deviceRef)
	config.Set("format", TextureFormatToJS(format))

	// Usage defaults to RENDER_ATTACHMENT in the spec, but we set it explicitly
	// for clarity (Rust wgpu also sets it explicitly).
	config.Set("usage", float64(usage))

	config.Set("alphaMode", CompositeAlphaModeToJS(alphaMode))

	if len(viewFormats) > 0 {
		arr := newJSArray()
		for _, vf := range viewFormats {
			arr.Call("push", TextureFormatToJS(vf))
		}
		config.Set("viewFormats", arr)
	}

	return config
}
