// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
)

// Adapter implements hal.Adapter for Metal.
type Adapter struct {
	instance *Instance
	raw      ID // id<MTLDevice>
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
		PresentModes: []hal.PresentMode{
			hal.PresentModeFifo,
			hal.PresentModeImmediate,
			hal.PresentModeMailbox,
		},
		AlphaModes: []hal.CompositeAlphaMode{
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
