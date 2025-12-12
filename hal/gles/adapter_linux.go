// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/types"
)

// Adapter implements hal.Adapter for OpenGL on Linux.
type Adapter struct {
	glCtx         *gl.Context
	eglCtx        *egl.Context
	displayHandle uintptr
	windowHandle  uintptr
	version       string
	renderer      string
}

// Open creates a logical device with the requested features and limits.
func (a *Adapter) Open(_ types.Features, _ types.Limits) (hal.OpenDevice, error) {
	// Make context current if we have one
	if a.eglCtx != nil {
		if err := a.eglCtx.MakeCurrent(); err != nil {
			return hal.OpenDevice{}, err
		}
	}

	device := &Device{
		glCtx:         a.glCtx,
		eglCtx:        a.eglCtx,
		displayHandle: a.displayHandle,
		windowHandle:  a.windowHandle,
	}

	queue := &Queue{
		glCtx:  a.glCtx,
		eglCtx: a.eglCtx,
	}

	return hal.OpenDevice{
		Device: device,
		Queue:  queue,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a texture format.
func (a *Adapter) TextureFormatCapabilities(format types.TextureFormat) hal.TextureFormatCapabilities {
	// OpenGL 3.3+ supports most common formats
	// TODO: Query actual format support from GL
	flags := hal.TextureFormatCapabilitySampled

	switch format {
	case types.TextureFormatRGBA8Unorm,
		types.TextureFormatRGBA8UnormSrgb,
		types.TextureFormatBGRA8Unorm,
		types.TextureFormatBGRA8UnormSrgb,
		types.TextureFormatRGBA16Float,
		types.TextureFormatRGBA32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve

	case types.TextureFormatR8Unorm,
		types.TextureFormatRG8Unorm,
		types.TextureFormatR16Float,
		types.TextureFormatRG16Float,
		types.TextureFormatR32Float,
		types.TextureFormatRG32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable

	case types.TextureFormatDepth16Unorm,
		types.TextureFormatDepth24Plus,
		types.TextureFormatDepth24PlusStencil8,
		types.TextureFormatDepth32Float,
		types.TextureFormatDepth32FloatStencil8:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityMultisample
	}

	return hal.TextureFormatCapabilities{
		Flags: flags,
	}
}

// SurfaceCapabilities returns surface capabilities.
func (a *Adapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities {
	return &hal.SurfaceCapabilities{
		Formats: []types.TextureFormat{
			types.TextureFormatBGRA8Unorm,
			types.TextureFormatRGBA8Unorm,
			types.TextureFormatBGRA8UnormSrgb,
			types.TextureFormatRGBA8UnormSrgb,
		},
		PresentModes: []hal.PresentMode{
			hal.PresentModeFifo,      // VSync on
			hal.PresentModeImmediate, // VSync off (if supported)
		},
		AlphaModes: []hal.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
		},
	}
}

// Destroy releases the adapter.
func (a *Adapter) Destroy() {
	// Adapter doesn't own the GL context
}
