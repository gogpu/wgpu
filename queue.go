//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pendingwrites"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/resource"
)

// Queue handles command submission and data transfers.
//
// Queue is safe for concurrent use from multiple goroutines. All mutating
// operations (Submit, WriteBuffer, WriteTexture) are serialized via an
// internal mutex, matching Rust wgpu-core which uses RwLock::write() on
// device.fence + device.command_indices during submit (queue.rs:1171-1173).
//
// The implementation lives in internal/queue (ADR-070 S9); this type is the
// declaration layer the public API is frozen on, and every method converts at
// the call boundary.
type Queue struct {
	impl *queue.Queue
}

// newQueue returns the declaration layer around a destination queue: the
// queue's HAL backend and its pending-write batch. The device the queue
// submits through is attached with impl.SetDevice once both exist.
func newQueue(halQ hal.Queue, pending *pendingwrites.Writes) *Queue {
	return &Queue{impl: queue.New(halQ, pending)}
}

// Submit submits command buffers for execution. Non-blocking.
// Returns a submission index that can be used with Poll() to track completion.
// Command buffers are owned by the caller — free them after Poll confirms completion.
//
// If there are pending WriteBuffer/WriteTexture operations, they are flushed
// and prepended before the user command buffers in a single HAL submit.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	return q.impl.Submit(submitCommandBuffers(commandBuffers))
}

// submitCommandBuffers unwraps the public command buffer handles for the
// submit path. A nil *CommandBuffer is a nil element (Submit reports it); a
// zero-value one stays a zero implementation, which the submit path accepts
// and spends exactly as the pre-move zero value was: it records no commands
// and holds nothing to validate or recycle.
func submitCommandBuffers(commandBuffers []*CommandBuffer) []*encoder.CommandBuffer {
	impls := make([]*encoder.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		switch {
		case cb == nil:
			impls[i] = nil
		case cb.impl == nil:
			impls[i] = &encoder.CommandBuffer{}
		default:
			impls[i] = cb.impl
		}
	}
	return impls
}

// Poll returns the last completed submission index. Non-blocking.
// All submissions with index <= the returned value have been completed by the GPU.
func (q *Queue) Poll() uint64 {
	return q.impl.Poll()
}

// WriteBuffer writes data to a buffer.
// If PendingWrites batching is enabled (DX12/Vulkan/Metal), the write is
// recorded into a shared command encoder and flushed on the next Submit.
// For GLES/Software backends, the write is performed immediately.
//
// MapWrite buffers (upload heap on DX12, host-visible on Vulkan) are written
// directly via HAL without staging — GPU copy into upload heap is undefined
// behavior on DX12 (upload heap is GENERIC_READ, read-only to GPU).
// See BUG-DX12-003.
//
// Validation (VAL-A1, WebGPU spec §21.1):
//   - Buffer must not be currently mapped
//   - Buffer must have CopyDst usage
//   - offset must be 4-byte aligned
//   - data size must be 4-byte aligned
//   - offset + data size must not exceed buffer size
//
// Matches Rust wgpu-core queue.rs:647-672 (validate_write_buffer_impl).
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	return q.impl.WriteBuffer(bufferImpl(buffer), offset, data)
}

// WriteTexture writes data to a texture.
// If PendingWrites batching is enabled (DX12/Vulkan/Metal), the write is
// recorded into a shared command encoder and flushed on the next Submit.
// Resource barriers are computed from the texture's tracked CurrentUsage().
// For GLES/Software backends, the write is performed immediately via HAL.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	return q.impl.WriteTexture(queueImageCopyTexture(dst), data, queueImageDataLayout(layout), queueExtent3D(size))
}

// SetSwapchainSuppressed temporarily disables swapchain semaphore binding
// for subsequent Submit calls. Used for offscreen renders (e.g., RepaintBoundary)
// that must not consume acquire/present semaphores intended for the compositor
// submit.
//
// BUG-WGPU-VK-005: When rendering to an offscreen texture before compositing
// to the swapchain surface, the offscreen submit must not hijack swapchain
// semaphores. Without suppression, the compositor submit runs without GPU-side
// synchronization, causing a race condition (flickering).
//
// Call with true before offscreen submits, false after:
//
//	queue.SetSwapchainSuppressed(true)
//	defer queue.SetSwapchainSuppressed(false)
//	queue.Submit(offscreenCmds...)
//
// Only meaningful on Vulkan — other backends are no-ops.
func (q *Queue) SetSwapchainSuppressed(suppressed bool) {
	q.impl.SetSwapchainSuppressed(suppressed)
}

// LastSubmissionIndex returns the most recent submission index.
// Used by resource Release() and onZero callbacks to schedule deferred destruction.
// Lock-free via atomic — safe to call from onZero callbacks during Triage
// while Queue.mu is held by Submit. ADR-056 deadlock fix.
func (q *Queue) LastSubmissionIndex() uint64 {
	return q.impl.LastSubmissionIndex()
}

// GetTimestampPeriod returns the number of nanoseconds per timestamp query
// tick, as Rust wgpu's Queue::get_timestamp_period: multiply the difference
// of two resolved timestamps by it to get nanoseconds. It returns 1.0 when
// the queue has no backend.
func (q *Queue) GetTimestampPeriod() float32 {
	return q.impl.GetTimestampPeriod()
}

// bufferImpl returns the implementation handle of b, or nil for an absent
// buffer: the destination's own nil check reports it, exactly as before the
// move.
func bufferImpl(b *Buffer) *resource.Buffer {
	if b == nil {
		return nil
	}
	return b.impl
}

// queueImageCopyTexture converts a public upload descriptor into the
// implementation's. A nil dst or a nil texture becomes a nil field, so the
// destination's own nil checks keep their exact errors.
func queueImageCopyTexture(dst *ImageCopyTexture) *queue.ImageCopyTexture {
	if dst == nil {
		return nil
	}
	return &queue.ImageCopyTexture{
		Texture:  textureImpl(dst.Texture),
		MipLevel: dst.MipLevel,
		Origin:   queue.Origin3D(dst.Origin),
		Aspect:   dst.Aspect,
	}
}

// textureImpl returns the implementation handle of t, or nil for an
// absent texture.
func textureImpl(t *Texture) *resource.Texture {
	if t == nil {
		return nil
	}
	return t.impl
}

// queueImageDataLayout converts a public layout into the implementation's, or
// nil for an absent layout.
func queueImageDataLayout(l *ImageDataLayout) *queue.ImageDataLayout {
	if l == nil {
		return nil
	}
	return &queue.ImageDataLayout{
		Offset:       l.Offset,
		BytesPerRow:  l.BytesPerRow,
		RowsPerImage: l.RowsPerImage,
	}
}

// queueExtent3D converts a public extent into the implementation's, or nil for
// an absent extent.
func queueExtent3D(e *Extent3D) *queue.Extent3D {
	if e == nil {
		return nil
	}
	return &queue.Extent3D{
		Width:              e.Width,
		Height:             e.Height,
		DepthOrArrayLayers: e.DepthOrArrayLayers,
	}
}
