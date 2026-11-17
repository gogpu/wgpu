// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

// The descriptors of the surface domain (ADR-070 S10). The public ones in the
// root package carry the public wrappers; the destination may name only
// internal types, so it declares the same shapes over gputypes values. The
// root declaration layer converts at the call boundary, field by field, and
// the HAL conversion (native) is built from the mirror inside the
// implementation, the internal/queue/descriptor.go pattern of S9.

import "github.com/gogpu/gputypes"

// SurfaceConfiguration is the surface domain's mirror of the public surface
// configuration: the WebGPU fields a surface needs to configure itself.
type SurfaceConfiguration struct {
	// Width and Height are the surface extent in physical pixels.
	Width  uint32
	Height uint32

	// Format is the texture format of surface textures.
	Format gputypes.TextureFormat

	// Usage specifies how surface textures will be used.
	Usage gputypes.TextureUsage

	// PresentMode controls presentation timing.
	PresentMode gputypes.PresentMode

	// AlphaMode controls alpha compositing.
	AlphaMode gputypes.CompositeAlphaMode
}

// TextureViewDescriptor is the surface domain's mirror of the public texture
// view descriptor: the fields a view of a surface texture resolves.
type TextureViewDescriptor struct {
	// Label is an optional debug name.
	Label string

	// Format is the view format; TextureFormatUndefined inherits the texture's.
	Format gputypes.TextureFormat

	// Dimension is the view dimension; TextureViewDimensionUndefined inherits
	// the texture's.
	Dimension gputypes.TextureViewDimension

	// Aspect selects the color, depth or stencil plane to view.
	Aspect gputypes.TextureAspect

	// BaseMipLevel is the first mip level in the view.
	BaseMipLevel uint32

	// MipLevelCount is the number of mip levels (0 means all remaining).
	MipLevelCount uint32

	// BaseArrayLayer is the first array layer in the view.
	BaseArrayLayer uint32

	// ArrayLayerCount is the number of array layers (0 means all remaining).
	ArrayLayerCount uint32
}
