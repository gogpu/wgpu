// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// Queue implements hal.Queue for OpenGL on Linux.
type Queue struct {
	glCtx  *gl.Context
	eglCtx *egl.Context
}

// Submit submits command buffers to the GPU.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	for _, cb := range commandBuffers {
		cmdBuf, ok := cb.(*CommandBuffer)
		if !ok {
			return fmt.Errorf("gles: invalid command buffer type")
		}

		// Execute recorded commands with GL error checking.
		for i, cmd := range cmdBuf.commands {
			cmd.Execute(q.glCtx)
			if glErr := q.glCtx.GetError(); glErr != 0 {
				hal.Logger().Warn("gles: GL error after command", "error", fmt.Sprintf("0x%x", glErr), "index", i, "command", fmt.Sprintf("%T", cmd))
			}
		}
	}

	// Signal fence if provided
	if fence != nil {
		if f, ok := fence.(*Fence); ok {
			f.Signal(fenceValue)
		}
	}

	// Flush GL commands
	q.glCtx.Flush()

	return nil
}

// ReadBuffer reads data from a GPU buffer into the provided byte slice.
// If the buffer has CPU-side data (populated by CopyTextureToBuffer), it reads
// from there directly. Otherwise it falls back to glMapBuffer with GL_READ_ONLY.
func (q *Queue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	buf, ok := buffer.(*Buffer)
	if !ok {
		return fmt.Errorf("gles: invalid buffer for ReadBuffer")
	}

	// If the buffer has CPU-side data (from CopyTextureToBuffer), read from it.
	if len(buf.data) > 0 {
		end := offset + uint64(len(data))
		if end > uint64(len(buf.data)) {
			return fmt.Errorf("gles: ReadBuffer offset+size (%d) exceeds buffer data length (%d)", end, len(buf.data))
		}
		copy(data, buf.data[offset:end])
		return nil
	}

	// Fall back to GL buffer mapping for GPU-resident buffers.
	if buf.id == 0 {
		return fmt.Errorf("gles: invalid buffer for ReadBuffer (no GL ID and no CPU data)")
	}

	// Bind buffer to COPY_READ_BUFFER target (avoids disturbing other bindings)
	q.glCtx.BindBuffer(gl.COPY_READ_BUFFER, buf.id)

	// Map the buffer for reading
	ptr := q.glCtx.MapBuffer(gl.COPY_READ_BUFFER, gl.READ_ONLY)
	if ptr == 0 {
		q.glCtx.BindBuffer(gl.COPY_READ_BUFFER, 0)
		return fmt.Errorf("gles: glMapBuffer returned null (function may not be available)")
	}

	// Copy from the mapped pointer at the given offset into data.
	//nolint:govet // FFI returns uintptr that must be converted to pointer for memory access
	src := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), uint64(len(data))+offset)
	copy(data, src[offset:])

	// Unmap and unbind
	q.glCtx.UnmapBuffer(gl.COPY_READ_BUFFER)
	q.glCtx.BindBuffer(gl.COPY_READ_BUFFER, 0)

	return nil
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
		q.glCtx.TexImage2D(tex.target, int32(dst.MipLevel), int32(tex.target),
			int32(size.Width), int32(size.Height), 0, format, dataType,
			uintptr(unsafe.Pointer(&data[0])))
	}

	q.glCtx.BindTexture(tex.target, 0)
	return nil
}

// Present presents a surface texture to the screen.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture) error {
	surf, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("gles: invalid surface type")
	}

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
