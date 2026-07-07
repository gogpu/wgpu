// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Surface implements hal.Surface for Metal using CAMetalLayer.
type Surface struct {
	layer       ID // CAMetalLayer
	device      *Device
	format      gputypes.TextureFormat
	width       uint32
	height      uint32
	presentMode hal.PresentMode
	// presentsWithTransaction enables Core Animation transaction-based present.
	// Required for smooth live window resize on macOS (wgpu #3756, Flutter/Skia).
	presentsWithTransaction bool
	configured              bool
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

	// Skip no-op reconfigure (present mode / vsync updates still run below).
	sizeChanged := s.configured && (s.width != config.Width || s.height != config.Height)
	if s.configured && !sizeChanged &&
		s.format == config.Format && s.device == mtlDevice {
		s.presentMode = config.PresentMode
		vsync := config.PresentMode == hal.PresentModeFifo
		msgSendVoid(s.layer, Sel("setDisplaySyncEnabled:"), argBool(vsync))
		return nil
	}

	// Rust wgpu Surface::configure waits for GPU idle before recreating the
	// swapchain. On Metal, setDrawableSize allocates a new IOSurface drawable
	// pool; without draining in-flight presents, rapid live-resize accumulates
	// gigabytes of unreleased IOSurfaces (imgui #2910, wgpu #9021).
	//
	// Skipped in transaction-present mode (live resize): waitUntilScheduled in
	// Queue.Present already serializes the drawable swap with the CA
	// transaction, and blocking nextDrawable (allowsNextDrawableTimeout=false)
	// paces pool allocation. A per-tick WaitIdle here would only add drag jank.
	if sizeChanged && s.device != nil && !s.presentsWithTransaction {
		_ = s.device.WaitIdle()
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
	framebufferOnly := config.Usage&gputypes.TextureUsageStorageBinding == 0
	msgSendVoid(s.layer, Sel("setFramebufferOnly:"), argBool(framebufferOnly))

	// Set maximum drawable count for frame latency control.
	// Rust wgpu: set_maximum_drawable_count(maximum_frame_latency + 1).
	// Default maximum_frame_latency=2 → drawable_count=3 (Metal default).
	_ = MsgSend(s.layer, Sel("setMaximumDrawableCount:"), uintptr(3))

	// Disable the 1-second timeout on nextDrawable (Rio/zed/ghostty pattern).
	// With the timeout enabled, nextDrawable returns nil under drawable-pool
	// pressure (rapid live resize); the caller then treats it as an acquire
	// failure and reconfigures the surface, allocating yet another IOSurface
	// pool — a feedback loop that leaks gigabytes during drag. With the
	// timeout disabled, nextDrawable blocks until a drawable is free, acting
	// as natural backpressure that paces the render loop to what Core
	// Animation can actually release.
	msgSendVoid(s.layer, Sel("setAllowsNextDrawableTimeout:"), argBool(false))

	// Set present mode
	s.presentMode = config.PresentMode
	// VSync is controlled by displaySyncEnabled (available since macOS 10.13)
	vsync := config.PresentMode == hal.PresentModeFifo
	msgSendVoid(s.layer, Sel("setDisplaySyncEnabled:"), argBool(vsync))

	// presentsWithTransaction default is false: normal rendering presents via
	// [commandBuffer presentDrawable:] from the render goroutine, and enabling
	// transaction present outside a live main-thread CA transaction defers every
	// frame until the next AppKit event (blank window).
	//
	// During macOS live resize the app layer flips this to true via
	// SetPresentsWithTransaction (Flutter/wgpu #3756 pattern): the main thread
	// blocks inside windowDidResize: waiting for the render thread, whose
	// commit + waitUntilScheduled + [drawable present] then lands inside the
	// open CA transaction — the drawable swap is atomic with the window resize,
	// so Core Animation neither stretches the old frame nor stockpiles
	// undisplayed drawables (the source of multi-GB IOSurface leaks).
	//
	// Reapply the current value on every Configure because setDrawableSize:
	// resets internal CAMetalLayer state on some macOS versions.
	msgSendVoid(s.layer, Sel("setPresentsWithTransaction:"), argBool(s.presentsWithTransaction))

	// Anchor the drawable content to the top-left of the layer instead of
	// stretching it to fill the layer's bounds.
	//
	// By default CAMetalLayer uses kCAGravityResize which scales (stretches)
	// the last rendered frame to fill the entire window as AppKit resizes it,
	// producing visible distortion during live drag. kCAGravityTopLeft leaves
	// the content at its natural pixel size and reveals the layer background
	// (black) in any newly-added margin — a clean letterbox effect during drag
	// with no geometry distortion. The post-drag resize then fills the window.
	//
	// This must be set on every Configure call because setDrawableSize: resets
	// internal CAMetalLayer state on some macOS versions.
	if gravity := NSString("topLeft"); gravity != 0 {
		msgSendVoid(s.layer, Sel("setContentsGravity:"), argPointer(uintptr(gravity)))
		Release(gravity)
	}

	hal.Logger().Info("metal: surface configured",
		"width", config.Width,
		"height", config.Height,
		"format", config.Format,
		"presentMode", config.PresentMode,
		"vsync", vsync,
	)

	s.configured = true
	return nil
}

// Unconfigure removes surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	hal.Logger().Debug("metal: surface unconfigured")
	// Nothing to release for Metal layer
	s.device = nil
}

