// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// allocateSwapchainFBO creates a persistent swapchain framebuffer for the
// Surface. User render passes that target this Surface render into this FBO
// (backed by a color renderbuffer), not the default framebuffer (FBO 0).
// Queue.Present performs an explicit Y-flipping glBlitFramebuffer from this
// FBO to FBO 0 before SwapBuffers.
//
// This closes an architectural gap vs Rust wgpu-hal/src/gles/egl.rs:
//   - Surface::configure (1537-1562) allocates swapchain renderbuffer + FBO.
//   - Surface::present    (1280-1308) blits with srcY0=height, srcY1=0.
//
// naga's in-shader Y-flip (`gl_Position.y = -gl_Position.y`, driven by
// glsl.WriterFlagAdjustCoordinateSpace) intentionally renders the scene
// upside-down inside this FBO; the present-time blit un-flips it. Together
// this yields a WebGPU-compliant top-left framebuffer origin for users.
//
// Must be called on the thread where the GL context is current.
// width/height must both be non-zero (caller should validate).
// Returns the new FBO and color renderbuffer names, or an error if allocation
// fails or the framebuffer is not complete.
func allocateSwapchainFBO(glCtx *gl.Context, format gputypes.TextureFormat, width, height uint32) (fbo, colorRbo uint32, err error) {
	if glCtx == nil {
		return 0, 0, fmt.Errorf("gles: allocateSwapchainFBO: nil gl context")
	}
	if width == 0 || height == 0 {
		return 0, 0, hal.ErrZeroArea
	}

	// Derive a sized internal format from the surface's texture format.
	// For typical sRGB / BGRA / RGBA surface formats this is GL_RGBA8 or
	// GL_SRGB8_ALPHA8. If the format is unsupported, textureFormatToGL
	// falls back to GL_RGBA8.
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

	// Restore default bindings before returning, regardless of success.
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
// Safe to call with zero handles (no-op). Must be called on the thread where
// the GL context is current.
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

// blitSwapchainToDefault performs the present-time Y-flipping blit from the
// Surface's swapchain offscreen FBO to the default framebuffer (FBO 0). Must
// be called with the GL context current, immediately before SwapBuffers.
//
// The source rect is Y-inverted (srcY0=height, srcY1=0); the destination rect
// is normal. Combined with naga's in-shader Y-flip this yields a top-left
// framebuffer origin on screen, matching WebGPU semantics.
//
// This is a no-op when there is no swapchain FBO (e.g. surface not yet
// configured, or failed allocation) — in that case whatever the user rendered
// directly into FBO 0 is presented as-is.
//
// Mirrors Rust wgpu-hal/src/gles/egl.rs Surface::present (1280-1308).
func (s *Surface) blitSwapchainToDefault() {
	if s.glCtx == nil || s.swapchainFBO == 0 {
		return
	}
	if s.fboWidth == 0 || s.fboHeight == 0 {
		return
	}

	ctx := s.glCtx

	// glBlitFramebuffer respects GL_SCISSOR_TEST on the draw framebuffer.
	// Disable it so the entire surface is copied (gg#226).
	ctx.Disable(gl.SCISSOR_TEST)

	ctx.BindFramebuffer(gl.READ_FRAMEBUFFER, s.swapchainFBO)
	ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)

	w := int32(s.fboWidth)
	h := int32(s.fboHeight)

	// Y-flip source rect: srcY0=h, srcY1=0.
	// GL's presentation is bottom-left origin; main rendering is intentionally
	// upside-down via naga's in-shader Y-flip. Un-flip here for presentation.
	ctx.BlitFramebuffer(
		0, h, w, 0, // source Y-flipped: top → bottom
		0, 0, w, h, // dest: normal
		gl.COLOR_BUFFER_BIT, gl.NEAREST,
	)

	// Restore default bindings.
	ctx.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)
}

// reconfigureSwapchainFBO destroys the existing swapchain FBO (if any) and
// allocates a new one at the new dimensions. On error, the Surface is left
// with all swapchain fields zeroed so subsequent Configure calls can retry.
func (s *Surface) reconfigureSwapchainFBO(format gputypes.TextureFormat, width, height uint32) error {
	// Destroy previous allocation (if any) before allocating new one.
	// Ensures no leak on resize.
	destroySwapchainFBO(s.glCtx, s.swapchainFBO, s.colorRenderbuffer)
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0
	s.fboWidth = 0
	s.fboHeight = 0

	fbo, colorRbo, err := allocateSwapchainFBO(s.glCtx, format, width, height)
	if err != nil {
		return err
	}
	s.swapchainFBO = fbo
	s.colorRenderbuffer = colorRbo
	s.fboWidth = width
	s.fboHeight = height
	return nil
}
