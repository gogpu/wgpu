//go:build !rust && !(js && wasm)

package wgpu

import "github.com/gogpu/wgpu/internal/encoder"

// The root side of the render bundle execution path: the public
// RenderPassEncoder.ExecuteBundles method, which converts the public wrappers
// and delegates.
//
// The path lived here until ADR-070 S6. At S5 it was split — the conformance
// checks on this declaration layer, the scope merge and the replay in
// internal/encoder — because RenderBundle stayed root through S6 and the
// destination cannot name a root type. S6 ended that premise (RenderBundle
// moved into internal/encoder), so the whole path is the destination's now:
// the checks, the usage merge, the resource tracking, the replay and the
// pass-state reset, with every error text unchanged. This file keeps its name
// and carries the delegation.

// ExecuteBundles executes reusable render command bundles in this render pass.
// Each bundle's commands are replayed into the pass, its resources join the
// pass's usage scope, and the pass is left without a pipeline, bind groups,
// vertex buffers or index buffer: draws after it must set them again.
func (p *RenderPassEncoder) ExecuteBundles(bundles ...*RenderBundle) {
	if len(bundles) == 0 {
		p.impl.ExecuteBundles()
		return
	}
	impls := make([]*encoder.RenderBundle, len(bundles))
	for i, bundle := range bundles {
		if bundle != nil {
			impls[i] = bundle.impl
		}
	}
	p.impl.ExecuteBundles(impls...)
}

// Render bundle validation sentinels, aliased to the destination's values so
// that errors.Is matches across the boundary — the S5 ErrDraw* precedent. Only
// the two the root package still names are here: its zero-value test matches
// errBundleDevice, and the destination reports errBundleCommand for the same
// contract. The other three belong to the destination's own tests, which use
// the values directly.
var (
	errBundleCommand = encoder.ErrBundleCommand
	errBundleDevice  = encoder.ErrBundleDevice
)
