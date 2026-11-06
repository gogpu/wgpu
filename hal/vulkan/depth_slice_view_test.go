// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestDepthSliceViewsDestroyedWithView checks the life of the 2D views a
// render pass on a depth slice of a 3D view uses: one per slice, reused,
// distinct from the 3D view, and destroyed with the 3D view together with the
// cached framebuffers that reference them.
func TestDepthSliceViewsDestroyedWithView(t *testing.T) {
	device, _, cleanup := tryCreateVulkanDevice(t)
	defer cleanup()
	d := device.(*Device)
	if !d.supports2DViewsOf3D {
		t.Skip("device has neither Vulkan 1.1 nor VK_KHR_maintenance1")
	}

	tex, err := d.CreateTexture(&hal.TextureDescriptor{
		Label:         "3d-target",
		Size:          hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 4},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension3D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer d.DestroyTexture(tex)
	halView, err := d.CreateTextureView(tex, &hal.TextureViewDescriptor{Dimension: gputypes.TextureViewDimension3D})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	view := halView.(*TextureView)

	slice1, err := view.depthSliceView(1)
	if err != nil {
		t.Fatalf("depthSliceView(1): %v", err)
	}
	again, err := view.depthSliceView(1)
	if err != nil || again != slice1 {
		t.Fatalf("depthSliceView(1) again = %v, %v; want the cached %v", again, err, slice1)
	}
	slice3, err := view.depthSliceView(3)
	if err != nil {
		t.Fatalf("depthSliceView(3): %v", err)
	}
	if slice1 == slice3 || slice1 == view.handle || slice3 == view.handle {
		t.Fatalf("slice views %v, %v and the 3D view %v must differ", slice1, slice3, view.handle)
	}

	cache := d.GetRenderPassCache()
	rp, err := cache.GetOrCreateRenderPass(RenderPassKey{
		ColorCount:  1,
		SampleCount: vk.SampleCountFlagBits(1),
		Colors: [hal.MaxColorAttachments]ColorAttachmentKeyEntry{{
			Format:      vk.FormatR8g8b8a8Unorm,
			LoadOp:      vk.AttachmentLoadOpClear,
			StoreOp:     vk.AttachmentStoreOpStore,
			FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
		}},
	})
	if err != nil {
		t.Fatalf("GetOrCreateRenderPass: %v", err)
	}
	fbKey := FramebufferKey{RenderPass: rp, Width: 8, Height: 8, ViewCount: 1}
	fbKey.Views[0] = slice1
	if _, err := cache.GetOrCreateFramebuffer(fbKey); err != nil {
		t.Fatalf("GetOrCreateFramebuffer: %v", err)
	}

	d.DestroyTextureView(view)
	if n := len(view.depthSlices.views); n != 0 {
		t.Errorf("%d depth-slice views left after DestroyTextureView", n)
	}
	cache.mu.RLock()
	_, stale := cache.framebuffers[fbKey]
	cache.mu.RUnlock()
	if stale {
		t.Error("the framebuffer of a destroyed depth-slice view is still cached")
	}
}
