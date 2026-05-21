// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// Surface implements hal.Surface for OpenGL on Windows.
// Lightweight — does NOT own the GL context. The context lives on Instance's
// hidden window via AdapterContext. Surface stores only the user HWND and a
// reference to the shared AdapterContext.
//
// Follows Rust wgpu-hal/src/gles/wgl.rs Surface (lines 672-677).
type Surface struct {
	hwnd       wgl.HWND
	ctx        *AdapterContext // shared, NOT owned
	configured bool
	config     *hal.SurfaceConfiguration

	// Swapchain offscreen framebuffer. User render passes that target this
	// Surface render into swapchainFBO (backed by colorRenderbuffer), not FBO 0.
	// Queue.Present blits this FBO to the default framebuffer with an explicit
	// Y-flip before SwapBuffers.
	swapchainFBO        uint32
	colorRenderbuffer   uint32
	fboWidth, fboHeight uint32
}

// GetAdapterInfo returns adapter information from this surface's GL context.
// Not used in the new architecture (Instance.EnumerateAdapters queries directly),
// but kept for interface compatibility.
func (s *Surface) GetAdapterInfo() hal.ExposedAdapter {
	glCtx := s.ctx.Lock()
	defer s.ctx.Unlock()

	caps := queryAdapterCapabilities(glCtx)

	driverInfo := "OpenGL 3.3+"
	if caps.IsES {
		driverInfo = fmt.Sprintf("OpenGL ES %d.%d", caps.GLMajor, caps.GLMinor)
	} else if caps.GLMajor > 0 {
		driverInfo = fmt.Sprintf("OpenGL %d.%d", caps.GLMajor, caps.GLMinor)
	}

	return hal.ExposedAdapter{
		Adapter: &Adapter{
			ctx:     s.ctx,
			version: fmt.Sprintf("%d.%d", caps.GLMajor, caps.GLMinor),
			caps:    caps,
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
				ShaderModel: 50,
				Flags:       caps.DownlevelFlags,
			},
		},
	}
}

// Configure configures the surface for presentation.
//
// Uses two separate lock scopes to avoid stomping MakeCurrent state:
// 1. LockForDC(userDC) — set swap interval (requires context on user DC)
// 2. Lock() — allocate swapchain FBO (requires context on hidden DC)
func (s *Surface) Configure(_ hal.Device, config *hal.SurfaceConfiguration) error {
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	// Scope 1: Set swap interval on user window DC.
	// SetSwapInterval requires a current context on the target DC.
	hdc := wgl.GetDC(s.hwnd)
	if hdc != 0 {
		s.ctx.LockForDC(hdc)
		wgl.LoadExtensions(hdc)
		if wgl.HasSwapControl() {
			var interval int
			switch config.PresentMode {
			case hal.PresentModeFifo, hal.PresentModeFifoRelaxed:
				interval = 1
			case hal.PresentModeImmediate, hal.PresentModeMailbox:
				interval = 0
			default:
				interval = 1
			}
			_ = wgl.SetSwapInterval(interval)
		}
		s.ctx.Unlock()
		wgl.ReleaseDC(s.hwnd, hdc)
	}

	// Scope 2: Allocate swapchain FBO on hidden DC.
	glCtx := s.ctx.Lock()
	defer s.ctx.Unlock()

	if err := s.reconfigureSwapchainFBOWith(glCtx, config.Format, config.Width, config.Height); err != nil {
		return fmt.Errorf("gles: failed to configure swapchain framebuffer: %w", err)
	}

	s.configured = true
	s.config = config
	return nil
}

// Unconfigure marks the surface as unconfigured.
func (s *Surface) Unconfigure(_ hal.Device) {
	glCtx := s.ctx.Lock()
	defer s.ctx.Unlock()

	destroySwapchainFBO(glCtx, s.swapchainFBO, s.colorRenderbuffer)
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
func (s *Surface) ActualExtent() (width, height uint32) {
	if s.config == nil {
		return 0, 0
	}
	return s.config.Width, s.config.Height
}

// Destroy releases the surface resources.
// Does NOT destroy the GL context — that's owned by Instance.
func (s *Surface) Destroy() {
	if s.ctx != nil {
		glCtx := s.ctx.Lock()
		destroySwapchainFBO(glCtx, s.swapchainFBO, s.colorRenderbuffer)
		s.ctx.Unlock()
	}
	s.swapchainFBO = 0
	s.colorRenderbuffer = 0
}

// SurfaceTexture implements hal.SurfaceTexture for OpenGL on Windows.
type SurfaceTexture struct {
	surface *Surface
}

func (t *SurfaceTexture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *SurfaceTexture) AddPendingRef()                      {}
func (t *SurfaceTexture) DecPendingRef()                      {}
func (t *SurfaceTexture) Destroy()                            {}
func (t *SurfaceTexture) NativeHandle() uintptr               { return 0 }
