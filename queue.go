package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Queue handles command submission and data transfers.
type Queue struct {
	hal       hal.Queue
	halDevice hal.Device
	device    *Device
	pending   *pendingWrites

	// lastSubmissionIndex is the most recent submission index returned by
	// hal.Queue.Submit(). Used by DestroyQueue to conservatively defer
	// resource destruction until after the latest known submission completes.
	lastSubmissionIndex uint64
}

// Submit submits command buffers for execution. Non-blocking.
// Returns a submission index that can be used with Poll() to track completion.
// Command buffers are owned by the caller — free them after Poll confirms completion.
//
// If there are pending WriteBuffer/WriteTexture operations, they are flushed
// and prepended before the user command buffers in a single HAL submit.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	if q.hal == nil {
		return 0, fmt.Errorf("wgpu: queue not available")
	}

	// Flush pending writes under lock, then release lock before HAL submit.
	var pendingCmdBuf hal.CommandBuffer
	var flushedEncoder hal.CommandEncoder
	var flushedDstTextures []hal.Texture
	var flushedDstBuffers []hal.Buffer

	if q.pending != nil {
		q.pending.mu.Lock()
		var err error
		pendingCmdBuf, flushedEncoder, flushedDstTextures, flushedDstBuffers, err = q.pending.flush()
		q.pending.mu.Unlock()
		if err != nil {
			return 0, fmt.Errorf("wgpu: flush pending writes: %w", err)
		}
	}

	// Build combined command buffer list: pending first, then user buffers.
	var allBuffers []hal.CommandBuffer
	if pendingCmdBuf != nil {
		allBuffers = make([]hal.CommandBuffer, 0, 1+len(commandBuffers))
		allBuffers = append(allBuffers, pendingCmdBuf)
	} else {
		allBuffers = make([]hal.CommandBuffer, 0, len(commandBuffers))
	}

	for i, cb := range commandBuffers {
		if cb == nil {
			return 0, fmt.Errorf("wgpu: command buffer at index %d is nil", i)
		}
		allBuffers = append(allBuffers, cb.halBuffer())
	}

	subIdx, err := q.hal.Submit(allBuffers)
	if err != nil {
		return 0, fmt.Errorf("wgpu: submit failed: %w", err)
	}

	// Track the latest submission index for deferred resource destruction.
	q.lastSubmissionIndex = subIdx

	// Record inflight resources and clean up completed ones.
	// dstTextures/dstBuffers prevent premature Release (BUG-DX12-006: use-after-free).
	if q.pending != nil {
		q.pending.mu.Lock()
		hasInflightWork := pendingCmdBuf != nil || flushedDstTextures != nil
		if hasInflightWork {
			q.pending.inflight = append(q.pending.inflight, inflightSubmission{
				submissionIndex: subIdx,
				staging:         nil, // staging managed by belt
				cmdBuf:          pendingCmdBuf,
				encoder:         flushedEncoder,
				dstTextures:     flushedDstTextures,
				dstBuffers:      flushedDstBuffers,
			})
		}
		// Update the staging belt with the actual submission index
		// (belt.finish() was called during flush() before Submit).
		if q.pending.belt != nil {
			q.pending.belt.setLastSubmissionIndex(subIdx)
		}
		q.pending.maintain(q.hal.PollCompleted())
		q.pending.mu.Unlock()
	}

	// Collect tracked refs from command buffers and associate with this submission.
	// Phase 2: per-command-buffer resource tracking — refs are Drop'd when GPU completes.
	if dq := q.destroyQueue(); dq != nil {
		var allRefs []*core.ResourceRef
		for _, cb := range commandBuffers {
			if cb != nil && len(cb.trackedRefs) > 0 {
				allRefs = append(allRefs, cb.trackedRefs...)
				cb.trackedRefs = nil
			}
		}
		if len(allRefs) > 0 {
			dq.TrackSubmission(subIdx, allRefs)
		}
	}

	// Triage deferred resource destructions from the DestroyQueue.
	// Resources whose GPU submissions have completed are now safe to destroy.
	if dq := q.destroyQueue(); dq != nil {
		dq.Triage(q.hal.PollCompleted())
	}

	return subIdx, nil
}

// Poll returns the last completed submission index. Non-blocking.
// All submissions with index <= the returned value have been completed by the GPU.
func (q *Queue) Poll() uint64 {
	if q.hal == nil {
		return 0
	}
	return q.hal.PollCompleted()
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
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	if q.hal == nil || buffer == nil {
		return fmt.Errorf("wgpu: WriteBuffer: queue or buffer is nil")
	}

	halBuffer := buffer.halBuffer()
	if halBuffer == nil {
		return fmt.Errorf("wgpu: WriteBuffer: no HAL buffer")
	}

	// MapWrite buffers live on upload heap (DX12) or host-visible memory (Vulkan).
	// They support direct CPU writes — no staging buffer or GPU copy needed.
	// CopyBufferRegion into DX12 upload heap is undefined behavior (GENERIC_READ
	// is read-only to GPU), so we MUST bypass PendingWrites for these buffers.
	if buffer.Usage()&gputypes.BufferUsageMapWrite != 0 {
		return q.hal.WriteBuffer(halBuffer, offset, data)
	}

	if q.pending != nil {
		return q.pending.writeBuffer(halBuffer, buffer.Usage(), offset, data)
	}

	return q.hal.WriteBuffer(halBuffer, offset, data)
}

// ReadBuffer reads data from a GPU buffer.
func (q *Queue) ReadBuffer(buffer *Buffer, offset uint64, data []byte) error {
	if q.hal == nil {
		return fmt.Errorf("wgpu: queue not available")
	}
	if buffer == nil {
		return fmt.Errorf("wgpu: buffer is nil")
	}

	halBuffer := buffer.halBuffer()
	if halBuffer == nil {
		return ErrReleased
	}

	return q.hal.ReadBuffer(halBuffer, offset, data)
}

// WriteTexture writes data to a texture.
// If PendingWrites batching is enabled (DX12/Vulkan/Metal), the write is
// recorded into a shared command encoder and flushed on the next Submit.
// Resource barriers are computed from the texture's tracked CurrentUsage().
// For GLES/Software backends, the write is performed immediately via HAL.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	if q.hal == nil || dst == nil {
		return fmt.Errorf("wgpu: WriteTexture: queue or destination is nil")
	}
	if dst.Texture == nil || dst.Texture.hal == nil {
		return fmt.Errorf("wgpu: WriteTexture: destination texture is invalid")
	}
	if layout == nil {
		return fmt.Errorf("wgpu: WriteTexture: layout is nil")
	}
	if size == nil {
		return fmt.Errorf("wgpu: WriteTexture: size is nil")
	}

	halDst := dst.toHAL()
	halLayout := layout.toHAL()
	halSize := size.toHAL()

	if q.pending != nil {
		return q.pending.writeTexture(halDst, data, &halLayout, &halSize)
	}

	return q.hal.WriteTexture(halDst, data, &halLayout, &halSize)
}

// LastSubmissionIndex returns the most recent submission index.
// Used by resource Release() methods to schedule deferred destruction.
func (q *Queue) LastSubmissionIndex() uint64 {
	return q.lastSubmissionIndex
}

// destroyQueue returns the device's DestroyQueue, or nil if unavailable.
func (q *Queue) destroyQueue() *core.DestroyQueue {
	if q.device != nil && q.device.core != nil {
		return q.device.core.DestroyQueueRef()
	}
	return nil
}

// release cleans up queue resources.
func (q *Queue) release() {
	if q.pending != nil {
		q.pending.destroy()
		q.pending = nil
	}
}
