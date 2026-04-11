//go:build !windows

package software

// platformBlit is a no-op on non-Windows platforms.
// On Windows this struct has real GDI fields (memDC, bitmap, oldBmp).
type platformBlit struct{}

// createPlatformFramebuffer returns nil on non-Windows — use Go memory.
func (s *Surface) createPlatformFramebuffer(_, _ int32) []byte { return nil }

// destroyPlatformFramebuffer is a no-op on non-Windows.
func (s *Surface) destroyPlatformFramebuffer() {}

// blitFramebufferToWindow is a no-op on non-Windows platforms.
// TODO: implement XPutImage for Linux X11, CGContextDrawImage for macOS.
func (s *Surface) blitFramebufferToWindow(_ []byte, _, _ int32) {}
