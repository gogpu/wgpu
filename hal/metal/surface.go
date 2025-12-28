// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// Surface implements hal.Surface for Metal using CAMetalLayer.
type Surface struct {
	layer       ID // CAMetalLayer
	device      *Device
	format      types.TextureFormat
	width       uint32
	height      uint32
	presentMode hal.PresentMode
}

// Configure configures the surface for presentation.
//
// Returns hal.ErrZeroArea if width or height is zero.
// This commonly happens when the window is minimized or not yet fully visible.
// Wait until the window has valid dimensions before calling Configure again.
func (s *Surface) Configure(device hal.Device, config *hal.SurfaceConfiguration) error {
	// Validate dimensions first (before any side effects).
	// This matches wgpu-core behavior which returns ConfigureSurfaceError::ZeroArea.
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	mtlDevice, ok := device.(*Device)
	if !ok {
		return fmt.Errorf("metal: invalid device type")
	}
	s.device = mtlDevice

	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Set device
	_ = MsgSend(s.layer, Sel("setDevice:"), uintptr(mtlDevice.raw))

	// Set pixel format
	s.format = config.Format
	pixelFormat := textureFormatToMTL(config.Format)
	_ = MsgSend(s.layer, Sel("setPixelFormat:"), uintptr(pixelFormat))

	// Set size
	s.width = config.Width
	s.height = config.Height
	size := CGSize{Width: CGFloat(config.Width), Height: CGFloat(config.Height)}
	msgSendCGSize(s.layer, Sel("setDrawableSize:"), size)

	// Configure framebuffer only if not using storage binding
	framebufferOnly := config.Usage&types.TextureUsageStorageBinding == 0
	var fbOnlyVal uintptr
	if framebufferOnly {
		fbOnlyVal = 1
	}
	_ = MsgSend(s.layer, Sel("setFramebufferOnly:"), fbOnlyVal)

	// Set present mode
	s.presentMode = config.PresentMode
	// VSync is controlled by displaySyncEnabled (available since macOS 10.13)
	vsync := config.PresentMode == hal.PresentModeFifo
	var vsyncVal uintptr
	if vsync {
		vsyncVal = 1
	}
	_ = MsgSend(s.layer, Sel("setDisplaySyncEnabled:"), vsyncVal)

	return nil
}

// Unconfigure removes surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	// Nothing to release for Metal layer
	s.device = nil
}

// AcquireTexture acquires the next surface texture for rendering.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	pool := NewAutoreleasePool()
	defer pool.Drain()

	drawable := MsgSend(s.layer, Sel("nextDrawable"))
	if drawable == 0 {
		return nil, fmt.Errorf("metal: failed to get next drawable")
	}
	Retain(drawable)

	texture := MsgSend(drawable, Sel("texture"))
	if texture == 0 {
		Release(drawable)
		return nil, fmt.Errorf("metal: drawable has no texture")
	}
	Retain(texture)

	mtlTexture := &Texture{
		raw:        texture,
		format:     s.format,
		width:      s.width,
		height:     s.height,
		depth:      1,
		mipLevels:  1,
		samples:    1,
		dimension:  types.TextureDimension2D,
		usage:      types.TextureUsageRenderAttachment,
		device:     s.device,
		isExternal: true,
	}

	return &hal.AcquiredSurfaceTexture{
		Texture: &SurfaceTexture{
			texture:  mtlTexture,
			drawable: drawable,
		},
		Suboptimal: false,
	}, nil
}

// DiscardTexture discards a surface texture without presenting it.
func (s *Surface) DiscardTexture(tex hal.SurfaceTexture) {
	if st, ok := tex.(*SurfaceTexture); ok {
		if st.drawable != 0 {
			Release(st.drawable)
			st.drawable = 0
		}
	}
}

// Destroy releases the surface.
func (s *Surface) Destroy() {
	if s.layer != 0 {
		Release(s.layer)
		s.layer = 0
	}
}

// SurfaceTexture wraps a Metal drawable texture.
type SurfaceTexture struct {
	texture  *Texture
	drawable ID // id<CAMetalDrawable>
}

// Destroy releases the surface texture.
func (st *SurfaceTexture) Destroy() {
	if st.texture != nil {
		st.texture.Destroy()
		st.texture = nil
	}
	if st.drawable != 0 {
		Release(st.drawable)
		st.drawable = 0
	}
}

// Drawable returns the drawable ID.
// This is used by the native backend to attach the drawable to a command buffer.
func (st *SurfaceTexture) Drawable() ID {
	return st.drawable
}

// msgSendCGSize sends an Objective-C message with a CGSize argument.
func msgSendCGSize(obj ID, sel SEL, size CGSize) {
	if obj == 0 {
		return
	}
	sizePtr := (*[2]uintptr)(unsafe.Pointer(&size))
	_ = MsgSend(obj, sel, sizePtr[0], sizePtr[1])
}
