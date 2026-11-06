//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// depthSliceViews creates and keeps the 2D views of single depth slices of a
// 3D texture view. A framebuffer attachment must be a 2D view, so a render
// pass on depth slice z of a 3D view (GPURenderPassColorAttachment.depthSlice)
// attaches a 2D view of array layer z of the 2D_ARRAY_COMPATIBLE image, at the
// view's base mip level.
//
// Rust wgpu-hal creates these views per command encoder
// (CommandEncoder::make_temp_texture_view, keyed by texture, format, mip level
// and depth slice) and destroys them when the encoder is dropped. Here they
// belong to the 3D view instead, created on first use and destroyed with it:
// an encoder hands its command pool to the CommandBuffer it finishes into, so
// it has no point after GPU completion of its own at which to destroy them,
// while the view's destruction is already deferred until the GPU is done with
// every submission that used it. A view keeps at most one 2D view per depth
// slice of its base mip level.
type depthSliceViews struct {
	format  vk.Format
	aspect  vk.ImageAspectFlags
	baseMip uint32

	mu    sync.Mutex
	views map[uint32]vk.ImageView
}

// depthSliceView returns the 2D view of depth slice slice of v, which must be
// a 3D view, creating it on first use.
func (v *TextureView) depthSliceView(slice uint32) (vk.ImageView, error) {
	s := v.depthSlices
	if s == nil {
		return 0, fmt.Errorf("vulkan: depth slice %d set on a view that is not 3D", slice)
	}
	d := v.device
	if d == nil || !d.supports2DViewsOf3D {
		return 0, fmt.Errorf("vulkan: rendering into a depth slice needs Vulkan 1.1 or VK_KHR_maintenance1")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.views[slice]; ok {
		return h, nil
	}
	info := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    v.image,
		ViewType: vk.ImageViewType2d,
		Format:   s.format,
		Components: vk.ComponentMapping{
			R: vk.ComponentSwizzleIdentity,
			G: vk.ComponentSwizzleIdentity,
			B: vk.ComponentSwizzleIdentity,
			A: vk.ComponentSwizzleIdentity,
		},
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     s.aspect,
			BaseMipLevel:   s.baseMip,
			LevelCount:     1,
			BaseArrayLayer: slice,
			LayerCount:     1,
		},
	}
	var h vk.ImageView
	if result := vkCreateImageView(d.cmds, d.handle, &info, nil, &h); result != vk.Success {
		return 0, fmt.Errorf("vulkan: vkCreateImageView for depth slice %d failed: %d", slice, result)
	}
	d.setObjectName(vk.ObjectTypeImageView, uint64(h), fmt.Sprintf("DepthSlice(%d)", slice))
	if s.views == nil {
		s.views = make(map[uint32]vk.ImageView)
	}
	s.views[slice] = h
	return h, nil
}

// destroyDepthSliceViews destroys the depth-slice views of v, first dropping
// the cached framebuffers that reference them.
func (d *Device) destroyDepthSliceViews(v *TextureView) {
	s := v.depthSlices
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for slice, h := range s.views {
		if d.renderPassCache != nil {
			d.renderPassCache.InvalidateFramebuffer(h)
		}
		vkDestroyImageView(d.cmds, d.handle, h, nil)
		delete(s.views, slice)
	}
}
