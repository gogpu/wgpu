//go:build !windows

package software

// blitFramebufferToWindow is a no-op on non-Windows platforms.
// TODO: Implement X11 XPutImage for Linux, CoreGraphics for macOS.
func blitFramebufferToWindow(_ uintptr, _ []byte, _, _ int32) {}
