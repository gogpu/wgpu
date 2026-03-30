package software

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
)

// Queue implements hal.Queue for the software backend.
type Queue struct {
	submissionIndex uint64
}

// Submit simulates command buffer submission.
// Software backend is synchronous — work is complete immediately.
func (q *Queue) Submit(_ []hal.CommandBuffer) (uint64, error) {
	q.submissionIndex++
	return q.submissionIndex, nil
}

// PollCompleted returns the highest submission index known to be completed.
// Software backend is synchronous — all submissions are immediately complete.
func (q *Queue) PollCompleted() uint64 {
	return q.submissionIndex
}

// ReadBuffer reads data from a buffer.
func (q *Queue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	if b, ok := buffer.(*Buffer); ok && b.data != nil {
		b.mu.RLock()
		copy(data, b.data[offset:])
		b.mu.RUnlock()
	}
	return nil
}

// WriteBuffer performs immediate buffer writes with real data storage.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	b, ok := buffer.(*Buffer)
	if !ok {
		return fmt.Errorf("software: WriteBuffer: invalid buffer type")
	}
	b.WriteData(offset, data)
	return nil
}

// WriteTexture performs immediate texture writes with real data storage.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	if tex, ok := dst.Texture.(*Texture); ok {
		// Simple implementation: just write data at offset
		// In a real implementation, this would respect layout parameters
		tex.WriteData(layout.Offset, data)
	}
	return nil
}

// Present simulates surface presentation.
// In software backend, this is essentially a no-op since framebuffer is already updated.
func (q *Queue) Present(_ hal.Surface, _ hal.SurfaceTexture) error {
	// In software backend, the framebuffer is already updated by render operations
	// Present just marks the frame as complete
	return nil
}

// GetTimestampPeriod returns 1.0 nanosecond timestamp period.
func (q *Queue) GetTimestampPeriod() float32 {
	return 1.0
}
