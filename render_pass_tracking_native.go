//go:build !rust && !(js && wasm)

package wgpu

// Staying-side shuttle (Prepare 2026-11-16). These declarations used to live
// in encoder_native.go, one of the three encoder context files that S4's Move
// relocates into internal/encoder. The staying white-box test
// surface_texture_lifetime_native_test.go names them, which is the planner's
// `unexported-boundary` refusal class: a staying file naming an unexported
// declaration that moves.
//
// With the declarations on the staying side the remaining direction is the
// tolerated one: the moving encoder files reference them, and the Move defers
// that through handRewrite. The Move relocates this file, its declarations and
// surface_texture_lifetime_native_test.go into internal/encoder.

import (
	"fmt"

	"github.com/gogpu/wgpu/internal/core"
)

func validateRenderPassTextureViews(desc *RenderPassDescriptor) error {
	if desc == nil {
		return nil
	}
	for _, attachment := range desc.ColorAttachments {
		if attachment.View != nil && attachment.View.impl.ResolveHAL() == nil {
			return fmt.Errorf("wgpu: BeginRenderPass: color attachment view is released: %w", core.ErrReleased)
		}
		if attachment.ResolveTarget != nil && attachment.ResolveTarget.impl.ResolveHAL() == nil {
			return fmt.Errorf("wgpu: BeginRenderPass: resolve target view is released: %w", core.ErrReleased)
		}
	}
	if attachment := desc.DepthStencilAttachment; attachment != nil && attachment.View != nil && attachment.View.impl.ResolveHAL() == nil {
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
			e.trackTextureView(attachment.View.impl)
		}
		if attachment.ResolveTarget != nil {
			e.trackTextureView(attachment.ResolveTarget.impl)
		}
	}
	if attachment := desc.DepthStencilAttachment; attachment != nil && attachment.View != nil {
		e.trackTextureView(attachment.View.impl)
	}
}
