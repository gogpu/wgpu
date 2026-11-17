//go:build windows && !(js && wasm)

package gles

import (
	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/hal"
)

var (
	_ hal.NativeHandlesProvider = (*Instance)(nil)
	_ hal.NativeHandlesProvider = (*Adapter)(nil)
	_ hal.NativeHandlesProvider = (*Device)(nil)
	_ hal.NativeHandlesProvider = (*Surface)(nil)
)

// contextHandles reports the WGL context of the shared AdapterContext. The
// context is created lazily on the render thread, so both handles are zero
// until it exists.
func contextHandles(c *AdapterContext) publichal.GLESHandles {
	if c == nil {
		return publichal.GLESHandles{}
	}
	return publichal.GLESHandles{HGLRC: uintptr(c.hglrc), HDC: uintptr(c.hiddenDC)}
}

// NativeHandles reports the OpenGL objects behind the instance. On Windows they
// are the WGL rendering context and its hidden device context.
func (i *Instance) NativeHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDGLES),
		GLES: contextHandles(i.ctx),
	}
}

// NativeHandles reports the OpenGL objects behind the adapter.
func (a *Adapter) NativeHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDGLES),
		GLES: contextHandles(a.ctx),
	}
}

// NativeHandles reports the OpenGL objects behind the device.
func (d *Device) NativeHandles() publichal.Handles {
	if d == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDGLES),
		GLES: contextHandles(d.ctx),
	}
}

// NativeHandles reports the OpenGL objects behind the surface. On Windows the
// presentation object is the caller's window, which this package does not
// name, so only the Kind is set.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDGLES)}
}
