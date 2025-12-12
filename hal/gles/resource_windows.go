// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
	"github.com/gogpu/wgpu/types"
)

// Surface implements hal.Surface for OpenGL on Windows.
type Surface struct {
	hwnd       wgl.HWND
	wglCtx     *wgl.Context
	glCtx      *gl.Context
	version    string
	renderer   string
	configured bool
	config     *hal.SurfaceConfiguration
}

// GetAdapterInfo returns adapter information from this surface's GL context.
func (s *Surface) GetAdapterInfo() hal.ExposedAdapter {
	vendor := s.glCtx.GetString(gl.VENDOR)

	// Query capabilities
	var maxTextureSize int32
	s.glCtx.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxTextureSize)

	var maxDrawBuffers int32
	s.glCtx.GetIntegerv(gl.MAX_DRAW_BUFFERS, &maxDrawBuffers)

	limits := types.DefaultLimits()
	limits.MaxTextureDimension1D = uint32(maxTextureSize)
	limits.MaxTextureDimension2D = uint32(maxTextureSize)
	limits.MaxColorAttachments = uint32(maxDrawBuffers)

	return hal.ExposedAdapter{
		Adapter: &Adapter{
			glCtx:    s.glCtx,
			wglCtx:   s.wglCtx,
			hwnd:     s.hwnd,
			version:  s.version,
			renderer: s.renderer,
		},
		Info: types.AdapterInfo{
			Name:       s.renderer,
			Vendor:     vendor,
			VendorID:   0,
			DeviceID:   0,
			DeviceType: types.DeviceTypeDiscreteGPU,
			Driver:     s.version,
			DriverInfo: "OpenGL 3.3+",
			Backend:    types.BackendGL,
		},
		Features: 0, // TODO: Query features
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
func (s *Surface) Configure(_ hal.Device, config *hal.SurfaceConfiguration) error {
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
	if s.wglCtx != nil {
		s.wglCtx.Destroy(s.hwnd)
		s.wglCtx = nil
	}
}

// SurfaceTexture implements hal.SurfaceTexture for OpenGL on Windows.
// It represents the default framebuffer.
type SurfaceTexture struct {
	surface *Surface
}

// Destroy is a no-op for surface textures.
func (t *SurfaceTexture) Destroy() {}
