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

	internalFormat, _, _ := textureFormatToGL(format)

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

// blitSwapchainToDefaultWith performs the present-time Y-flipping blit from
// the Surface's swapchain FBO to the default framebuffer (FBO 0).
// Must be called with GL context current on the user window DC.
//
// Mirrors Rust wgpu-hal/src/gles/egl.rs Surface::present (1280-1308).
func (s *Surface) blitSwapchainToDefaultWith(glCtx *gl.Context) {
	if glCtx == nil || s.swapchainFBO == 0 {
		return
	}
	if s.fboWidth == 0 || s.fboHeight == 0 {
		return
	}

	glCtx.Disable(gl.SCISSOR_TEST)

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
