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

// release cleans up queue resources.
func (q *Queue) release() {
	if q.fence != nil && q.halDevice != nil {
		q.halDevice.DestroyFence(q.fence)
		q.fence = nil
	}
}
