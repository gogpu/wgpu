// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
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
func (a *Adapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	// Make context current if we have one
	if a.eglCtx != nil {
		if err := a.eglCtx.MakeCurrent(); err != nil {
			return hal.OpenDevice{}, err
		}
	}

	// Create and bind a persistent VAO. OpenGL Core Profile requires a VAO
	// to be bound for any draw call. We keep one bound for the device lifetime.
	vao := a.glCtx.GenVertexArrays(1)
	a.glCtx.BindVertexArray(vao)

	device := &Device{
		glCtx:         a.glCtx,
		eglCtx:        a.eglCtx,
		displayHandle: a.displayHandle,
		windowHandle:  a.windowHandle,
		vao:           vao,
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
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	// OpenGL 3.3+ supports most common formats
	// Note: Full format support querying requires glGetInternalformativ (GL 4.2+).
	flags := hal.TextureFormatCapabilitySampled

	switch format {
	case gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGBA32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve

	case gputypes.TextureFormatR8Unorm,
		gputypes.TextureFormatRG8Unorm,
		gputypes.TextureFormatR16Float,
		gputypes.TextureFormatRG16Float,
		gputypes.TextureFormatR32Float,
		gputypes.TextureFormatRG32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable

	case gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8:
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
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
			gputypes.TextureFormatBGRA8UnormSrgb,
			gputypes.TextureFormatRGBA8UnormSrgb,
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
