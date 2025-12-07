package noop

import (
	"github.com/gogpu/wgpu/hal"
)

// Queue implements hal.Queue for the noop backend.
type Queue struct{}

// Submit simulates command buffer submission.
// If a fence is provided, it is signaled with the given value.
func (q *Queue) Submit(_ []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	if fence != nil {
		if f, ok := fence.(*Fence); ok {
			f.value.Store(fenceValue)
		}
	}
	return nil
}

// WriteBuffer simulates immediate buffer writes.
// If the buffer has storage, copies data to it.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	if b, ok := buffer.(*Buffer); ok && b.data != nil {
		copy(b.data[offset:], data)
	}
}

// WriteTexture simulates immediate texture writes.
// This is a no-op since textures don't store data.
func (q *Queue) WriteTexture(_ *hal.ImageCopyTexture, _ []byte, _ *hal.ImageDataLayout, _ *hal.Extent3D) {
}

// Present simulates surface presentation.
// Always succeeds.
func (q *Queue) Present(_ hal.Surface, _ hal.SurfaceTexture) error {
	return nil
}

// GetTimestampPeriod returns 1.0 nanosecond timestamp period.
func (q *Queue) GetTimestampPeriod() float32 {
	return 1.0
}
