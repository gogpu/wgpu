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

// Present blits the software-rendered framebuffer to the window.
// The framebuffer is in BGRA byte order (matching the surface format) after
// executeFullscreenBlit's RGBA→BGRA swizzle. GDI BI_RGB 32-bit expects BGRA,
// so no additional conversion is needed — the framebuffer is passed directly.
// In headless mode (hwnd == 0), this is a no-op.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture) error {
	s, ok := surface.(*Surface)
	if !ok || s.hwnd == 0 {
		return nil
	}

	s.mu.RLock()
	fb := s.framebuffer
	w := s.width
	h := s.height
	s.mu.RUnlock()

	if len(fb) == 0 || w == 0 || h == 0 {
		return nil
	}

	// Framebuffer is BGRA — pass directly to GDI, no swap needed.
	blitFramebufferToWindow(s.hwnd, fb, int32(w), int32(h))
	return nil
}

// GetTimestampPeriod returns 1.0 nanosecond timestamp period.
func (q *Queue) GetTimestampPeriod() float32 {
	return 1.0
}

// SupportsCommandBufferCopies returns false for the software backend.
// Writes are handled directly via memcpy without command buffer batching.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return false
}
