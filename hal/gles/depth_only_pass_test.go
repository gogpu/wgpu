// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestBeginRenderPassDepthOnly checks that a pass without color attachments
// binds the depth texture's framebuffer with the view's mip level and layer
// and sets the viewport of that mip level. GLES recorded neither, so a
// shadow-map pass drew into the framebuffer bound before it.
func TestBeginRenderPassDepthOnly(t *testing.T) {
	tex := &Texture{
		target: gl.TEXTURE_2D_ARRAY,
		format: gputypes.TextureFormatDepth32Float,
		size:   hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 2},
	}
	view := &TextureView{texture: tex, baseMip: 1, mipCount: 1, baseLayer: 1, layerCount: 1}
	enc := &CommandEncoder{}
	if err := enc.BeginEncoding("depth-only"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	enc.BeginRenderPass(&hal.RenderPassDescriptor{
		DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
			View: view, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
		},
	})

	var bind *BindDepthStencilFBOCommand
	var viewport *SetViewportCommand
	for _, c := range enc.commands {
		switch c := c.(type) {
		case *BindDepthStencilFBOCommand:
			bind = c
		case *SetViewportCommand:
			viewport = c
		}
	}
	if bind == nil {
		t.Fatal("no BindDepthStencilFBOCommand recorded")
	}
	if bind.texture != tex || bind.mipLevel != 1 || bind.layer != 1 {
		t.Errorf("bind = texture %p mip %d layer %d, want %p mip 1 layer 1", bind.texture, bind.mipLevel, bind.layer, tex)
	}
	if viewport == nil {
		t.Fatal("no SetViewportCommand recorded")
	}
	if viewport.width != 4 || viewport.height != 4 {
		t.Errorf("viewport = %vx%v, want 4x4 (mip 1 of 8x8)", viewport.width, viewport.height)
	}
}
