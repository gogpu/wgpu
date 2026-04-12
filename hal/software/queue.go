package software

import (
	"fmt"
	"log/slog"

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
// Respects dst.Origin (destination position), layout.BytesPerRow (source stride),
// layout.Offset (source data offset), and size (region dimensions).
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	tex, ok := dst.Texture.(*Texture)
	if !ok {
		return nil
	}
	if size == nil || len(data) == 0 {
		return nil
	}

	const bytesPerPixel = 4

	srcBytesPerRow := layout.BytesPerRow
	if srcBytesPerRow == 0 {
		srcBytesPerRow = size.Width * bytesPerPixel
	}

	dstBytesPerRow := uint64(tex.width) * bytesPerPixel

	tex.mu.Lock()
	defer tex.mu.Unlock()

	for row := uint32(0); row < size.Height; row++ {
		srcStart := layout.Offset + uint64(row)*uint64(srcBytesPerRow)
		srcEnd := srcStart + uint64(size.Width)*bytesPerPixel
		if srcEnd > uint64(len(data)) {
			break
		}

		dstStart := uint64(dst.Origin.Y+row)*dstBytesPerRow + uint64(dst.Origin.X)*bytesPerPixel
		dstEnd := dstStart + uint64(size.Width)*bytesPerPixel
		if dstEnd > uint64(len(tex.data)) {
			break
		}

		copy(tex.data[dstStart:dstEnd], data[srcStart:srcEnd])
	}

	return nil
}

// Present blits the software-rendered framebuffer to the window.
// The framebuffer is in BGRA byte order (matching the surface format) after
// executeFullscreenBlit's RGBA→BGRA swizzle. GDI BI_RGB 32-bit expects BGRA,
// so no additional conversion is needed — the framebuffer is passed directly.
// In headless mode (hwnd == 0), this is a no-op.
//
// IMPORTANT: Uses the SurfaceTexture's buffer (captured at AcquireTexture time),
// NOT s.framebuffer (which may have been replaced by Configure during resize).
// This prevents race: render draws into acquired buffer, Configure allocates new one,
// Present must blit the buffer that was actually rendered into.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	s, ok := surface.(*Surface)
	if !ok || s.hwnd == 0 {
		return nil
	}

	// Use the texture that was acquired and rendered into this frame.
	// This is safe even if Configure() replaced s.framebuffer mid-frame.
	if st, ok := texture.(*SurfaceTexture); ok && st.data != nil {
		w := st.width
		h := st.height
		if w == 0 || h == 0 {
			return nil
		}
		slog.Debug("software: Present",
			"tex_w", w, "tex_h", h,
			"surface_w", s.width, "surface_h", s.height)
		s.blitFramebufferToWindow(st.data, int32(w), int32(h))
		return nil
	}

	// Fallback: use surface framebuffer (legacy path).
	s.mu.RLock()
	fb := s.framebuffer
	w := s.width
	h := s.height
	s.mu.RUnlock()

	if len(fb) == 0 || w == 0 || h == 0 {
		return nil
	}

	s.blitFramebufferToWindow(fb, int32(w), int32(h))
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
