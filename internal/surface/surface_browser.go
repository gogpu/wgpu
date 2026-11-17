//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package surface holds the surface domain's implementation (ADR-070 S10).
//
// This context is the browser one: the implementation configures an
// HTMLCanvasElement's GPUCanvasContext against the GPUDevice it reaches
// through the one-method DeviceHost seam (seams_browser.go). The instance
// stays in the root package, which resolves the canvas and owns the surface's
// target ownership source.
package surface

import (
	"errors"
	"fmt"
	"image"
	"syscall/js"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/resource"
)

// Surface is the implementation of a platform rendering surface on the browser
// build: the canvas context, the seam view of the device it is configured
// against, and the configured texture format.
type Surface struct {
	browser *browser.Surface

	// device is the seam view of the device the surface is configured
	// against; nil until Configure succeeds.
	device   DeviceHost
	released bool

	// configFormat is the format GetCurrentTexture reports its texture in.
	configFormat gputypes.TextureFormat
}

// NewBrowser returns the implementation of a surface wrapped around an already
// created browser surface.
func NewBrowser(bs *browser.Surface) *Surface {
	return &Surface{browser: bs}
}

// Released reports whether the surface has been released. A nil receiver is
// the root declaration layer's zero value, which is not released.
func (s *Surface) Released() bool {
	return s != nil && s.released
}

// Browser returns the browser surface, or nil after Release.
func (s *Surface) Browser() *browser.Surface {
	if s == nil {
		return nil
	}
	return s.browser
}

// Configure configures the surface for presentation.
//
// Must be called before GetCurrentTexture(). Sets the canvas dimensions and
// calls GPUCanvasContext.configure() with the device, format, usage, and alpha mode.
//
// On browser, PresentMode is ignored (browser always uses FIFO / VSync).
func (s *Surface) Configure(host DeviceHost, config *SurfaceConfiguration) error {
	// A nil receiver is the root declaration layer's zero value: released is
	// false, so the pre-move answers are the nil-argument ones and ErrReleased
	// after them.
	if s == nil {
		if config == nil {
			return fmt.Errorf("wgpu: surface configuration is nil")
		}
		if host == nil {
			return fmt.Errorf("wgpu: device is nil")
		}
		return resource.ErrReleased
	}
	if s.released {
		return resource.ErrReleased
	}
	if config == nil {
		return fmt.Errorf("wgpu: surface configuration is nil")
	}
	if host == nil {
		return fmt.Errorf("wgpu: device is nil")
	}

	// Validate present mode (Rust wgpu panics on Mailbox/Immediate on web).
	switch config.PresentMode {
	case gputypes.PresentModeMailbox, gputypes.PresentModeImmediate:
		return fmt.Errorf("wgpu: present mode %v not supported on browser; only Fifo is supported", config.PresentMode)
	}

	// Build the JS GPUCanvasConfiguration object.
	jsConfig := browser.BuildSurfaceConfiguration(
		host.Browser().Ref(),
		config.Format,
		config.Usage,
		config.AlphaMode,
		nil, // viewFormats -- SurfaceConfiguration doesn't expose them yet
	)

	formatStr := browser.TextureFormatToJS(config.Format)
	s.browser.Configure(jsConfig, config.Width, config.Height, formatStr)
	s.device = host
	s.configFormat = config.Format
	return nil
}

// Unconfigure removes the surface configuration.
// After this call, GetCurrentTexture() will fail until Configure is called again.
func (s *Surface) Unconfigure() {
	if s == nil || s.released {
		return
	}
	s.browser.Unconfigure()
	s.device = nil
}

// GetCurrentTexture acquires the next texture for rendering.
//
// Returns the surface texture and whether the surface is suboptimal (always
// false on browser). The texture is automatically presented when the command
// buffer using it is submitted and control returns to the browser event loop.
//
// The texture is returned by value: the root declaration layer embeds it, so an
// acquisition costs the one allocation the public wrapper is, like before the
// move (ADR-070 §15.7).
func (s *Surface) GetCurrentTexture() (SurfaceTexture, bool, error) {
	if s == nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: surface not configured")
	}
	if s.released {
		return SurfaceTexture{}, false, resource.ErrReleased
	}
	if s.device == nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: surface not configured")
	}

	bt, err := s.browser.GetCurrentTexture()
	if err != nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: %w", err)
	}

	impl := resource.NewTexture(bt, s.configFormat)
	return SurfaceTexture{
		texture: impl,
	}, false, nil // Browser surfaces are never suboptimal
}

