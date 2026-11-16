// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package queue

// The copy descriptors of the queue domain, re-typed to the internal resource
// handles (ADR-070 S9). The public descriptors in the root package carry the
// public wrappers (wgpu.Texture); the destination may name only internal
// types, so it declares the same shapes over *resource.Texture. The root
// declaration layer converts at the call boundary, field by field: the
// wrappers are thin handles, so the conversion is the handle itself, never a
// copy of resource state.
//
// The HAL conversion (descriptor_hal.go) is native only, like the HAL copy
// types it builds: the browser and rust queues convert straight to their
// backend objects.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/resource"
)

// Extent3D is a 3D size.
type Extent3D struct {
	Width              uint32
	Height             uint32
	DepthOrArrayLayers uint32
}

// Origin3D is a 3D origin point.
type Origin3D struct {
	X uint32
	Y uint32
	Z uint32
}

// ImageDataLayout describes the layout of image data in a buffer.
type ImageDataLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

// ImageCopyTexture describes a texture subresource and origin for write
// operations.
type ImageCopyTexture struct {
	Texture  *resource.Texture
	MipLevel uint32
	Origin   Origin3D
	Aspect   gputypes.TextureAspect
}
