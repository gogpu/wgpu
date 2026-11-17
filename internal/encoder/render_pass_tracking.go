//go:build !rust && !(js && wasm)

package encoder

// The render-pass attachment checks, relocated from the root package by S4's
// Move (the "staying-side shuttle" file render_pass_tracking_native.go of
// commit ae8631e): BeginRenderPass validates its attachment views and keeps
// their owners alive until the command buffer is spent.

import (
	"fmt"

	"github.com/gogpu/wgpu/internal/core"
)

func validateRenderPassTextureViews(desc *RenderPassDescriptor) error {
	if desc == nil {
		return nil
	}
	for _, attachment := range desc.ColorAttachments {
		if attachment.View != nil && attachment.View.ResolveHAL() == nil {
			return fmt.Errorf("wgpu: BeginRenderPass: color attachment view is released: %w", core.ErrReleased)
		}
		if attachment.ResolveTarget != nil && attachment.ResolveTarget.ResolveHAL() == nil {
			return fmt.Errorf("wgpu: BeginRenderPass: resolve target view is released: %w", core.ErrReleased)
		}
	}
	if attachment := desc.DepthStencilAttachment; attachment != nil && attachment.View != nil && attachment.View.ResolveHAL() == nil {
		return fmt.Errorf("wgpu: BeginRenderPass: depth/stencil attachment view is released: %w", core.ErrReleased)
	}
	return nil
}

func trackRenderPassTextureViews(e *CommandEncoder, desc *RenderPassDescriptor) {
	if e == nil || desc == nil {
		return
	}
	for _, attachment := range desc.ColorAttachments {
		if attachment.View != nil {
			e.trackTextureView(attachment.View)
		}
		if attachment.ResolveTarget != nil {
			e.trackTextureView(attachment.ResolveTarget)
		}
	}
	if attachment := desc.DepthStencilAttachment; attachment != nil && attachment.View != nil {
		e.trackTextureView(attachment.View)
	}
}
