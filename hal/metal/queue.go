// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
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
	pool := NewAutoreleasePool()
	defer pool.Drain()

	for _, buf := range commandBuffers {
		cb, ok := buf.(*CommandBuffer)
		if !ok || cb == nil {
			continue
		}

		// If fence provided, encode a signal on the shared event.
		// MTLSharedEvent.signaledValue is updated by the GPU when the command
		// buffer completes — we do NOT set the Go-side value here.
		if fence != nil {
			if mtlFence, ok := fence.(*Fence); ok && mtlFence != nil {
				_ = MsgSend(cb.raw, Sel("encodeSignalEvent:value:"),
					uintptr(mtlFence.event), uintptr(fenceValue))
			}
		}

		// Schedule presentation BEFORE commit (Metal requirement)
		if cb.drawable != 0 {
			_ = MsgSend(cb.raw, Sel("presentDrawable:"), uintptr(cb.drawable))
		}

		// Commit the command buffer
		_ = MsgSend(cb.raw, Sel("commit"))
	}
	return nil
}

// ReadBuffer reads data from a buffer.
func (q *Queue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return fmt.Errorf("metal: invalid buffer")
	}
	ptr := buf.Contents()
	if ptr == nil {
		return fmt.Errorf("metal: buffer not mappable")
	}
	src := unsafe.Slice((*byte)(unsafe.Add(ptr, int(offset))), len(data))
	copy(data, src)
	return nil
}

// WriteBuffer writes data to a buffer immediately.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}

	ptr := buf.Contents()
	if ptr == nil {
		return // Buffer is not mappable
	}

	dst := unsafe.Slice((*byte)(unsafe.Add(ptr, int(offset))), len(data))
	copy(dst, data)
}

// WriteTexture writes data to a texture using a staging buffer and blit encoder.
//
// Metal textures with StorageModePrivate cannot be written from the CPU directly.
// This method creates a temporary Shared buffer, copies the pixel data into it,
// then uses a blit command encoder to copy from the buffer into the texture.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	tex, ok := dst.Texture.(*Texture)
	if !ok || tex == nil || len(data) == 0 || size == nil {
		return
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Create a temporary staging buffer with Shared storage mode.
	stagingBuffer := MsgSend(q.device.raw, Sel("newBufferWithBytes:length:options:"),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(MTLStorageModeShared))
	if stagingBuffer == 0 {
		hal.Logger().Warn("metal: WriteTexture staging buffer creation failed",
			"dataSize", len(data),
		)
		return
	}
	defer Release(stagingBuffer)

	// Create a one-shot command buffer for the blit operation.
	cmdBuffer := MsgSend(q.commandQueue, Sel("commandBuffer"))
	if cmdBuffer == 0 {
		hal.Logger().Warn("metal: WriteTexture command buffer creation failed")
		return
	}
	Retain(cmdBuffer)
	defer Release(cmdBuffer)

	blitEncoder := MsgSend(cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		hal.Logger().Warn("metal: WriteTexture blit encoder creation failed")
		return
	}

	// Calculate layout parameters.
	bytesPerRow := layout.BytesPerRow
	if bytesPerRow == 0 {
		// Estimate bytes per row from width and format (assume 4 bytes/pixel for RGBA8).
		bytesPerRow = size.Width * 4
	}
	layers := size.DepthOrArrayLayers
	if layers == 0 {
		layers = 1
	}
	bytesPerImage := layout.RowsPerImage * bytesPerRow
	if bytesPerImage == 0 {
		bytesPerImage = size.Height * bytesPerRow
	}

	sourceOrigin := MTLOrigin{
		X: NSUInteger(dst.Origin.X),
		Y: NSUInteger(dst.Origin.Y),
		Z: NSUInteger(dst.Origin.Z),
	}
	sourceSize := MTLSize{
		Width:  NSUInteger(size.Width),
		Height: NSUInteger(size.Height),
		Depth:  NSUInteger(layers),
	}

	msgSendVoid(blitEncoder, Sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
		argPointer(uintptr(stagingBuffer)),
		argUint64(uint64(layout.Offset)),
		argUint64(uint64(bytesPerRow)),
		argUint64(uint64(bytesPerImage)),
		argStruct(sourceSize, mtlSizeType),
		argPointer(uintptr(tex.raw)),
		argUint64(uint64(dst.Origin.Z)),
		argUint64(uint64(dst.MipLevel)),
		argStruct(sourceOrigin, mtlOriginType),
	)

	_ = MsgSend(blitEncoder, Sel("endEncoding"))

	// Commit and wait synchronously — WriteTexture is a blocking API.
	_ = MsgSend(cmdBuffer, Sel("commit"))
	_ = MsgSend(cmdBuffer, Sel("waitUntilCompleted"))

	hal.Logger().Debug("metal: WriteTexture completed",
		"width", size.Width,
		"height", size.Height,
		"dataSize", len(data),
		"format", tex.format,
	)
}

// Present presents a surface texture to the screen.
//
// Note: On Metal, the actual presentation is scheduled via presentDrawable:
// in Submit() BEFORE the command buffer is committed. This ensures proper
// synchronization between GPU work and display.
//
// This method only releases the drawable reference. The present was already
// scheduled during Submit() if a drawable was attached to the command buffer.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	st, ok := texture.(*SurfaceTexture)
	if !ok || st == nil {
		return nil
	}

	// Release drawable reference (presentation was scheduled in Submit)
	if st.drawable != 0 {
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
