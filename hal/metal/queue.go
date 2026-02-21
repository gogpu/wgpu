// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
)

// maxFramesInFlight is the maximum number of frames the CPU can get ahead of
// the GPU. A value of 2 matches the Vulkan and DX12 backends and provides good
// latency/throughput balance. When the CPU tries to submit a frame beyond this
// limit, it blocks until the GPU finishes an earlier frame, preventing unbounded
// resource growth and drawable pool exhaustion.
const maxFramesInFlight = 2

// Queue implements hal.Queue for Metal.
type Queue struct {
	device       *Device
	commandQueue ID // id<MTLCommandQueue>

	// frameSemaphore limits CPU-ahead-of-GPU frames. Each Submit consumes a
	// slot from the buffered channel; the GPU's addCompletedHandler callback
	// returns the slot when the command buffer finishes execution.
	// nil if block support is unavailable (graceful degradation).
	frameSemaphore chan struct{}
}

// Submit submits command buffers to the GPU.
//
// Frame throttling: when frameSemaphore is initialized, Submit blocks until a
// frame slot is available (at most maxFramesInFlight frames in-flight). A
// completion handler on the last command buffer signals the semaphore when the
// GPU finishes, releasing the slot for the next frame. This prevents unbounded
// memory growth from queued command buffers and avoids drawable pool exhaustion.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	// Acquire a frame slot — blocks if maxFramesInFlight frames are in-flight.
	// This is the CPU-side throttle point.
	if q.frameSemaphore != nil {
		<-q.frameSemaphore
	}

	hal.Logger().Debug("metal: Submit",
		"buffers", len(commandBuffers),
		"hasFence", fence != nil,
	)

	pool := NewAutoreleasePool()
	defer pool.Drain()

	lastIdx := len(commandBuffers) - 1
	for i, buf := range commandBuffers {
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
			hal.Logger().Debug("metal: presentDrawable scheduled")
		}

		// On the last command buffer, register a completion handler to release
		// the frame semaphore slot when the GPU finishes this batch.
		if i == lastIdx && q.frameSemaphore != nil {
			q.registerFrameCompletionHandler(cb.raw)
			hal.Logger().Debug("metal: frame completion handler registered")
		}

		// Commit the command buffer
		_ = MsgSend(cb.raw, Sel("commit"))
	}

	// If there were no valid command buffers but we acquired a semaphore slot,
	// release it immediately to avoid deadlock.
	if lastIdx < 0 && q.frameSemaphore != nil {
		q.frameSemaphore <- struct{}{}
	}

	return nil
}

