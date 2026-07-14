// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"image"
	"sync/atomic"
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

	// submissionIndex is a monotonically increasing counter for tracking submissions.
	submissionIndex uint64

	// completedIndex tracks the highest submission index completed by the GPU.
	// Updated atomically by addCompletedHandler blocks that fire when the GPU
	// finishes executing the last command buffer of each Submit batch.
	// This matches Rust wgpu-hal's Fence.completed_value pattern.
	completedIndex atomic.Uint64

	// frameSemaphore limits CPU-ahead-of-GPU frames. Each Submit consumes a
	// slot from the buffered channel; the GPU's addCompletedHandler callback
	// returns the slot when the command buffer finishes execution.
	// nil if block support is unavailable (graceful degradation).
	frameSemaphore chan struct{}
}

// Submit submits command buffers to the GPU.
// Returns a monotonically increasing submission index for tracking completion.
//
// Frame throttling: when frameSemaphore is initialized, Submit blocks until a
// frame slot is available (at most maxFramesInFlight frames in-flight). A
// completion handler on the last command buffer signals the semaphore when the
// GPU finishes, releasing the slot for the next frame. This prevents unbounded
// memory growth from queued command buffers and avoids drawable pool exhaustion.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	// Acquire a frame slot — blocks if maxFramesInFlight frames are in-flight.
	// This is the CPU-side throttle point.
	if q.frameSemaphore != nil {
		<-q.frameSemaphore
	}

	hal.Logger().Debug("metal: Submit",
		"buffers", len(commandBuffers),
	)

	q.submissionIndex++
	subIdx := q.submissionIndex

	pool := NewAutoreleasePool()
	defer pool.Drain()

	lastIdx := len(commandBuffers) - 1
	for i, buf := range commandBuffers {
		cb, ok := buf.(*CommandBuffer)
		if !ok || cb == nil {
			continue
		}

		// On the last command buffer, register completion handlers.
		if i == lastIdx {
			// Track actual GPU completion for PollCompleted().
			// Uses addCompletedHandler to atomically store the submission index
			// when the GPU finishes, matching Rust wgpu-hal Fence.completed_value.
			q.registerSubmissionCompletionHandler(cb.raw, subIdx)

			// Release frame semaphore slot for CPU-ahead throttling.
			if q.frameSemaphore != nil {
				q.registerFrameCompletionHandler(cb.raw)
				hal.Logger().Debug("metal: frame completion handler registered")
			}
		}

		// Commit the command buffer
		_ = MsgSend(cb.raw, Sel("commit"))
	}

	// If there were no valid command buffers but we acquired a semaphore slot,
	// release it immediately to avoid deadlock.
	if lastIdx < 0 && q.frameSemaphore != nil {
		q.frameSemaphore <- struct{}{}
	}

	return subIdx, nil
}

// PollCompleted returns the highest submission index known to be completed by the GPU.
// Updated atomically by addCompletedHandler blocks registered in Submit.
// This matches Rust wgpu-hal's Fence.get_latest() / Device.get_fence_value() pattern.
func (q *Queue) PollCompleted() uint64 {
	return q.completedIndex.Load()
}

// registerSubmissionCompletionHandler attaches an addCompletedHandler: block to
// the command buffer that atomically stores the submission index when the GPU
// finishes execution. This provides accurate GPU completion tracking for
// PollCompleted(), replacing the conservative heuristic that returned
// submissionIndex - maxFramesInFlight.
//
// If block creation fails, the completedIndex is updated immediately as a
// fallback — this is conservative (reports completion too early rather than
// too late) but prevents maintain() from never recycling resources.
func (q *Queue) registerSubmissionCompletionHandler(cmdBuffer ID, subIdx uint64) {
	blockPtr := newGPUCompletionBlock(&q.completedIndex, subIdx)
	if blockPtr == 0 {
		// Block creation failed — update immediately as fallback.
		hal.Logger().Warn("metal: submission completion block creation failed, updating immediately")
		q.completedIndex.Store(subIdx)
		return
	}

	_ = MsgSend(cmdBuffer, Sel("addCompletedHandler:"), blockPtr)
	hal.Logger().Debug("metal: submission completion handler registered", "subIdx", subIdx)
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

	// With _NSConcreteGlobalBlock, Block_copy() is a no-op — Metal holds
	// the same pointer. The block is pinned via blockPinRegistry until
	// the completion callback fires and unpins it.
	_ = MsgSend(cmdBuffer, Sel("addCompletedHandler:"), blockPtr)
}

