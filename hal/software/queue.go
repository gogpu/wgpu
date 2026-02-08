//go:build software

package software

import (
	"github.com/gogpu/wgpu/hal"
)

// Queue implements hal.Queue for the software backend.
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
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	if b, ok := buffer.(*Buffer); ok {
		b.WriteData(offset, data)
	}
}

// WriteTexture performs immediate texture writes with real data storage.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	if tex, ok := dst.Texture.(*Texture); ok {
		// Simple implementation: just write data at offset
		// In a real implementation, this would respect layout parameters
		tex.WriteData(layout.Offset, data)
	}
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
