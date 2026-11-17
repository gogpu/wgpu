//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file adapts the internal device to stencilblit.Device, the upload
// payload's creation seam (ADR-070 §15.1). The pre-move root adapter
// (stencil_host_native.go) converted the payload's internal descriptor mirrors
// to the public descriptors and called the public creation path; the adapter is
// now internal, and it builds the HAL descriptors the device's shape-1 creation
// methods take.
//
// The device cannot satisfy stencilblit.Device directly: the seam's Create*
// methods take the payload's internal descriptor mirrors, while the device's
// take HAL descriptors and internal handles (lead ruling 2). The conversion
// therefore lives here, not at a public boundary; every field is set explicitly
// and nothing is dropped.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// stencilHost is the payload's view of the device. The queue's and the
// encoder's seam methods hand the payload this adapter.
type stencilHost struct{ d *Device }

// Pipelines returns the blit pipelines of key and the layout of their bind
// groups from the device's cache, building them on first use.
func (h stencilHost) Pipelines(key stencilblit.Key) (stencilblit.Blit, error) {
	return h.d.pipelines(key)
}

// CreateTexture creates the staging texture the descriptor describes. The
// zero size members resolve to the WebGPU defaults exactly as the public path's
// normalizeTextureDescriptor resolved them: the payload always passes a real
// copy extent, and the guard keeps a zero layer count from reaching the HAL.
func (h stencilHost) CreateTexture(desc stencilblit.TextureDescriptor) (*resource.Texture, error) {
	size := desc.Size
	if size.Height == 0 {
		size.Height = 1
	}
	if size.DepthOrArrayLayers == 0 {
		size.DepthOrArrayLayers = 1
	}
	mipLevelCount := desc.MipLevelCount
	if mipLevelCount == 0 {
		mipLevelCount = 1
	}
	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}
	dimension := desc.Dimension
	if dimension == gputypes.TextureDimensionUndefined {
		dimension = gputypes.TextureDimension2D
	}
	return h.d.CreateTexture(&hal.TextureDescriptor{
		Label:         desc.Label,
		Size:          hal.Extent3D{Width: size.Width, Height: size.Height, DepthOrArrayLayers: size.DepthOrArrayLayers},
		MipLevelCount: mipLevelCount,
		SampleCount:   sampleCount,
		Dimension:     dimension,
		Format:        desc.Format,
		Usage:         desc.Usage,
	})
}

// CreateTextureView creates a view of texture.
func (h stencilHost) CreateTextureView(texture *resource.Texture, desc stencilblit.TextureViewDescriptor) (*resource.TextureView, error) {
	return h.d.CreateTextureView(texture, &hal.TextureViewDescriptor{
		Label:           desc.Label,
		Format:          desc.Format,
		Dimension:       desc.Dimension,
		Aspect:          desc.Aspect,
		BaseMipLevel:    desc.BaseMipLevel,
		MipLevelCount:   desc.MipLevelCount,
		BaseArrayLayer:  desc.BaseArrayLayer,
		ArrayLayerCount: desc.ArrayLayerCount,
	})
}

// CreateBuffer creates a buffer. A MappedAtCreation buffer is mapped when the
// call returns, as the public creation path's is.
func (h stencilHost) CreateBuffer(desc stencilblit.BufferDescriptor) (*resource.Buffer, error) {
	return h.d.CreateBuffer(&hal.BufferDescriptor{
		Label:            desc.Label,
		Size:             desc.Size,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	})
}

// CreateBindGroup creates a bind group over desc.Layout.
func (h stencilHost) CreateBindGroup(desc stencilblit.BindGroupDescriptor) (*resource.BindGroup, error) {
	entries := make([]BindGroupEntry, len(desc.Entries))
	for i, e := range desc.Entries {
		entries[i] = BindGroupEntry{
			Binding:     e.Binding,
			Buffer:      e.Buffer,
			Offset:      e.Offset,
			Size:        e.Size,
			TextureView: e.TextureView,
		}
	}
	return h.d.CreateBindGroup(&BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  desc.Layout,
		Entries: entries,
	})
}
