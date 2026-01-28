// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gles

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/gputypes"
)

// Surface implements hal.Surface for OpenGL on Linux.
type Surface struct {
	displayHandle uintptr
	windowHandle  uintptr
	eglCtx        *egl.Context
	eglDisplay    egl.EGLDisplay
	eglSurface    egl.EGLSurface
	glCtx         *gl.Context
	version       string
	renderer      string
	configured    bool
	config        *hal.SurfaceConfiguration
}

// GetAdapterInfo returns adapter information from this surface's GL context.
func (s *Surface) GetAdapterInfo() hal.ExposedAdapter {
	vendor := s.glCtx.GetString(gl.VENDOR)

	// Query capabilities
	var maxTextureSize int32
	s.glCtx.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxTextureSize)

	var maxDrawBuffers int32
	s.glCtx.GetIntegerv(gl.MAX_DRAW_BUFFERS, &maxDrawBuffers)

	limits := gputypes.DefaultLimits()
	limits.MaxTextureDimension1D = uint32(maxTextureSize)
	limits.MaxTextureDimension2D = uint32(maxTextureSize)
	limits.MaxColorAttachments = uint32(maxDrawBuffers)

	return hal.ExposedAdapter{
		Adapter: &Adapter{
			glCtx:         s.glCtx,
			eglCtx:        s.eglCtx,
			displayHandle: s.displayHandle,
			windowHandle:  s.windowHandle,
			version:       s.version,
			renderer:      s.renderer,
		},
		Info: gputypes.AdapterInfo{
			Name:       s.renderer,
			Vendor:     vendor,
			VendorID:   0,
			DeviceID:   0,
			DeviceType: gputypes.DeviceTypeDiscreteGPU,
			Driver:     s.version,
			DriverInfo: "OpenGL 3.3+",
			Backend:    gputypes.BackendGL,
		},
		Features: 0, // Note: Feature detection requires GL extension queries.
		Capabilities: hal.Capabilities{
			Limits: limits,
			AlignmentsMask: hal.Alignments{
				BufferCopyOffset: 4,
				BufferCopyPitch:  256,
			},
			DownlevelCapabilities: hal.DownlevelCapabilities{
				ShaderModel: 50, // SM5.0
				Flags:       0,
			},
		},
	}
}

// Configure configures the surface for presentation.
//
// Returns hal.ErrZeroArea if width or height is zero.
// This commonly happens when the window is minimized or not yet fully visible.
// Wait until the window has valid dimensions before calling Configure again.
func (s *Surface) Configure(_ hal.Device, config *hal.SurfaceConfiguration) error {
	// Validate dimensions first (before any side effects).
	// This matches wgpu-core behavior which returns ConfigureSurfaceError::ZeroArea.
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	s.configured = true
	s.config = config
	return nil
}

// Unconfigure marks the surface as unconfigured.
func (s *Surface) Unconfigure(_ hal.Device) {
	s.configured = false
	s.config = nil
}

// AcquireTexture returns the next surface texture for rendering.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return &hal.AcquiredSurfaceTexture{
		Texture: &SurfaceTexture{
			surface: s,
		},
		Suboptimal: false,
	}, nil
}

// DiscardTexture discards a previously acquired texture.
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {}

// Destroy releases the surface resources.
func (s *Surface) Destroy() {
	if s.eglCtx != nil {
		s.eglCtx.Destroy()
		s.eglCtx = nil
	}
}

// SurfaceTexture implements hal.SurfaceTexture for OpenGL.
// It represents the default framebuffer.
type SurfaceTexture struct {
	surface *Surface
}

// Destroy is a no-op for surface textures.
func (t *SurfaceTexture) Destroy() {}
