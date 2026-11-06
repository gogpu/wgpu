//go:build !(js && wasm)

package software

import (
	"fmt"
	"image"
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

// resolveTexture extracts the underlying *Texture from a HAL texture.
// Handles both *Texture and *SurfaceTexture (which embeds Texture by value).
func resolveTexture(t hal.Texture) *Texture {
	switch v := t.(type) {
	case *Texture:
		return v
	case *SurfaceTexture:
		return &v.Texture
	default:
		return nil
	}
}

// WriteTexture performs immediate texture writes with real data storage: the
// region of size at dst.Origin of mip level dst.MipLevel (Origin.Z is the
// first array layer or depth slice; every layer of size.DepthOrArrayLayers
// is written), read from data laid out by layout (Offset, BytesPerRow,
// RowsPerImage), as CopyBufferToTexture reads a buffer. Each level is
// written at its own pitch (subresource.go). Rows are counted in texel
// blocks, so a block-compressed format uploads block row by block row.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	tex := resolveTexture(dst.Texture)
	if tex == nil {
		return nil
	}
	if size == nil || len(data) == 0 {
		return nil
	}
	var l hal.ImageDataLayout
	if layout != nil {
		l = *layout
	}
	r, ok := tex.regionOf(dst.MipLevel, dst.Origin, *size)
	if !ok {
		logRegionOutside("WriteTexture", tex, dst.MipLevel, dst.Origin, *size)
		return nil
	}

	tex.mu.Lock()
	defer tex.mu.Unlock()
	tex.copyLinear(r, data, l, true, dst.Aspect)
	return nil
}

// Present blits the software-rendered framebuffer to the window.
// The framebuffer is in BGRA byte order (matching the surface format) after
// executeFullscreenBlit's RGBA→BGRA swizzle. GDI BI_RGB 32-bit expects BGRA,
// so no additional conversion is needed — the framebuffer is passed directly.
// In headless mode (hwnd == 0), this is a no-op.
//
// When damageRects is non-empty, only the specified regions are blitted to the
// window (partial BitBlt on Windows, partial XPutImage on Linux). This is the
// damage-aware presentation path for GUI frameworks where only a small region
// (spinner, cursor blink) changed between frames. Rects are clipped to surface
// bounds before blitting.
//
// When damageRects is nil or empty, the entire surface is blitted — identical
// to the legacy full-surface present path.
//
// IMPORTANT: Uses the SurfaceTexture's buffer (captured at AcquireTexture time),
// NOT s.framebuffer (which may have been replaced by Configure during resize).
// This prevents race: render draws into acquired buffer, Configure allocates new one,
// Present must blit the buffer that was actually rendered into.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture, damageRects []image.Rectangle) error {
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
			"surface_w", s.width, "surface_h", s.height,
			"damageRects", len(damageRects))
		if len(damageRects) > 0 {
			s.blitDamageRectsToWindow(st.data, int32(w), int32(h), damageRects)
		} else {
			s.blitFramebufferToWindow(st.data, int32(w), int32(h))
		}
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

	if len(damageRects) > 0 {
		s.blitDamageRectsToWindow(fb, int32(w), int32(h), damageRects)
	} else {
		s.blitFramebufferToWindow(fb, int32(w), int32(h))
	}
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

// SetSwapchainSuppressed is a no-op on the software backend.
// Software backend has no GPU semaphores. See BUG-WGPU-VK-005 (Vulkan-specific).
func (q *Queue) SetSwapchainSuppressed(_ bool) {}
