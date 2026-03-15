package wgpu

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gogpu/wgpu/hal"
)

// defaultSubmitTimeout is the maximum time to wait for GPU work to complete
// after submitting command buffers. 30 seconds accommodates heavy compute workloads.
const defaultSubmitTimeout = 30 * time.Second

// Queue handles command submission and data transfers.
type Queue struct {
	hal        hal.Queue
	halDevice  hal.Device
	fence      hal.Fence
	fenceValue atomic.Uint64
	device     *Device
}

// Submit submits command buffers for execution.
// This is a synchronous operation - it blocks until the GPU has completed all submitted work.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) error {
	if q.hal == nil {
		return fmt.Errorf("wgpu: queue not available")
	}

	halBuffers := make([]hal.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		if cb == nil {
			return fmt.Errorf("wgpu: command buffer at index %d is nil", i)
		}
		halBuffers[i] = cb.halBuffer()
	}

	nextValue := q.fenceValue.Add(1)
	err := q.hal.Submit(halBuffers, q.fence, nextValue)
	if err != nil {
		return fmt.Errorf("wgpu: submit failed: %w", err)
	}

	_, err = q.halDevice.Wait(q.fence, nextValue, defaultSubmitTimeout)
	if err != nil {
		return fmt.Errorf("wgpu: wait failed: %w", err)
	}

	for _, cb := range commandBuffers {
		raw := cb.halBuffer()
		if raw != nil {
			q.halDevice.FreeCommandBuffer(raw)
		}
	}

	return nil
}

// SubmitWithFence submits command buffers for execution with fence-based tracking.
// Unlike Submit, this method does NOT block until GPU completion. Instead, it
// signals the provided fence with submissionIndex when the work completes.
// The caller is responsible for polling or waiting on the fence.
//
// If fence is nil, the submission proceeds without fence signaling.
// Command buffers must be freed by the caller after the fence signals
// (use Device.FreeCommandBuffer).
func (q *Queue) SubmitWithFence(commandBuffers []*CommandBuffer, fence *Fence, submissionIndex uint64) error {
	if q.hal == nil {
		return fmt.Errorf("wgpu: queue not available")
	}

	halBuffers := make([]hal.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		if cb == nil {
			return fmt.Errorf("wgpu: command buffer at index %d is nil", i)
		}
		halBuffers[i] = cb.halBuffer()
	}

	var halFence hal.Fence
	if fence != nil {
		halFence = fence.hal
	}

	err := q.hal.Submit(halBuffers, halFence, submissionIndex)
	if err != nil {
		return fmt.Errorf("wgpu: submit failed: %w", err)
	}

	return nil
}

// WriteBuffer writes data to a buffer.
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	if q.hal == nil || buffer == nil {
		return fmt.Errorf("wgpu: WriteBuffer: queue or buffer is nil")
	}

	halBuffer := buffer.halBuffer()
	if halBuffer == nil {
		return fmt.Errorf("wgpu: WriteBuffer: no HAL buffer")
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
	return q.hal.WriteTexture(halDst, data, &halLayout, &halSize)
}

// release cleans up queue resources.
func (q *Queue) release() {
	if q.fence != nil && q.halDevice != nil {
		q.halDevice.DestroyFence(q.fence)
		q.fence = nil
	}
}