// SetPresentsWithTransaction toggles Core Animation transaction-based present.
//
// Enable during macOS live resize so [drawable present] (issued after
// waitUntilScheduled in Queue.Present) commits atomically with the window
// resize CA transaction — no stretch, no drawable stockpiling. Disable for
// normal rendering where the render goroutine presents outside any
// main-thread transaction.
func (s *Surface) SetPresentsWithTransaction(enabled bool) {
	if s.presentsWithTransaction == enabled {
		return
	}
	s.presentsWithTransaction = enabled
	if s.layer != 0 {
		msgSendVoid(s.layer, Sel("setPresentsWithTransaction:"), argBool(enabled))
	}
	hal.Logger().Debug("metal: presentsWithTransaction", "enabled", enabled)
}

// AcquireTexture acquires the next surface texture for rendering.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	pool := NewAutoreleasePool()
	defer pool.Drain()

	drawable := MsgSend(s.layer, Sel("nextDrawable"))
	if drawable == 0 {
		hal.Logger().Error("metal: nextDrawable failed", "layer", s.layer)
		return nil, fmt.Errorf("metal: failed to get next drawable")
	}
	Retain(drawable)

	texture := MsgSend(drawable, Sel("texture"))
	if texture == 0 {
		hal.Logger().Error("metal: drawable has no texture", "drawable", drawable)
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
		dimension:  gputypes.TextureDimension2D,
		usage:      gputypes.TextureUsageRenderAttachment,
		device:     s.device,
		isExternal: true,
	}

	hal.Logger().Debug("metal: surface texture acquired",
		"drawable", drawable,
		"texture", texture,
		"width", s.width,
		"height", s.height,
	)

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
		st.releaseAcquired()
	}
}

// ActualExtent returns the configured surface dimensions.
// Metal does not clamp the extent, so these always match the requested values.
// Returns (0, 0) if the surface is not configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	return s.width, s.height
}

// Destroy releases the surface.
func (s *Surface) Destroy() {
	hal.Logger().Debug("metal: surface destroyed")
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

// CurrentUsage returns 0 — Metal has no explicit resource state tracking.
func (st *SurfaceTexture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (st *SurfaceTexture) AddPendingRef()                      {}
func (st *SurfaceTexture) DecPendingRef()                      {}

// Destroy releases the surface texture.
func (st *SurfaceTexture) Destroy() {
	st.releaseAcquired()
}

// releaseAcquired balances the two Retains taken in AcquireTexture: the
// MTLTexture and the CAMetalDrawable. The texture retain must be released
// here — Device.DestroyTexture deliberately skips isExternal textures, so
// without this the drawable's IOSurface stays pinned forever. One leaked
// retain per presented frame accumulates gigabytes (observed: >1000 live
// IOSurfaces / ~9 GB footprint after a resize-heavy session).
func (st *SurfaceTexture) releaseAcquired() {
	if st.texture != nil {
		if st.texture.raw != 0 {
			Release(st.texture.raw)
			st.texture.raw = 0
		}
		st.texture.device = nil
		st.texture = nil
	}
	if st.drawable != 0 {
		Release(st.drawable)
		st.drawable = 0
	}
}

// NativeHandle returns the raw MTLTexture handle.
func (st *SurfaceTexture) NativeHandle() uintptr {
	if st.texture != nil {
		return uintptr(st.texture.raw)
	}
	return 0
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
	msgSendVoid(obj, sel, argStruct(size, cgSizeType))
}
