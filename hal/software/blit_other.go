//go:build !windows

package software

// blitFramebufferToWindow is a no-op on non-Windows platforms.
// TODO: implement XPutImage for Linux X11, CoreGraphics for macOS.
func blitFramebufferToWindow(_ uintptr, _ []byte, _, _ int32) {}