// Present presents a surface texture to the screen.
//
// On browser, this is a NO-OP. The swapchain is presented automatically when
// control returns to the browser event loop. This matches Rust wgpu where
// WebSurfaceOutputDetail::present() is an empty function.
func (s *Surface) Present(_ *SurfaceTexture) error {
	// No-op on browser. Presentation is automatic.
	return nil
}

// PresentWithDamage presents a surface texture, optionally with damage rects.
// On browser, damage rects are ignored — the browser composites the full canvas.
// This method exists for API compatibility with the native backend.
func (s *Surface) PresentWithDamage(st *SurfaceTexture, _ []image.Rectangle) error {
	return s.Present(st)
}

// ReadPixels is not supported by browser WebGPU surfaces.
// Headless surface readback is a Pure-Go software-backend extension.
func (s *Surface) ReadPixels() ([]byte, error) {
	if s == nil || s.released {
		return nil, resource.ErrReleased
	}
	if s.device == nil {
		return nil, fmt.Errorf("wgpu: surface not configured")
	}
	return nil, fmt.Errorf("wgpu: ReadPixels not supported on this backend")
}

// ActualExtent returns the configured surface dimensions.
// On browser, the canvas dimensions are always used as-is (no driver clamping).
// Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s == nil || s.released || s.browser == nil {
		return 0, 0
	}
	return s.browser.Width(), s.browser.Height()
}

// DiscardTexture discards the acquired surface texture without presenting it.
//
// On browser, this is a NO-OP. The browser does not support discarding a
// surface texture -- it will be presented regardless when the event loop runs.
func (s *Surface) DiscardTexture() {
	// No-op on browser. Cannot discard the texture.
}

// PresentPixels is not supported on browser backend.
// Software-backend extension for direct pixel presentation.
func (s *Surface) PresentPixels(_ []byte, _, _ uint32, _ []image.Rectangle) error {
	return errors.New("wgpu: PresentPixels is not supported on the browser backend")
}

// WritePixels is not supported on browser backend.
// Software-backend extension for direct framebuffer write.
func (s *Surface) WritePixels(_ []byte, _, _ uint32) error {
	return errors.New("wgpu: WritePixels is not supported on the browser backend")
}

// SetPresentsWithTransaction is a no-op on browser backend.
// Core Animation transactions are macOS-only.
func (s *Surface) SetPresentsWithTransaction(_ bool) {}

// Release releases the surface. The root declaration layer drops the target
// ownership source after this returns.
func (s *Surface) Release() {
	if s == nil || s.released {
		return
	}
	s.released = true
	if s.browser != nil {
		s.browser.Destroy()
	}
}

// SurfaceTexture is a texture acquired from a surface for rendering.
//
// On browser, the texture is automatically presented when the command buffer
// using it is submitted and control returns to the browser event loop.
type SurfaceTexture struct {
	texture  *resource.Texture
	released bool
}

// Resource returns the resource-package texture the surface acquired.
func (st *SurfaceTexture) Resource() *resource.Texture {
	if st == nil {
		return nil
	}
	return st.texture
}

// CreateView creates a texture view of this surface texture and returns the
// resource-package view; the root declaration layer wraps it.
//
// Pass nil for desc to create a default view (all mips, all layers, same format).
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*resource.TextureView, error) {
	if st == nil || st.released || st.texture == nil || st.texture.Browser() == nil {
		return nil, resource.ErrReleased
	}

	var jsDesc js.Value
	if desc != nil {
		jsDesc = browser.BuildTextureViewDescriptor(
			desc.Label,
			desc.Format, desc.Dimension, desc.Aspect,
			desc.BaseMipLevel, desc.MipLevelCount,
			desc.BaseArrayLayer, desc.ArrayLayerCount,
		)
	} else {
		jsDesc = js.Undefined()
	}

	bv := st.texture.Browser().CreateView(jsDesc)
	return resource.NewTextureView(bv, st.texture), nil
}
