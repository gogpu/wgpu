// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package encoder

// The copy and render-pass-attachment descriptors of the encoder domain,
// re-typed to the internal resource handles (ADR-070 S4). The public
// descriptors in the root package carry the public wrappers (wgpu.Texture,
// wgpu.TextureView); the destination may name only internal types, so it
// declares the same shapes over *resource.Texture and friends. The root
// declaration layer converts at the call boundary, field by field: the
// wrappers are thin handles, so the conversion is the handle itself, never a
// copy of resource state.
//
// The pass descriptors themselves are per context (descriptor_native.go,
// descriptor_browser.go, descriptor_rust.go): the native one carries
// timestamp writes over a query set, which the browser and rust contexts have
// no type for, exactly as their public descriptors differ. The HAL conversion
// methods are native too (descriptor_hal.go): the browser and rust encoders
// convert straight to their backend objects.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/resource"
)

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	View          *resource.TextureView
	ResolveTarget *resource.TextureView
	// DepthSlice is the slice of a 3D view to render to (WebGPU
	// GPURenderPassColorAttachment.depthSlice).
	DepthSlice *uint32
	LoadOp     gputypes.LoadOp
	StoreOp    gputypes.StoreOp
	ClearValue gputypes.Color
}

// RenderPassDepthStencilAttachment describes a depth/stencil attachment.
type RenderPassDepthStencilAttachment struct {
	View              *resource.TextureView
	DepthLoadOp       gputypes.LoadOp
	DepthStoreOp      gputypes.StoreOp
	DepthClearValue   float32
	DepthReadOnly     bool
	StencilLoadOp     gputypes.LoadOp
	StencilStoreOp    gputypes.StoreOp
	StencilClearValue uint32
	StencilReadOnly   bool
}

// ImageCopyTexture describes a texture subresource and origin for copy
// operations.
type ImageCopyTexture struct {
	Texture  *resource.Texture
	MipLevel uint32
	Origin   gputypes.Origin3D
	Aspect   gputypes.TextureAspect
}

// TextureCopy describes a texture-to-texture copy region.
type TextureCopy struct {
	Source      ImageCopyTexture
	Destination ImageCopyTexture
	Size        gputypes.Extent3D
}

// TextureUsageTransition defines a texture usage state transition.
type TextureUsageTransition struct {
	OldUsage gputypes.TextureUsage
	NewUsage gputypes.TextureUsage
}

// TextureRange specifies a range of texture subresources.
type TextureRange struct {
	Aspect          gputypes.TextureAspect
	BaseMipLevel    uint32
	MipLevelCount   uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
}

// TextureBarrier defines a texture state transition for synchronization.
type TextureBarrier struct {
	Texture *resource.Texture
	Range   TextureRange
	Usage   TextureUsageTransition
}

// ImageDataLayout describes the layout of image data in a buffer.
type ImageDataLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

// BufferTextureCopy defines a buffer-texture copy region.
type BufferTextureCopy struct {
	BufferLayout ImageDataLayout
	TextureBase  ImageCopyTexture
	Size         gputypes.Extent3D
}