// WriteBuffer writes data to a buffer immediately.
//
// Fast path: if the buffer is CPU-mappable (Shared/Managed storage), copies
// data directly via memcpy. Slow path: if the buffer is GPU-only (Private
// storage), creates a temporary staging buffer and blits the data. The staging
// path matches the pattern used by WriteTexture and Rust wgpu.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return fmt.Errorf("metal: WriteBuffer: invalid buffer")
	}
	if len(data) == 0 {
		return nil
	}

	// Fast path: buffer is mappable (Shared/Managed storage mode).
	ptr := buf.Contents()
	if ptr != nil {
		dst := unsafe.Slice((*byte)(unsafe.Add(ptr, int(offset))), len(data))
		copy(dst, data)
		return nil
	}

	// Slow path: buffer is Private storage — use staging buffer + blit.
	// This is a defense-in-depth fallback; with the CopyDst→Shared fix in
	// CreateBuffer, this path should rarely be reached.
	return q.writeBufferStaged(buf, offset, data)
}

// writeBufferStaged copies data to a Private-mode buffer via a temporary
// Shared staging buffer and a blit command. This mirrors the staging pattern
// used by WriteTexture and matches Rust wgpu's Queue::write_buffer behavior.
func (q *Queue) writeBufferStaged(buf *Buffer, offset uint64, data []byte) error {
	hal.Logger().Debug("metal: WriteBuffer using staging path",
		"size", len(data), "offset", offset)

	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Create temporary Shared staging buffer with the data.
	staging := MsgSend(q.device.raw, Sel("newBufferWithBytes:length:options:"),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		uintptr(MTLResourceStorageModeShared))
	if staging == 0 {
		return fmt.Errorf("metal: WriteBuffer: staging buffer creation failed (size=%d)", len(data))
	}

	// Create a one-shot command buffer for the blit.
	cmdBuffer := MsgSend(q.commandQueue, Sel("commandBuffer"))
	if cmdBuffer == 0 {
		Release(staging)
		return fmt.Errorf("metal: WriteBuffer: command buffer creation failed")
	}
	Retain(cmdBuffer)

	blitEncoder := MsgSend(cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		Release(staging)
		Release(cmdBuffer)
		return fmt.Errorf("metal: WriteBuffer: blit encoder creation failed")
	}

	// copyFromBuffer:sourceOffset:toBuffer:destinationOffset:size:
	msgSendVoid(blitEncoder, Sel("copyFromBuffer:sourceOffset:toBuffer:destinationOffset:size:"),
		argPointer(uintptr(staging)),
		argUint64(0),
		argPointer(uintptr(buf.raw)),
		argUint64(offset),
		argUint64(uint64(len(data))),
	)
	_ = MsgSend(blitEncoder, Sel("endEncoding"))

	// Try async release via completion handler (same pattern as WriteTexture).
	blockPtr := newCompletedHandlerBlock(staging)
	if blockPtr != 0 {
		_ = MsgSend(cmdBuffer, Sel("addCompletedHandler:"), blockPtr)
		_ = MsgSend(cmdBuffer, Sel("commit"))
		Release(cmdBuffer)
		// Block is pinned via blockPinRegistry until the completion callback
		// fires and unpins it. No runtime.KeepAlive needed.
		return nil
	}

	// Fallback: synchronous path.
	_ = MsgSend(cmdBuffer, Sel("commit"))
	_ = MsgSend(cmdBuffer, Sel("waitUntilCompleted"))
	Release(staging)
	Release(cmdBuffer)
	return nil
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
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	if dst == nil || layout == nil {
		return fmt.Errorf("metal: WriteTexture: invalid arguments")
	}
	tex, ok := dst.Texture.(*Texture)
	if !ok || tex == nil || len(data) == 0 || size == nil {
		return fmt.Errorf("metal: WriteTexture: invalid arguments")
	}
	copyLayout, err := validateMetalTextureDataCopyLayout(uint64(len(data)), tex.format, *layout, *size)
	if err != nil {
		return fmt.Errorf("metal: WriteTexture: %w", err)
	}
	normalizedLayout := *layout
	normalizedLayout.BytesPerRow = copyLayout.bytesPerRow
	plan, bytesPerImage, ok := validateMetalBufferTextureCopyPlan(tex.format, tex.dimension, normalizedLayout, dst.Origin, *size)
	if !ok || bytesPerImage != copyLayout.bytesPerImage {
		return fmt.Errorf("metal: WriteTexture: copy address arithmetic overflows")
	}

	// Apple Silicon UMA fast path: Shared textures accept synchronous CPU writes
	// via replaceRegion:. This avoids per-upload staging buffers and one-shot
	// blit command buffers — the main source of resize-induced memory growth.
	if tex.isShared {
		return q.writeTextureShared(tex, dst, data, layout, copyLayout, size, plan)
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Create a temporary staging buffer with Shared storage mode.
	// newBufferWithBytes copies data[] into GPU-visible memory synchronously,
	// so the caller's slice is consumed before this method returns.
	stagingBuffer := MsgSend(q.device.raw, Sel("newBufferWithBytes:length:options:"),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(MTLStorageModeShared))
	if stagingBuffer == 0 {
		return fmt.Errorf("metal: WriteTexture: staging buffer creation failed (dataSize=%d)", len(data))
	}
	// Do NOT defer Release(stagingBuffer) — it will be released either by
	// the completion handler (async path) or explicitly (sync fallback).

	// Create a one-shot command buffer for the blit operation.
	cmdBuffer := MsgSend(q.commandQueue, Sel("commandBuffer"))
	if cmdBuffer == 0 {
		Release(stagingBuffer)
		return fmt.Errorf("metal: WriteTexture: command buffer creation failed")
	}
	Retain(cmdBuffer)

	blitEncoder := MsgSend(cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		Release(stagingBuffer)
		Release(cmdBuffer)
		return fmt.Errorf("metal: WriteTexture: blit encoder creation failed")
	}

	strides := metalBlitStrides(tex.dimension, uint64(copyLayout.bytesPerRow), copyLayout.bytesPerImage)
	for operation := uint32(0); operation < plan.operationCount; operation++ {
		destination, _ := plan.textureRegion(tex.dimension, dst.Origin, *size, operation)
		sourceOffset, _ := plan.bufferOffset(layout.Offset, copyLayout.bytesPerImage, operation)
		msgSendVoid(blitEncoder, Sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
			argPointer(uintptr(stagingBuffer)),
			argUint64(sourceOffset),
			argUint64(strides.bytesPerRow),
			argUint64(strides.bytesPerImage),
			argStruct(destination.size, mtlSizeType),
			argPointer(uintptr(tex.raw)),
			argUint64(uint64(destination.slice)),
			argUint64(uint64(dst.MipLevel)),
			argStruct(destination.origin, mtlOriginType),
		)
	}

	_ = MsgSend(blitEncoder, Sel("endEncoding"))

	// Try async path: register a completion handler to release the staging
	// buffer when the GPU finishes the blit. This avoids a full pipeline stall
	// that waitUntilCompleted causes (multi-ms per 4K texture).
	blockPtr := newCompletedHandlerBlock(stagingBuffer)
	if blockPtr != 0 {
		// Register completion handler BEFORE commit.
		// addCompletedHandler: retains the command buffer internally.
		_ = MsgSend(cmdBuffer, Sel("addCompletedHandler:"), blockPtr)

		// Commit — GPU will execute the blit asynchronously.
		_ = MsgSend(cmdBuffer, Sel("commit"))

		// Release our reference to the command buffer. The Metal runtime
		// retains it until the completion handler fires.
		Release(cmdBuffer)

		// Block is pinned via blockPinRegistry until the completion callback
		// fires and unpins it. No runtime.KeepAlive needed.

		hal.Logger().Debug("metal: WriteTexture committed (async)",
			"width", size.Width,
			"height", size.Height,
			"dataSize", len(data),
			"format", tex.format,
		)
		return nil
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
	return nil
}

// writeTextureShared writes pixel data directly into a Shared-storage Metal texture.
// It uses the slice-aware replaceRegion form so 2D arrays and true 3D textures
// preserve their distinct Metal layouts. The write is synchronous; no GPU command
// buffer or staging buffer is required. Valid only when tex.isShared is true.
func (q *Queue) writeTextureShared(tex *Texture, dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, copyLayout metalTextureDataCopyLayout, size *hal.Extent3D, plan metalCopyPlan) error {
	strides := metalReplaceRegionStrides(tex.dimension, uint64(copyLayout.bytesPerRow), copyLayout.bytesPerImage)
	for operation := uint32(0); operation < plan.operationCount; operation++ {
		offset, _ := plan.bufferOffset(layout.Offset, copyLayout.bytesPerImage, operation)
		src := data[offset:]
		destination, _ := plan.textureRegion(tex.dimension, dst.Origin, *size, operation)
		region := MTLRegion{Origin: destination.origin, Size: destination.size}

		// The slice-aware form carries an image stride for true 3D writes. Array
		// writes target one physical slice and therefore pass a zero image stride.
		msgSendVoid(tex.raw, Sel("replaceRegion:mipmapLevel:slice:withBytes:bytesPerRow:bytesPerImage:"),
			argStruct(region, mtlRegionType),
			argUint64(uint64(dst.MipLevel)),
			argUint64(uint64(destination.slice)),
			argPointer(uintptr(unsafe.Pointer(&src[0]))),
			argUint64(strides.bytesPerRow),
			argUint64(strides.bytesPerImage),
		)
	}

	hal.Logger().Debug("metal: WriteTexture (shared direct)",
		"width", size.Width,
		"height", size.Height,
		"bytesPerRow", copyLayout.bytesPerRow,
	)
	return nil
}

// Present presents a surface texture to the screen.
//
// When the surface uses presentsWithTransaction (default on macOS), presentation
// follows the wgpu-hal pattern: commit an empty command buffer, wait until
// scheduled, then call CAMetalDrawable.present. This synchronizes with Core
// Animation during live window resize (wgpu #3756).
//
// damageRects is accepted but ignored — Metal has no compositor damage API.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture, _ []image.Rectangle) error {
	hal.Logger().Debug("metal: Present")
	st, ok := texture.(*SurfaceTexture)
	if !ok || st == nil {
		return nil
	}

	if st.drawable == 0 {
		return nil
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	useTransaction := false
	if ms, ok := surface.(*Surface); ok && ms != nil {
		useTransaction = ms.presentsWithTransaction
	}

	cmdBuffer := MsgSend(q.commandQueue, Sel("commandBuffer"))
	if cmdBuffer == 0 {
		Release(st.drawable)
		st.drawable = 0
		return nil
	}

	if !useTransaction {
		_ = MsgSend(cmdBuffer, Sel("presentDrawable:"), uintptr(st.drawable))
	}
	_ = MsgSend(cmdBuffer, Sel("commit"))

	if useTransaction {
		_ = MsgSend(cmdBuffer, Sel("waitUntilScheduled"))
		_ = MsgSend(st.drawable, Sel("present"))
		hal.Logger().Debug("metal: presentDrawable (transaction) committed")
	} else {
		hal.Logger().Debug("metal: presentDrawable committed")
	}

	// Present consumes the surface texture: release both retains taken in
	// AcquireTexture. The MTLTexture retain is balanced here (not in
	// DestroyTexture, which skips isExternal textures) — leaking it pins the
	// drawable's IOSurface forever, accumulating gigabytes across frames.
	st.releaseAcquired()

	return nil
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
func (q *Queue) GetTimestampPeriod() float32 {
	// Metal timestamps are in nanoseconds
	return 1.0
}

// SupportsCommandBufferCopies returns true for Metal.
// Metal uses command buffers for copy operations — PendingWrites batches them.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return true
}

// SetSwapchainSuppressed is a no-op on Metal.
// Metal presents via CAMetalDrawable which is not affected by command buffer
// submission ordering — each presentDrawable: call operates on a specific
// drawable, not on implicit semaphore state. See BUG-WGPU-VK-005 (Vulkan-specific).
func (q *Queue) SetSwapchainSuppressed(_ bool) {}
