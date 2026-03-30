package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
)

// Queue handles command submission and data transfers.
type Queue struct {
	hal       hal.Queue
	halDevice hal.Device
	device    *Device
}

// Submit submits command buffers for execution. Non-blocking.
// Returns a submission index that can be used with Poll() to track completion.
// Command buffers are owned by the caller — free them after Poll confirms completion.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	if q.hal == nil {
		return 0, fmt.Errorf("wgpu: queue not available")
	}

	halBuffers := make([]hal.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		if cb == nil {
			return 0, fmt.Errorf("wgpu: command buffer at index %d is nil", i)
		}
		halBuffers[i] = cb.halBuffer()
	}

	subIdx, err := q.hal.Submit(halBuffers)
	if err != nil {
		return 0, fmt.Errorf("wgpu: submit failed: %w", err)
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
	// Queue no longer owns fences — HAL manages synchronization internally.
}
