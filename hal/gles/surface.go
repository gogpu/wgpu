// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// allocateSwapchainFBO creates a persistent swapchain framebuffer.
// Must be called with the GL context current (caller holds AdapterContext lock).
func allocateSwapchainFBO(glCtx *gl.Context, format gputypes.TextureFormat, width, height uint32) (fbo, colorRbo uint32, err error) {
	if glCtx == nil {
		return 0, 0, fmt.Errorf("gles: allocateSwapchainFBO: nil gl context")
	}
	if width == 0 || height == 0 {
		return 0, 0, hal.ErrZeroArea
	}

	glFormat, err := describeTextureFormat(format)
	if err != nil {
		return 0, 0, err
	}
	internalFormat := glFormat.internal

	colorRbo = glCtx.GenRenderbuffers(1)
	if colorRbo == 0 {
		return 0, 0, fmt.Errorf("gles: glGenRenderbuffers returned 0")
	}
	glCtx.BindRenderbuffer(gl.RENDERBUFFER, colorRbo)
	glCtx.RenderbufferStorage(gl.RENDERBUFFER, internalFormat, int32(width), int32(height))

	fbo = glCtx.GenFramebuffers(1)
	if fbo == 0 {
		glCtx.BindRenderbuffer(gl.RENDERBUFFER, 0)
		glCtx.DeleteRenderbuffers(colorRbo)
		return 0, 0, fmt.Errorf("gles: glGenFramebuffers returned 0")
	}
	glCtx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	glCtx.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, colorRbo)

	status := glCtx.CheckFramebufferStatus(gl.FRAMEBUFFER)

	glCtx.BindFramebuffer(gl.FRAMEBUFFER, 0)
	glCtx.BindRenderbuffer(gl.RENDERBUFFER, 0)

	if status != gl.FRAMEBUFFER_COMPLETE {
		glCtx.DeleteFramebuffers(fbo)
		glCtx.DeleteRenderbuffers(colorRbo)
		return 0, 0, fmt.Errorf("gles: swapchain framebuffer incomplete (status 0x%x)", status)
	}

	hal.Logger().Debug("gles: allocated swapchain FBO",
		"fbo", fbo,
		"colorRbo", colorRbo,
		"width", width,
		"height", height,
		"internalFormat", fmt.Sprintf("0x%x", internalFormat),
	)
	return fbo, colorRbo, nil
}

// destroySwapchainFBO releases the swapchain framebuffer and its attachments.
// Safe to call with zero handles or nil context.
func destroySwapchainFBO(glCtx *gl.Context, fbo, colorRbo uint32) {
	if glCtx == nil {
		return
	}
	if fbo != 0 {
		glCtx.DeleteFramebuffers(fbo)
	}
	if colorRbo != 0 {
		glCtx.DeleteRenderbuffers(colorRbo)
	}
}

// enableFramebufferSRGB turns on GL_FRAMEBUFFER_SRGB on a desktop GL context
// and reports whether it did. Desktop GL encodes writes and blends to sRGB
// attachments only with it enabled; OpenGL ES always encodes and has no such
// capability. Rust wgpu-hal enables it once per context and turns it off only
// around the present blit (wgl.rs, egl.rs), but only when the window system
// offers sRGB surfaces (WGL_EXT_framebuffer_sRGB, EGL 1.5 or
// EGL_KHR_gl_colorspace). Framebuffer-object sRGB is core since GL 3.0, so we
// enable it on every desktop context: without it an offscreen sRGB target
// stores the linear value and blends in sRGB space.
func enableFramebufferSRGB(glCtx *gl.Context, isES bool) bool {
	if isES {
		return false
	}
	glCtx.Enable(gl.FRAMEBUFFER_SRGB)
	return true
}

// blitSwapchainToDefaultWith performs the present-time Y-flipping blit from
// the Surface's swapchain FBO to the default framebuffer (FBO 0).
// Must be called with GL context current on the user window DC.
//
// framebufferSRGB says the context has GL_FRAMEBUFFER_SRGB enabled: it is
// turned off for the blit, so the stored bytes reach the window unconverted
// whatever the formats, and on again after.
//
// Mirrors Rust wgpu-hal/src/gles/egl.rs Surface::present (1280-1308).
func (s *Surface) blitSwapchainToDefaultWith(glCtx *gl.Context, framebufferSRGB bool) {
	if glCtx == nil || s.swapchainFBO == 0 {
		return
	}
	if s.fboWidth == 0 || s.fboHeight == 0 {
		return
	}

	glCtx.Disable(gl.SCISSOR_TEST)
	if framebufferSRGB {
		glCtx.Disable(gl.FRAMEBUFFER_SRGB)
		defer glCtx.Enable(gl.FRAMEBUFFER_SRGB)
	}

	glCtx.BindFramebuffer(gl.READ_FRAMEBUFFER, s.swapchainFBO)
	glCtx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)

	w := int32(s.fboWidth)
	h := int32(s.fboHeight)

	glCtx.BlitFramebuffer(
		0, h, w, 0, // source Y-flipped
		0, 0, w, h, // dest normal
		gl.COLOR_BUFFER_BIT, gl.NEAREST,
	)

	glCtx.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	glCtx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)
}

// reconfigureSwapchainFBOWith destroys the existing swapchain FBO and
// allocates a new one. Caller must hold the AdapterContext lock.
func (s *Surface) reconfigureSwapchainFBOWith(glCtx *gl.Context, format gputypes.TextureFormat, width, height uint32) error {
	destroySwapchainFBO(glCtx, s.swapchainFBO, s.colorRenderbuffer)
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0
	s.fboWidth = 0
	s.fboHeight = 0

	fbo, colorRbo, err := allocateSwapchainFBO(glCtx, format, width, height)
	if err != nil {
		return err
	}
	s.swapchainFBO = fbo
	s.colorRenderbuffer = colorRbo
	s.fboWidth = width
	s.fboHeight = height
	return nil
}
