//go:build rust

package wgpu

// Staying-side shuttle (Prepare 2026-11-16). These declarations used to live
// in encoder_rust.go, one of the three encoder context files that S4's Move
// relocates into internal/encoder. The staying white-box test
// encoder_depth_slice_rust_test.go names them, which is the planner's
// `unexported-boundary` refusal class: a staying file naming an unexported
// declaration that moves.
//
// With the declarations on the staying side the remaining direction is the
// tolerated one: the moving encoder files reference them, and the Move defers
// that through handRewrite. The Move relocates this file, its declarations and
// encoder_depth_slice_rust_test.go into internal/encoder.

import (
	"errors"
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// --- Render pass descriptor conversion ---

// errRustDepthSliceUnmapped is returned for a color attachment with a
// DepthSlice under -tags rust: go-webgpu's RenderPassColorAttachment has no
// depthSlice field and always sends WGPU_DEPTH_SLICE_UNDEFINED, so the slice
// would be dropped silently.
var errRustDepthSliceUnmapped = errors.New("wgpu: RenderPassColorAttachment.DepthSlice is not supported with -tags rust: go-webgpu does not map depthSlice yet")

func convertRenderPassDescriptorRust(desc *RenderPassDescriptor) (*rwgpu.RenderPassDescriptor, error) {
	if desc == nil {
		return &rwgpu.RenderPassDescriptor{}, nil
	}

	rDesc := &rwgpu.RenderPassDescriptor{
		Label: desc.Label,
	}

	rDesc.ColorAttachments = make([]rwgpu.RenderPassColorAttachment, len(desc.ColorAttachments))
	for i, ca := range desc.ColorAttachments {
		if ca.DepthSlice != nil {
			return nil, fmt.Errorf("color attachment %d: %w", i, errRustDepthSliceUnmapped)
		}
		att := rwgpu.RenderPassColorAttachment{
			LoadOp:  ca.LoadOp,
			StoreOp: ca.StoreOp,
			ClearValue: rwgpu.Color{
				R: ca.ClearValue.R,
				G: ca.ClearValue.G,
				B: ca.ClearValue.B,
				A: ca.ClearValue.A,
			},
		}
		if ca.View != nil {
			att.View = ca.View.impl.Rust()
		}
		if ca.ResolveTarget != nil {
			att.ResolveTarget = ca.ResolveTarget.impl.Rust()
		}
		rDesc.ColorAttachments[i] = att
	}

	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		rDSA := &rwgpu.RenderPassDepthStencilAttachment{
			DepthLoadOp:       dsa.DepthLoadOp,
			DepthStoreOp:      dsa.DepthStoreOp,
			DepthClearValue:   dsa.DepthClearValue,
			DepthReadOnly:     dsa.DepthReadOnly,
			StencilLoadOp:     dsa.StencilLoadOp,
			StencilStoreOp:    dsa.StencilStoreOp,
			StencilClearValue: dsa.StencilClearValue,
			StencilReadOnly:   dsa.StencilReadOnly,
		}
		if dsa.View != nil {
			rDSA.View = dsa.View.impl.Rust()
		}
		rDesc.DepthStencilAttachment = rDSA
	}

	return rDesc, nil
}