// registerFrameCompletionHandler attaches an addCompletedHandler: block to the
// command buffer that signals frameSemaphore when the GPU finishes execution.
func (q *Queue) registerFrameCompletionHandler(cmdBuffer ID) {
	blockPtr := newFrameCompletionBlock(q.frameSemaphore)
	if blockPtr == 0 {
		// Block creation failed — release the semaphore slot immediately
		// so the pipeline does not deadlock. This degrades gracefully to
		// no throttling for this frame.
		hal.Logger().Warn("metal: frame completion block creation failed")
		q.frameSemaphore <- struct{}{}
		return
	}

	// addCompletedHandler: copies the block internally, so the Go-side
	// blockLiteral can be collected after this call returns.
	_ = MsgSend(cmdBuffer, Sel("addCompletedHandler:"), blockPtr)
	runtime.KeepAlive(blockPtr)
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
//
// The staging buffer is released asynchronously via addCompletedHandler when
// the GPU finishes the blit, avoiding a full pipeline stall. If block creation
// fails, falls back to synchronous waitUntilCompleted + immediate Release.
//
// The caller's data slice is consumed synchronously — newBufferWithBytes copies
// the bytes into the staging buffer before this method returns, so the caller
// may reuse or free the data slice immediately.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	tex, ok := dst.Texture.(*Texture)
	if !ok || tex == nil || len(data) == 0 || size == nil {
		return
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Create a temporary staging buffer with Shared storage mode.
	// newBufferWithBytes copies data[] into GPU-visible memory synchronously,
	// so the caller's slice is consumed before this method returns.
	stagingBuffer := MsgSend(q.device.raw, Sel("newBufferWithBytes:length:options:"),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(MTLStorageModeShared))
	if stagingBuffer == 0 {
		hal.Logger().Warn("metal: WriteTexture staging buffer creation failed",
			"dataSize", len(data),
		)
		return
	}
	// Do NOT defer Release(stagingBuffer) — it will be released either by
	// the completion handler (async path) or explicitly (sync fallback).

	// Create a one-shot command buffer for the blit operation.
	cmdBuffer := MsgSend(q.commandQueue, Sel("commandBuffer"))
	if cmdBuffer == 0 {
		Release(stagingBuffer)
		hal.Logger().Warn("metal: WriteTexture command buffer creation failed")
		return
	}
	Retain(cmdBuffer)

	blitEncoder := MsgSend(cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		Release(stagingBuffer)
		Release(cmdBuffer)
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

	// Try async path: register a completion handler to release the staging
	// buffer when the GPU finishes the blit. This avoids a full pipeline stall
	// that waitUntilCompleted causes (multi-ms per 4K texture).
	blockPtr, blockID := newCompletedHandlerBlock(stagingBuffer)
	if blockPtr != 0 {
		// Register completion handler BEFORE commit.
		// addCompletedHandler: retains the command buffer internally.
		_ = MsgSend(cmdBuffer, Sel("addCompletedHandler:"), blockPtr)

		// Commit — GPU will execute the blit asynchronously.
		_ = MsgSend(cmdBuffer, Sel("commit"))

		// Release our reference to the command buffer. The Metal runtime
		// retains it until the completion handler fires.
		Release(cmdBuffer)

		// Keep the block literal alive until after commit + addCompletedHandler
		// have consumed it. The Metal runtime copies block data during
		// addCompletedHandler, so after this point the Go-side literal
		// can be collected.
		runtime.KeepAlive(blockPtr)
		// Suppress "unused" warning — blockID is used only in the cancellation
		// path below and runtime.KeepAlive above prevents premature GC.
		_ = blockID

		hal.Logger().Debug("metal: WriteTexture committed (async)",
			"width", size.Width,
			"height", size.Height,
			"dataSize", len(data),
			"format", tex.format,
		)
		return
	}

	// Fallback: block creation failed — use synchronous path.
	_ = MsgSend(cmdBuffer, Sel("commit"))
	_ = MsgSend(cmdBuffer, Sel("waitUntilCompleted"))
	Release(stagingBuffer)
	Release(cmdBuffer)

	hal.Logger().Debug("metal: WriteTexture completed (sync fallback)",
		"width", size.Width,
		"height", size.Height,
		"dataSize", len(data),
		"format", tex.format,
	)
}

// Present presents a surface texture to the screen.
//
// Creates a dedicated command buffer, calls presentDrawable:, and commits.
// This matches the Rust wgpu Metal backend pattern where presentation is
// handled in a separate command buffer from rendering work.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	hal.Logger().Debug("metal: Present")
	st, ok := texture.(*SurfaceTexture)
	if !ok || st == nil {
		return nil
	}

	if st.drawable != 0 {
		pool := NewAutoreleasePool()

		// Create a dedicated command buffer for presentation
		cmdBuffer := MsgSend(q.commandQueue, Sel("commandBuffer"))
		if cmdBuffer != 0 {
			_ = MsgSend(cmdBuffer, Sel("presentDrawable:"), uintptr(st.drawable))
			_ = MsgSend(cmdBuffer, Sel("commit"))
			hal.Logger().Debug("metal: presentDrawable committed")
		}

		Release(st.drawable)
		st.drawable = 0

		pool.Drain()
	}

	return nil
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// Metal timestamps are in nanoseconds
	return 1.0
}
