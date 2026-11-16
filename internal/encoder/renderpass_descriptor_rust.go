//go:build rust

package encoder

// The rust render-pass descriptor conversion, relocated from the root package
// by S4's Move (the "staying-side shuttle" file encoder_seams_rust.go of
// commit ae8631e).

import (
	"errors"
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

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
			att.View = ca.View.Rust()
		}
		if ca.ResolveTarget != nil {
			att.ResolveTarget = ca.ResolveTarget.Rust()
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
			rDSA.View = dsa.View.Rust()
		}
		rDesc.DepthStencilAttachment = rDSA
	}

	return rDesc, nil
}
