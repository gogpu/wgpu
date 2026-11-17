//go:build linux && !(js && wasm)

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

// contextHandles reports the EGL display, context and config of the shared
// AdapterContext.
func contextHandles(c *AdapterContext) publichal.GLESHandles {
	if c == nil {
		return publichal.GLESHandles{}
	}
	eglCtx := c.EGL()
	if eglCtx == nil {
		return publichal.GLESHandles{}
	}
	return publichal.GLESHandles{
		EGLDisplay: uintptr(eglCtx.Display()),
		EGLContext: uintptr(eglCtx.EGLContext()),
		EGLConfig:  uintptr(eglCtx.Config()),
	}
}

// NativeHandles reports the OpenGL objects behind the instance.
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

// NativeHandles reports the EGLSurface of the surface; it is zero while the
// surface is not configured.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDGLES),
		GLES: publichal.GLESHandles{EGLSurface: uintptr(s.eglSurface)},
	}
}
