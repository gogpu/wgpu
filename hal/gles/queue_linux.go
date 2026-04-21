// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// Queue implements hal.Queue for OpenGL on Linux.
type Queue struct {
	glCtx           *gl.Context
	eglCtx          *egl.Context
	submissionIndex uint64
}

// Submit submits command buffers to the GPU.
// GLES is synchronous, so the submission is effectively complete immediately after Flush.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	for _, cb := range commandBuffers {
		cmdBuf, ok := cb.(*CommandBuffer)
		if !ok {
			return 0, fmt.Errorf("gles: invalid command buffer type")
		}

		// Execute recorded commands with GL error checking.
		for i, cmd := range cmdBuf.commands {
			cmd.Execute(q.glCtx)
			if glErr := q.glCtx.GetError(); glErr != 0 {
				hal.Logger().Warn("gles: GL error after command", "error", fmt.Sprintf("0x%x", glErr), "index", i, "command", fmt.Sprintf("%T", cmd))
			}
		}
	}

	// Flush GL commands
	q.glCtx.Flush()

	q.submissionIndex++
	return q.submissionIndex, nil
}

// PollCompleted returns the highest submission index known to be completed.
// GLES is synchronous — after Flush, all submitted work is complete.
func (q *Queue) PollCompleted() uint64 {
	return q.submissionIndex
}

// WriteBuffer writes data to a buffer immediately.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return fmt.Errorf("gles: WriteBuffer: invalid buffer type")
	}
	if len(data) == 0 {
		return nil
	}

	q.glCtx.BindBuffer(buf.target, buf.id)
	q.glCtx.BufferSubData(buf.target, int(offset), len(data), uintptr(unsafe.Pointer(&data[0])))
	q.glCtx.BindBuffer(buf.target, 0)
	return nil
}

// WriteTexture writes data to a texture immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	tex, ok := dst.Texture.(*Texture)
	if !ok {
		return fmt.Errorf("gles: invalid texture type for WriteTexture")
	}

	_, format, dataType := textureFormatToGL(tex.format)

	q.glCtx.BindTexture(tex.target, tex.id)

	if tex.target == gl.TEXTURE_2D {
		// Set alignment to 1 for single-channel formats (R8) whose row stride
		// may not be a multiple of the default 4-byte GL_UNPACK_ALIGNMENT.
		if tex.format == gputypes.TextureFormatR8Unorm {
			q.glCtx.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
		}
		// Use TexSubImage2D to update existing texture data (Rust wgpu-hal pattern).
		// TexImage2D reallocates storage on every call; TexSubImage2D updates in-place.
		q.glCtx.TexSubImage2D(tex.target, int32(dst.MipLevel),
			0, 0, int32(size.Width), int32(size.Height), format, dataType,
			uintptr(unsafe.Pointer(&data[0])))
		// Restore default alignment after upload.
		if tex.format == gputypes.TextureFormatR8Unorm {
			q.glCtx.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
		}
	}

	q.glCtx.BindTexture(tex.target, 0)

	hal.Logger().Debug("gles: texture written",
		"format", tex.format,
		"width", size.Width,
		"height", size.Height,
	)

	return nil
}

// Present presents a surface texture to the screen.
//
// Before SwapBuffers, blits the Surface's swapchain offscreen FBO to the
// default framebuffer (FBO 0) with an explicit Y-flip. User render passes
// render upside-down into the swapchain FBO (driven by naga's in-shader
// Y-flip); the blit un-flips for presentation. Mirrors Rust wgpu-hal
// src/gles/egl.rs Surface::present (1280-1308).
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture) error {
	surf, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("gles: invalid surface type")
	}

	surf.blitSwapchainToDefault()

	// Use EGL SwapBuffers to present the rendered content
	result := egl.SwapBuffers(surf.eglDisplay, surf.eglSurface)
	if result == egl.False {
		return fmt.Errorf("gles: eglSwapBuffers failed: error 0x%x", egl.GetError())
	}

	return nil
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// OpenGL doesn't have a standard way to query this
	// Return 1.0 to indicate nanoseconds
	return 1.0
}

// SupportsCommandBufferCopies returns false for GLES on Linux.
// GLES uses direct GL calls for writes, not command buffer copy operations.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return false
}
