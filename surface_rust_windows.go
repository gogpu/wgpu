//go:build rust && windows

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// createPlatformSurface creates a rendering surface on Windows via HWND.
func createPlatformSurface(instance *rwgpu.Instance, displayHandle, windowHandle uintptr) (*rwgpu.Surface, error) {
	return instance.CreateSurfaceFromWindowsHWND(displayHandle, windowHandle)
}
