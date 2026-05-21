// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"fmt"
	"image"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// Queue implements hal.Queue for OpenGL.
// Holds a shared *AdapterContext (owned by Instance).
type Queue struct {
	ctx             *AdapterContext
	submissionIndex uint64
	fence           *Fence // signaled at each submit for GPU completion tracking
}

// Submit submits command buffers to the GPU.
// Acquires the AdapterContext lock, makes context current on hidden window DC,
// executes all GL commands, flushes, and signals the fence.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	glCtx := q.ctx.Lock()
	defer q.ctx.Unlock()

	for _, cb := range commandBuffers {
		cmdBuf, ok := cb.(*CommandBuffer)
		if !ok {
			return 0, fmt.Errorf("gles: invalid command buffer type")
		}

		for i, cmd := range cmdBuf.commands {
			cmd.Execute(glCtx)
			if glErr := glCtx.GetError(); glErr != 0 {
				hal.Logger().Warn("gles: GL error after command", "error", fmt.Sprintf("0x%x", glErr), "index", i, "command", fmt.Sprintf("%T", cmd))
			}
		}
	}

	glCtx.Flush()

	q.submissionIndex++

	if q.fence != nil {
		q.fence.Signal(q.submissionIndex)
	}

	return q.submissionIndex, nil
}

// PollCompleted returns the highest submission index known to be completed.
// When GL fence sync is available, polls pending sync objects. Otherwise,
// assumes all work is complete after Flush (GLES is synchronous).
func (q *Queue) PollCompleted() uint64 {
	if q.fence != nil {
		q.fence.Maintain()
		return q.fence.GetLatest()
	}
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

	glCtx := q.ctx.Lock()
	defer q.ctx.Unlock()

	glCtx.BindBuffer(buf.target, buf.id)
	glCtx.BufferSubData(buf.target, int(offset), len(data), unsafe.Pointer(&data[0]))
	glCtx.BindBuffer(buf.target, 0)
	return nil
}

// WriteTexture writes data to a texture immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	tex, ok := dst.Texture.(*Texture)
	if !ok {
		return fmt.Errorf("gles: invalid texture type for WriteTexture")
	}

	glCtx := q.ctx.Lock()
	defer q.ctx.Unlock()

	_, format, dataType := textureFormatToGL(tex.format)

	glCtx.BindTexture(tex.target, tex.id)

	if tex.target == gl.TEXTURE_2D {
		if tex.format == gputypes.TextureFormatR8Unorm {
			glCtx.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
		}
		glCtx.TexSubImage2D(tex.target, int32(dst.MipLevel),
			0, 0, int32(size.Width), int32(size.Height), format, dataType,
			unsafe.Pointer(&data[0]))
		if tex.format == gputypes.TextureFormatR8Unorm {
			glCtx.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
		}
	}

	glCtx.BindTexture(tex.target, 0)

	hal.Logger().Debug("gles: texture written",
		"format", tex.format,
		"width", size.Width,
		"height", size.Height,
	)

	return nil
}

// Present presents a surface texture to the screen.
//
// Makes the GL context current on the user window's DC (via LockForDC),
// blits the swapchain FBO to the default framebuffer with Y-flip, then
// SwapBuffers. Mirrors Rust wgpu-hal wgl.rs Surface::present (682-750):
// GetDC → lock_with_dc → blit → SwapBuffers → ReleaseDC.
//
// damageRects is accepted but ignored on Windows WGL — WGL has no
// damage-aware swap API.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture, _ []image.Rectangle) error {
	surf, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("gles: invalid surface type")
	}

	// Get fresh DC for the user window (Rust: Gdi::GetDC(self.window)).
	hdc := wgl.GetDC(surf.hwnd)
	if hdc == 0 {
		return fmt.Errorf("gles: GetDC failed for hwnd 0x%x", surf.hwnd)
	}
	defer wgl.ReleaseDC(surf.hwnd, hdc)

	// MakeCurrent to user window DC for presentation.
	glCtx := q.ctx.LockForDC(hdc)
	defer q.ctx.Unlock()

	surf.blitSwapchainToDefaultWith(glCtx)

	return wgl.SwapBuffers(hdc)
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// OpenGL doesn't have a standard way to query this
	// Return 1.0 to indicate nanoseconds
	return 1.0
}

// SupportsCommandBufferCopies returns false for GLES.
// GLES uses direct GL calls (glBufferSubData, glTexSubImage2D) for writes,
// not command buffer copy operations.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return false
}

// SetSwapchainSuppressed is a no-op on GLES.
// GLES uses eglSwapBuffers for presentation, which is not affected by command
// submission ordering. See BUG-WGPU-VK-005 (Vulkan-specific).
func (q *Queue) SetSwapchainSuppressed(_ bool) {}
