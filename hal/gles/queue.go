// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// Queue implements hal.Queue for OpenGL.
type Queue struct {
	glCtx  *gl.Context
	wglCtx *wgl.Context
}

// Submit submits command buffers to the GPU.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	for _, cb := range commandBuffers {
		cmdBuf, ok := cb.(*CommandBuffer)
		if !ok {
			return fmt.Errorf("gles: invalid command buffer type")
		}

		// Execute recorded commands
		for _, cmd := range cmdBuf.commands {
			cmd.Execute(q.glCtx)
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

// WriteBuffer writes data to a buffer immediately.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}

	// Determine target from usage
	target := uint32(gl.ARRAY_BUFFER)

	q.glCtx.BindBuffer(target, buf.id)
	q.glCtx.BufferSubData(target, int(offset), len(data), unsafe.Pointer(&data[0]))
	q.glCtx.BindBuffer(target, 0)
}

// WriteTexture writes data to a texture immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	tex, ok := dst.Texture.(*Texture)
	if !ok {
		return
	}

	_, format, dataType := textureFormatToGL(tex.format)

	q.glCtx.BindTexture(tex.target, tex.id)

	if tex.target == gl.TEXTURE_2D {
		q.glCtx.TexImage2D(tex.target, int32(dst.MipLevel), int32(tex.target),
			int32(size.Width), int32(size.Height), 0, format, dataType,
			unsafe.Pointer(&data[0]))
	}

	q.glCtx.BindTexture(tex.target, 0)
}

// Present presents a surface texture to the screen.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture) error {
	surf, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("gles: invalid surface type")
	}

	return surf.wglCtx.SwapBuffers()
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// OpenGL doesn't have a standard way to query this
	// Return 1.0 to indicate nanoseconds
	return 1.0
}
