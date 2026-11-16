//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"
)

// This is the root-side render bundle execution path: the public
// RenderPassEncoder.ExecuteBundles method, the bundle-conformance checks and
// the wiring between them and the destination's replay step.
//
// S6 shape (2026-11-16). At S5 this path was split — the checks on the root
// declaration layer, the scope merge and the replay in internal/encoder
// (ReplayBundle and ResetBundleState) — because RenderBundle stayed root
// through S6 and the destination cannot name a root type. That premise ends
// where it was made: S6 moves RenderBundle into internal/encoder, so the S6
// Move re-homes this whole path (checks, tracking, replay, reset) onto the
// destination's own bundle type, and this file becomes the public wrapper's
// delegation to it. The second reason is the one the S5 fix 56e4471 pinned for
// the reverse direction: the replay and the checks read the bundle's resolved
// state, which a root declaration layer cannot name once the type is
// destination-resident.
//
// Until that Move this file is the pre-move implementation, behavior
// unchanged.

// ExecuteBundles executes reusable render command bundles in this render pass.
// Each bundle's commands are replayed into the pass, its resources join the
// pass's usage scope, and the pass is left without a pipeline, bind groups,
// vertex buffers or index buffer: draws after it must set them again.
//
// The destination supplies the two bundle-free halves — ReplayBundle (scope
// merge, the replay callback, the pre-move error wrapping) and
// ResetBundleState — because it cannot name *RenderBundle (see the file
// comment).
func (p *RenderPassEncoder) ExecuteBundles(bundles ...*RenderBundle) {
	for _, bundle := range bundles {
		if err := p.executeBundle(bundle); err != nil {
			p.encoder.setError(fmt.Errorf("wgpu: RenderPass.ExecuteBundles: %w", err))
			break
		}
	}
	if len(bundles) > 0 {
		p.impl.ResetBundleState()
	}
}

// executeBundle checks that a bundle fits this pass, merges its usages into
// the pass's usage scope, keeps its resources alive until the GPU completes
// the submission, and replays its commands into the backend pass.
// Matches Rust wgpu-core command/render.rs execute_bundle and
// command/bundle.rs RenderBundle::execute.
func (p *RenderPassEncoder) executeBundle(bundle *RenderBundle) error {
	if err := p.checkBundle(bundle); err != nil {
		return err
	}
	// The scope merge and the replay are the destination's (it holds the core
	// pass); resource tracking stays here, where the wrappers convert without
	// an allocation, in the pre-move order: merge, track, replay.
	return p.impl.ReplayBundle(bundle.scope, bundle.label, func() {
		for _, ref := range bundle.refs {
			p.encoder.trackRef(ref)
		}
		for _, group := range bundle.groups {
			p.encoder.trackBindGroup(group)
		}
		for _, buffer := range bundle.buffers {
			p.encoder.trackBuffer(buffer.impl)
		}
	}, bundle.replay)
}

// checkBundle checks that a bundle can run in this pass: same device, the
// pass's attachment formats and sample count, and read-only where the pass's
// depth/stencil is. Matches the checks of Rust render.rs execute_bundle.
func (p *RenderPassEncoder) checkBundle(bundle *RenderBundle) error {
	if bundle == nil {
		return fmt.Errorf("%w: render bundle is nil", errBundleCommand)
	}
	if bundle.released.Load() {
		return fmt.Errorf("bundle %q: %w", bundle.label, ErrReleased)
	}
	if bundle.bundleContents == nil {
		return fmt.Errorf("%w: render bundle %q was not created by RenderBundleEncoder.Finish", errBundleCommand, bundle.label)
	}
	// The bundle and the pass hold the same core.DeviceView the pass's core
	// encoder records against: one core device per device, so pointer identity
	// is device identity (the pre-move comparison of the two *Device).
	if bundle.device.Core() != p.core.Device() {
		return fmt.Errorf("bundle %q: %w", bundle.label, errBundleDevice)
	}
	if ctx := p.core.PassContext(); ctx != nil {
		if err := ctx.CheckCompatible(&bundle.target.context, bundle.label); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.label, err)
		}
	}
	if (p.depthReadOnly && !bundle.target.depthReadOnly) || (p.stencilReadOnly && !bundle.target.stencilReadOnly) {
		return fmt.Errorf("bundle %q: %w", bundle.label, errBundleReadOnly)
	}
	return nil
}
