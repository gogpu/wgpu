//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package queue

// The HAL conversions of the queue's copy descriptors (ADR-070 S9, the
// internal/encoder/descriptor_hal.go precedent). They live in their own native
// file: the HAL copy types are native-only, and the browser and rust queues
// convert straight to their backend objects.

import "github.com/gogpu/wgpu/internal/hal"

func (e Extent3D) toHAL() hal.Extent3D {
	return hal.Extent3D{Width: e.Width, Height: e.Height, DepthOrArrayLayers: e.DepthOrArrayLayers}
}

func (o Origin3D) toHAL() hal.Origin3D {
	return hal.Origin3D{X: o.X, Y: o.Y, Z: o.Z}
}

func (l ImageDataLayout) toHAL() hal.ImageDataLayout {
	return hal.ImageDataLayout{Offset: l.Offset, BytesPerRow: l.BytesPerRow, RowsPerImage: l.RowsPerImage}
}

func (i *ImageCopyTexture) toHAL() *hal.ImageCopyTexture {
	if i == nil || i.Texture == nil || i.Texture.ResolveHAL() == nil {
		return nil
	}
	return &hal.ImageCopyTexture{
		Texture:  i.Texture.ResolveHAL(),
		MipLevel: i.MipLevel,
		Origin:   i.Origin.toHAL(),
		Aspect:   i.Aspect,
	}
}
