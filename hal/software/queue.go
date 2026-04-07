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

// Present presents the rendered framebuffer to the window.
// If the surface has a valid window handle, it blits the framebuffer via
// platform-native APIs (GDI StretchDIBits on Windows).
// In headless mode (hwnd == 0), this is a no-op.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture) error {
	s, ok := surface.(*Surface)
	if !ok || s.hwnd == 0 {
		return nil // headless mode — no window to blit to
	}

	s.mu.RLock()
	fb := s.framebuffer
	width := int32(s.width)
	height := int32(s.height)
	s.mu.RUnlock()

	if len(fb) == 0 || width <= 0 || height <= 0 {
		return nil
	}

	// Convert RGBA -> BGRA and blit to window.
	// Reuse blitBuf on Surface to avoid per-frame allocation.
	needed := int(width) * int(height) * 4
	if len(s.blitBuf) < needed {
		s.blitBuf = make([]byte, needed)
	}
	for i := 0; i < needed; i += 4 {
		s.blitBuf[i+0] = fb[i+2] // B
		s.blitBuf[i+1] = fb[i+1] // G
		s.blitBuf[i+2] = fb[i+0] // R
		s.blitBuf[i+3] = fb[i+3] // A
	}

	blitFramebufferToWindow(s.hwnd, s.blitBuf[:needed], width, height)
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
