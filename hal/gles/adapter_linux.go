// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// Adapter implements hal.Adapter for OpenGL on Linux.
// Holds its *AdapterContext weakly (ADR-077): an adapter is often kept by an
// app without being released, and a strong reference would keep the context,
// and the EGLDisplay under it, alive past the native display the app closes.
// Open takes the Device's own reference and fails once the context is gone.
type Adapter struct {
	ctx           *AdapterContext
	displayHandle uintptr
	windowHandle  uintptr
	version       string
	renderer      string

	// caps holds the probed adapter capabilities (extensions, features,
	// limits, MSAA support). Populated by queryAdapterCapabilities during
	// adapter enumeration.
	caps AdapterCapabilities
}

// Open creates a logical device with the requested features and limits.
// The Device holds a reference to the adapter's context until Destroy, and
// the Queue borrows it. Once every surface and device of the context is gone,
// Open returns ErrContextReleased.
func (a *Adapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	// EnumerateAdapters(nil) path returns an adapter with nil ctx because no
	// EGL context can be created without a display/window handle. Return a
	// descriptive error instead of a nil pointer dereference at GenVertexArrays.
	if a.ctx == nil || a.ctx.GL() == nil {
		return hal.OpenDevice{}, fmt.Errorf("gles: adapter has no GL context — pass a surface hint to CreateSurface before RequestDevice")
	}
	if !a.ctx.Acquire() {
		return hal.OpenDevice{}, fmt.Errorf("%w: request an adapter with a live surface", ErrContextReleased)
	}
	open, err := a.open()
	if err != nil {
		a.ctx.Release()
		return hal.OpenDevice{}, err
	}
	return open, nil
}

// open builds the Device and Queue on a context the caller has acquired.
func (a *Adapter) open() (hal.OpenDevice, error) {
	glCtx, err := a.ctx.Lock()
	if err != nil {
		return hal.OpenDevice{}, err
	}
	defer a.ctx.Unlock()

	framebufferSRGB := enableFramebufferSRGB(glCtx, a.caps.IsES)
	useFirstProvokingVertex(glCtx, a.caps.IsES, a.caps.Extensions)

	// VAO is created lazily in CreateCommandEncoder — ensures it's allocated
	// on the window surface (after Configure), not on the pbuffer (during Open).
	vao := uint32(0)

	// Query hardware texture unit limit for binding validation.
	var maxTexUnits int32
	glCtx.GetIntegerv(gl.MAX_TEXTURE_IMAGE_UNITS, &maxTexUnits)
	if maxTexUnits <= 0 {
		maxTexUnits = 8 // Conservative default
	}

	vendor := glCtx.GetString(gl.VENDOR)

	hal.Logger().Info("gles: device opened",
		"vendor", vendor,
		"version", a.version,
		"renderer", a.renderer,
		"maxTextureUnits", maxTexUnits,
	)

	glslVer := GLSLVersionToNaga(a.caps.GLSLVersion, a.caps.IsES)

	device := &Device{
		ctx:                 a.ctx,
		displayHandle:       a.displayHandle,
		windowHandle:        a.windowHandle,
		vao:                 vao,
		maxTextureUnits:     maxTexUnits,
		maxMSAA:             a.caps.MaxMSAASamples,
		glslVersion:         glslVer,
		shaderBindingLayout: glslVer.SupportsExplicitLocations(),
		fullInstancing:      a.caps.FullyFeaturedInstancing,
		cubeArrays:          a.caps.DownlevelFlags.Contains(gputypes.DownlevelFlagsCubeArrayTextures),
		depthStencilTextureMode: glVersionAtLeast(a.caps.GLMajor, a.caps.GLMinor, a.caps.IsES,
			[2]int{3, 1}, [2]int{4, 3}),
		maxColorAttachments: a.caps.Limits.MaxColorAttachments,
		isES:                a.caps.IsES,
		immutableStorage:    a.caps.TextureStorage,
	}

	queue := &Queue{
		ctx:             a.ctx,
		fence:           NewFence(glCtx),
		framebufferSRGB: framebufferSRGB,
	}

	return hal.OpenDevice{
		Device: device,
		Queue:  queue,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a texture format.
// Uses probed GL extension and MSAA information for accurate per-format detection.
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return queryTextureFormatCapabilities(format, a.caps.Features, a.caps.MaxMSAASamples, a.caps.Extensions)
}

// SurfaceCapabilities returns surface capabilities, or nil when the adapter's
// context is gone: no Device can be opened to present with.
func (a *Adapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities {
	if a.ctx != nil && !a.ctx.Alive() {
		return nil
	}
	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
			gputypes.TextureFormatBGRA8UnormSrgb,
			gputypes.TextureFormatRGBA8UnormSrgb,
		},
		PresentModes: []gputypes.PresentMode{
			hal.PresentModeFifo,      // VSync on
			hal.PresentModeImmediate, // VSync off (if supported)
		},
		AlphaModes: []gputypes.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
		},
	}
}

// QualifySurface returns a request-local adapter for surface, bound to the
// context the surface uses. Core calls it for the cached adapter when a new
// surface asks for an adapter (core/instance.go surfaceAdapterQualifier), as
// for Vulkan. GLES enumerates once per instance, with the first surface; a
// window closed and reopened in the same Instance gets a new context, and
// this gives it an adapter that opens on it. The returned adapter holds the
// context weakly, like every GLES adapter.
func (a *Adapter) QualifySurface(surface hal.Surface) (hal.Adapter, error) {
	s, ok := surface.(*Surface)
	if !ok || s == nil {
		return nil, fmt.Errorf("gles: %w: not a GLES surface", hal.ErrUnsupportedSurfaceTarget)
	}
	if s.ctx == nil {
		return nil, fmt.Errorf("gles: surface has no AdapterContext")
	}
	if !s.ctx.Alive() {
		return nil, ErrContextReleased
	}
	exposed, ok := s.GetAdapterInfo()
	if !ok || exposed.Adapter == nil {
		return nil, fmt.Errorf("gles: the surface's context is below OpenGL 3.3 / ES 3.0 or released")
	}
	return exposed.Adapter, nil
}

// Destroy releases the adapter. It holds no reference to its context.
func (a *Adapter) Destroy() {}
