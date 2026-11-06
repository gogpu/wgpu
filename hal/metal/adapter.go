// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Adapter implements hal.Adapter for Metal.
type Adapter struct {
	instance              *Instance
	raw                   ID   // id<MTLDevice>
	formatDepth24Stencil8 bool // true if Depth24UnormStencil8 supported (Intel-era AMD only)
	// compileOptions drive shader compilation: the MSL version naga writes
	// and Metal compiles, read once from the OS version (P1-MTL-006).
	compileOptions shaderCompileOptions
}

// mapTextureFormat converts a WebGPU texture format to Metal pixel format,
// accounting for device capabilities (e.g. Depth24 support on Apple Silicon).
func (a *Adapter) mapTextureFormat(format gputypes.TextureFormat) MTLPixelFormat {
	switch format {
	case gputypes.TextureFormatDepth24Plus:
		if a.formatDepth24Stencil8 {
			return MTLPixelFormatDepth24UnormStencil8
		}
		return MTLPixelFormatDepth32Float
	case gputypes.TextureFormatDepth24PlusStencil8:
		if a.formatDepth24Stencil8 {
			return MTLPixelFormatDepth24UnormStencil8
		}
		return MTLPixelFormatDepth32FloatStencil8
	default:
		return textureFormatToMTL(format)
	}
}

// mapViewFormat returns the Metal pixel format of a view of a texture in
// textureFormat, as Rust wgpu-hal src/metal/adapter.rs map_view_format does.
// Metal samples the depth plane through a view in the combined pixel format,
// so a view of the stencil plane of a combined depth/stencil texture needs
// X24_Stencil8 (over Depth24UnormStencil8 storage) or X32_Stencil8. Any other
// view of a combined texture, its depth plane included, takes the texture's
// own pixel format.
//
// Rust wgpu-core hands the HAL the combined format for a view of a depth or
// stencil component (src/resource.rs create_texture_view). Our core passes the
// view format as given, so a view format that is the combined format or one
// of its components (Depth24Plus, Depth32Float, Stencil8) is resolved here,
// and Stencil8 selects the stencil plane as the StencilOnly aspect does.
func (a *Adapter) mapViewFormat(textureFormat, viewFormat gputypes.TextureFormat, aspect gputypes.TextureAspect) MTLPixelFormat {
	if viewFormat == gputypes.TextureFormatUndefined {
		viewFormat = textureFormat
	}
	if !isCombinedDepthStencilFormat(textureFormat) || !isDepthStencilComponent(viewFormat, textureFormat) {
		return a.mapTextureFormat(viewFormat)
	}
	stencil := aspect == gputypes.TextureAspectStencilOnly ||
		(viewFormat == gputypes.TextureFormatStencil8 && aspect != gputypes.TextureAspectDepthOnly)
	if !stencil {
		return a.mapTextureFormat(textureFormat)
	}
	if textureFormat == gputypes.TextureFormatDepth24PlusStencil8 && a.formatDepth24Stencil8 {
		return MTLPixelFormatX24Stencil8
	}
	return MTLPixelFormatX32Stencil8
}

// isDepthStencilComponent reports whether viewFormat is combinedFormat itself
// or one of its depth and stencil components. The component cases are Rust
// wgpu-types TextureFormat::is_depth_stencil_component.
func isDepthStencilComponent(viewFormat, combinedFormat gputypes.TextureFormat) bool {
	switch combinedFormat {
	case gputypes.TextureFormatDepth24PlusStencil8:
		return viewFormat == combinedFormat ||
			viewFormat == gputypes.TextureFormatDepth24Plus ||
			viewFormat == gputypes.TextureFormatStencil8
	case gputypes.TextureFormatDepth32FloatStencil8:
		return viewFormat == combinedFormat ||
			viewFormat == gputypes.TextureFormatDepth32Float ||
			viewFormat == gputypes.TextureFormatStencil8
	default:
		return false
	}
}

// Open opens a logical device with the requested features and limits.
func (a *Adapter) Open(features gputypes.Features, limits gputypes.Limits) (hal.OpenDevice, error) {
	device, err := newDevice(a)
	if err != nil {
		return hal.OpenDevice{}, err
	}

	queue := &Queue{
		device:       device,
		commandQueue: device.commandQueue,
	}

	// Initialize frame semaphore for CPU-ahead-of-GPU throttling.
	// Uses a buffered channel of size maxFramesInFlight pre-filled with tokens.
	// Each Submit() consumes a token; the GPU's addCompletedHandler: returns it.
	// If block support is unavailable, frameSemaphore stays nil (no throttling).
	if symNSConcreteGlobalBlock != 0 {
		queue.frameSemaphore = make(chan struct{}, maxFramesInFlight)
		for i := 0; i < maxFramesInFlight; i++ {
			queue.frameSemaphore <- struct{}{}
		}
	}

	// Back-reference so Device.WaitIdle can drain the frame semaphore.
	device.queue = queue

	hal.Logger().Debug("metal: adapter opened",
		"maxFramesInFlight", maxFramesInFlight,
		"blockSupport", symNSConcreteGlobalBlock != 0,
	)

	return hal.OpenDevice{
		Device: device,
		Queue:  queue,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a specific texture format.
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	flags := hal.TextureFormatCapabilitySampled

	// Most common formats support all operations on Metal
	switch format {
	case gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGBA32Float:
		flags |= hal.TextureFormatCapabilityStorage |
			hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve

	case gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth16Unorm:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityMultisample

	case gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32FloatStencil8:
		flags |= hal.TextureFormatCapabilityRenderAttachment
	}

	return hal.TextureFormatCapabilities{
		Flags: flags,
	}
}

// SurfaceCapabilities returns capabilities for a specific surface.
func (a *Adapter) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	if surface == nil {
		return nil
	}

	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatBGRA8UnormSrgb,
			gputypes.TextureFormatRGBA8Unorm,
			gputypes.TextureFormatRGBA8UnormSrgb,
			gputypes.TextureFormatRGBA16Float,
		},
		PresentModes: []gputypes.PresentMode{
			hal.PresentModeFifo,
			hal.PresentModeImmediate,
			hal.PresentModeMailbox,
		},
		AlphaModes: []gputypes.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
		},
	}
}

// Destroy releases the adapter.
func (a *Adapter) Destroy() {
	if a.raw != 0 {
		Release(a.raw)
		a.raw = 0
	}
}
