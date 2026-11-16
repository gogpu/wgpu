//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package surface holds the surface domain's implementation (ADR-070 S10).
//
// This context is the rust one: the implementation configures a go-webgpu
// surface against the wgpu-native device it reaches through the one-method
// DeviceHost seam (seams_rust.go). The instance stays in the root package,
// which creates the surface and owns its target ownership source.
package surface

import (
	"errors"
	"fmt"
	"image"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// Surface is the implementation of a platform rendering surface on the rust
// build: the go-webgpu surface, the seam view of the wgpu-native device it is
// configured against, and the configuration fields texture creation needs.
type Surface struct {
	r *rwgpu.Surface

	// device is the seam view of the device the surface is configured
	// against; nil until Configure succeeds or after Unconfigure.
	device   DeviceHost
	released bool

	// Cached configuration for texture creation.
	configFormat gputypes.TextureFormat
	configWidth  uint32
	configHeight uint32
}

// NewR returns the implementation of a surface wrapped around an already
// created go-webgpu surface.
func NewR(r *rwgpu.Surface) *Surface {
	return &Surface{r: r}
}

// Released reports whether the surface has been released. A nil receiver is
// the root declaration layer's zero value, which is not released.
func (s *Surface) Released() bool {
	return s != nil && s.released
}

// R returns the go-webgpu surface, or nil after Release.
func (s *Surface) R() *rwgpu.Surface {
	if s == nil {
		return nil
	}
	return s.r
}

// Configure configures the surface for presentation.
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
		return core.ErrReleased
	}
	if s.released {
		return core.ErrReleased
	}
	if config == nil {
		return fmt.Errorf("wgpu: surface configuration is nil")
	}
	if host == nil {
		return fmt.Errorf("wgpu: device is nil")
	}

	rConfig := &rwgpu.SurfaceConfiguration{
		Format:      config.Format,
		Usage:       config.Usage,
		Width:       config.Width,
		Height:      config.Height,
		AlphaMode:   config.AlphaMode,
		PresentMode: config.PresentMode,
	}

	if err := s.r.Configure(host.R(), rConfig); err != nil {
		return fmt.Errorf("wgpu: failed to configure surface: %w", err)
	}

	s.device = host
	s.configFormat = config.Format
	s.configWidth = config.Width
	s.configHeight = config.Height
	return nil
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	if s == nil || s.released {
		return
	}
	s.r.Unconfigure()
	s.device = nil
}

// GetCurrentTexture acquires the next texture for rendering.
// Returns the surface texture and whether the surface is suboptimal.
//
// The texture is returned by value: the root declaration layer embeds it, so an
// acquisition costs the one allocation the public wrapper is, like before the
// move (ADR-070 §15.7).
func (s *Surface) GetCurrentTexture() (SurfaceTexture, bool, error) {
	if s == nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: surface not configured")
	}
	if s.released {
		return SurfaceTexture{}, false, core.ErrReleased
	}
	if s.device == nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: surface not configured")
	}

	rst, suboptimal, err := s.r.GetCurrentTexture()
	if err != nil {
		return SurfaceTexture{}, false, fmt.Errorf("wgpu: %w", err)
	}

	impl := resource.NewTexture(rst.Texture, s.configFormat)
	return SurfaceTexture{
		r:       rst,
		texture: impl,
		surface: s,
	}, suboptimal, nil
}

// Present presents a surface texture to the screen.
func (s *Surface) Present(texture *SurfaceTexture) error {
	if s == nil {
		if texture == nil {
			return fmt.Errorf("wgpu: surface texture is nil")
		}
		return core.ErrReleased
	}
	if s.released {
		return core.ErrReleased
	}
	if texture == nil {
		return fmt.Errorf("wgpu: surface texture is nil")
	}
	// go-webgpu Present takes variadic *SurfaceTexture.
	return s.r.Present(texture.r)
}

// PresentWithDamage presents a surface texture, optionally with damage rects.
// On Rust backend, damage rects are ignored. wgpu-native does not support them.
func (s *Surface) PresentWithDamage(st *SurfaceTexture, _ []image.Rectangle) error {
	return s.Present(st)
}

// ReadPixels is not supported by the Rust FFI backend.
// Headless surface readback is a Pure-Go software-backend extension.
func (s *Surface) ReadPixels() ([]byte, error) {
	if s == nil || s.released {
		return nil, core.ErrReleased
	}
	if s.device == nil {
		return nil, fmt.Errorf("wgpu: surface not configured")
	}
	return nil, fmt.Errorf("wgpu: ReadPixels not supported on this backend")
}

// ActualExtent returns the configured surface dimensions.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s == nil || s.released {
		return 0, 0
	}
	return s.configWidth, s.configHeight
}

// DiscardTexture discards the acquired surface texture without presenting it.
// On Rust backend, this is a no-op.
func (s *Surface) DiscardTexture() {
	// No-op: wgpu-native does not support texture discard.
}

// PresentPixels is not supported on Rust FFI backend.
// Software-backend extension for direct pixel presentation.
func (s *Surface) PresentPixels(_ []byte, _, _ uint32, _ []image.Rectangle) error {
	return errors.New("wgpu: PresentPixels is not supported on the Rust backend")
}

// WritePixels is not supported on Rust FFI backend.
// Software-backend extension for direct framebuffer write.
func (s *Surface) WritePixels(_ []byte, _, _ uint32) error {
	return errors.New("wgpu: WritePixels is not supported on the Rust backend")
}

// SetPresentsWithTransaction is a no-op on Rust backend.
// Core Animation transactions are macOS-only.
func (s *Surface) SetPresentsWithTransaction(_ bool) {}

// Release releases the surface. The root declaration layer drops the target
// ownership source after this returns.
func (s *Surface) Release() {
	if s == nil || s.released {
		return
	}
	s.released = true
	if s.r != nil {
		s.r.Release()
	}
}

// SurfaceTexture is a texture acquired from a surface for rendering.
type SurfaceTexture struct {
	r       *rwgpu.SurfaceTexture
	texture *resource.Texture
	surface *Surface
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
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*resource.TextureView, error) {
	if st == nil || st.texture == nil || st.texture.Rust() == nil {
		return nil, core.ErrReleased
	}

	var rDesc *rwgpu.TextureViewDescriptor
	if desc != nil {
		rDesc = &rwgpu.TextureViewDescriptor{
			Label:           desc.Label,
			Format:          desc.Format,
			Dimension:       desc.Dimension,
			Aspect:          rwgpu.TextureAspect(desc.Aspect),
			BaseMipLevel:    desc.BaseMipLevel,
			MipLevelCount:   desc.MipLevelCount,
			BaseArrayLayer:  desc.BaseArrayLayer,
			ArrayLayerCount: desc.ArrayLayerCount,
		}
	}

	rv, err := st.texture.Rust().CreateView(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create surface texture view: %w", err)
	}

	return resource.NewTextureView(rv, st.texture), nil
}
