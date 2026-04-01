// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// Adapter implements hal.Adapter for OpenGL.
type Adapter struct {
	glCtx    *gl.Context
	wglCtx   *wgl.Context
	hwnd     wgl.HWND
	version  string
	renderer string
}

// Open creates a logical device with the requested features and limits.
func (a *Adapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	// OpenGL requires a current context to do anything. If the adapter was
	// created without a surface (placeholder from EnumerateAdapters(nil)),
	// glCtx is nil and we cannot proceed.
	if a.glCtx == nil {
		return hal.OpenDevice{}, fmt.Errorf("gles: GL context not initialized — create a surface first")
	}

	// Make context current if we have one
	if a.wglCtx != nil {
		if err := a.wglCtx.MakeCurrent(); err != nil {
			return hal.OpenDevice{}, err
		}
	}

	// Create and bind a persistent VAO. OpenGL Core Profile requires a VAO
	// to be bound for any draw call. We keep one bound for the device lifetime.
	vao := a.glCtx.GenVertexArrays(1)
	a.glCtx.BindVertexArray(vao)

	// Query hardware texture unit limit for binding validation.
	var maxTexUnits int32
	a.glCtx.GetIntegerv(gl.MAX_TEXTURE_IMAGE_UNITS, &maxTexUnits)
	if maxTexUnits <= 0 {
		maxTexUnits = 8 // Conservative default
	}

	vendor := a.glCtx.GetString(gl.VENDOR)

	hal.Logger().Info("gles: device opened",
		"vendor", vendor,
		"version", a.version,
		"renderer", a.renderer,
		"maxTextureUnits", maxTexUnits,
	)

	device := &Device{
		glCtx:           a.glCtx,
		wglCtx:          a.wglCtx,
		hwnd:            a.hwnd,
		vao:             vao,
		maxTextureUnits: maxTexUnits,
	}

	queue := &Queue{
		glCtx:  a.glCtx,
		wglCtx: a.wglCtx,
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
