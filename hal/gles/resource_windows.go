// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
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

	// Swapchain offscreen framebuffer. User render passes that target this
	// Surface render into swapchainFBO (backed by colorRenderbuffer), not FBO 0.
	// Queue.Present blits this FBO to the default framebuffer with an explicit
	// Y-flip before SwapBuffers. Mirrors Rust wgpu-hal/src/gles/egl.rs
	// Surface::configure (1537-1562) / Surface::present (1280-1308).
	swapchainFBO        uint32
	colorRenderbuffer   uint32
	fboWidth, fboHeight uint32
}

// GetAdapterInfo returns adapter information from this surface's GL context.
// Probes GL version, extensions, features, limits, and MSAA support to build
// an accurate ExposedAdapter. Follows Rust wgpu-hal adapter.rs expose pattern.
func (s *Surface) GetAdapterInfo() hal.ExposedAdapter {
	caps := queryAdapterCapabilities(s.glCtx)

	driverInfo := "OpenGL 3.3+"
	if caps.IsES {
		driverInfo = fmt.Sprintf("OpenGL ES %d.%d", caps.GLMajor, caps.GLMinor)
	} else if caps.GLMajor > 0 {
		driverInfo = fmt.Sprintf("OpenGL %d.%d", caps.GLMajor, caps.GLMinor)
	}

	return hal.ExposedAdapter{
		Adapter: &Adapter{
			glCtx:    s.glCtx,
			wglCtx:   s.wglCtx,
			hwnd:     s.hwnd,
			version:  s.version,
			renderer: s.renderer,
			caps:     caps,
		},
		Info: gputypes.AdapterInfo{
			Name:       caps.Renderer,
			Vendor:     caps.Vendor,
			VendorID:   caps.VendorID,
			DeviceID:   0,
			DeviceType: caps.DeviceType,
			Driver:     caps.Version,
			DriverInfo: driverInfo,
			Backend:    gputypes.BackendGL,
		},
		Features: caps.Features,
		Capabilities: hal.Capabilities{
			Limits: caps.Limits,
			AlignmentsMask: hal.Alignments{
				BufferCopyOffset: 4,
				BufferCopyPitch:  4,
			},
			DownlevelCapabilities: hal.DownlevelCapabilities{
				ShaderModel: 50, // SM5.0
				Flags:       caps.DownlevelFlags,
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

	// Load WGL extensions and set swap interval for VSync control.
	// wglGetProcAddress requires a current GL context.
	if s.wglCtx != nil {
		wgl.LoadExtensions(s.wglCtx.HDC())

		if wgl.HasSwapControl() {
			var interval int
			switch config.PresentMode {
			case hal.PresentModeFifo, hal.PresentModeFifoRelaxed:
				interval = 1 // VSync on
			case hal.PresentModeImmediate, hal.PresentModeMailbox:
				interval = 0 // VSync off
			default:
				interval = 1 // Default to VSync
			}
			if err := wgl.SetSwapInterval(interval); err != nil {
				return fmt.Errorf("gles: failed to set swap interval: %w", err)
			}
		}
	}

	// Allocate / resize the swapchain offscreen FBO. User render passes
	// target this FBO; Present blits it to FBO 0 with Y-flip.
	if err := s.reconfigureSwapchainFBO(config.Format, config.Width, config.Height); err != nil {
		return fmt.Errorf("gles: failed to configure swapchain framebuffer: %w", err)
	}

	s.configured = true
	s.config = config
	return nil
}

// Unconfigure marks the surface as unconfigured.
func (s *Surface) Unconfigure(_ hal.Device) {
	destroySwapchainFBO(s.glCtx, s.swapchainFBO, s.colorRenderbuffer)
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0
	s.fboWidth = 0
	s.fboHeight = 0
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

// ActualExtent returns the configured surface dimensions.
// GLES does not clamp the extent, so these always match the requested values.
// Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s.config == nil {
		return 0, 0
	}
	return s.config.Width, s.config.Height
}

// Destroy releases the surface resources.
func (s *Surface) Destroy() {
	// Release swapchain FBO before tearing down the GL context.
	destroySwapchainFBO(s.glCtx, s.swapchainFBO, s.colorRenderbuffer)
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0
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

// CurrentUsage returns 0 — GLES surface textures have no state tracking.
func (t *SurfaceTexture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *SurfaceTexture) AddPendingRef()                      {}
func (t *SurfaceTexture) DecPendingRef()                      {}

// Destroy is a no-op for surface textures.
func (t *SurfaceTexture) Destroy() {}

// NativeHandle returns 0 (OpenGL default framebuffer has no handle).
func (t *SurfaceTexture) NativeHandle() uintptr { return 0 }
