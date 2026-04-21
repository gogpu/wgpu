//go:build !windows && !linux

package software

// platformBlit is a no-op on platforms without native blit support.
// Windows has GDI (blit_windows.go), Linux has X11 (blit_linux.go).
type platformBlit struct{}

// createPlatformFramebuffer returns nil — use Go heap memory.
func (s *Surface) createPlatformFramebuffer(_, _ int32) []byte { return nil }

// destroyPlatformFramebuffer is a no-op.
func (s *Surface) destroyPlatformFramebuffer() {}

// blitFramebufferToWindow is a no-op on unsupported platforms.
// TODO: implement CGImage+CALayer blit for macOS (Phase 2).
func (s *Surface) blitFramebufferToWindow(_ []byte, _, _ int32) {}
