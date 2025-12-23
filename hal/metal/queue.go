// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"unsafe"

	"github.com/gogpu/wgpu/hal"
)

// Queue implements hal.Queue for Metal.
type Queue struct {
	device       *Device
	commandQueue ID // id<MTLCommandQueue>
}

// Submit submits command buffers to the GPU.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	for _, buf := range commandBuffers {
		cb, ok := buf.(*CommandBuffer)
		if !ok || cb == nil {
			continue
		}

		// If fence provided, signal it on completion
		if fence != nil {
			if mtlFence, ok := fence.(*Fence); ok && mtlFence != nil {
				// Metal uses MTLEvent for synchronization
				// encodeSignalEvent:value: on command buffer
				_ = MsgSend(cb.raw, Sel("encodeSignalEvent:value:"),
					uintptr(mtlFence.event), uintptr(fenceValue))
				mtlFence.value = fenceValue
			}
		}

		// Commit the command buffer
		_ = MsgSend(cb.raw, Sel("commit"))
	}
	return nil
}

// WriteBuffer writes data to a buffer immediately.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}

	ptr := buf.Contents()
	if ptr == 0 {
		return // Buffer is not mappable
	}

	// Copy data using unsafe
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr+uintptr(offset))), len(data))
	copy(dst, data)
}

// WriteTexture writes data to a texture immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	// This requires a staging buffer and blit encoder
	// For now, not implemented
}

// Present presents a surface texture to the screen.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	st, ok := texture.(*SurfaceTexture)
	if !ok || st == nil {
		return nil
	}

	if st.drawable != 0 {
		// Present the drawable
		_ = MsgSend(st.drawable, Sel("present"))
		Release(st.drawable)
		st.drawable = 0
	}

	return nil
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// Metal timestamps are in nanoseconds
	return 1.0
}
