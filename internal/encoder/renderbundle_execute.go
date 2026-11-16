//go:build !rust && !(js && wasm)

package encoder

import (
	"fmt"

	"github.com/gogpu/wgpu/internal/core"
)

// This is the render bundle execution path of the render pass encoder: the
// bundle-conformance checks, the usage merge, the resource tracking, the
// replay and the pass-state reset, all against this package's RenderBundle.
//
// The path lived in the root package until ADR-070 S6. At S5 it was split —
// the checks on the root declaration layer, the scope merge and the replay
// here (ReplayBundle and ResetBundleState) — because RenderBundle stayed root
// through S6 and this package cannot name a root type. S6 ends that premise:
// RenderBundle lives here now, so the whole path does, and the root wrapper
// delegates. The second reason is the one the S5 fix 56e4471 pinned for the
// reverse direction: a check or a replay that reads the bundle's resolved
// state cannot be written in a declaration layer once the type is
// destination-resident.
//
// Matches Rust wgpu-core command/render.rs execute_bundle and
// command/bundle.rs RenderBundle::execute.

// ExecuteBundles executes reusable render command bundles in this render pass.
// Each bundle's commands are replayed into the pass, its resources join the
// pass's usage scope, and the pass is left without a pipeline, bind groups,
// vertex buffers or index buffer: draws after it must set them again.
//
// The public wrapper delegates here; every error text below is the pre-move
// one.
func (p *RenderPassEncoder) ExecuteBundles(bundles ...*RenderBundle) {
	for _, bundle := range bundles {
		if err := p.executeBundle(bundle); err != nil {
			p.encoder.SetError(fmt.Errorf("wgpu: RenderPass.ExecuteBundles: %w", err))
			break
		}
	}
	if len(bundles) > 0 {
		p.resetBundleState()
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
	// The pre-move order: merge, track, replay. Merge errors and replay
	// errors keep their pre-move wrapping.
	if err := p.core.MergeRenderBundle(bundle.scope); err != nil {
		return fmt.Errorf("bundle %q: %w", bundle.label, err)
	}
	for _, ref := range bundle.refs {
		p.encoder.TrackRef(ref)
	}
	for _, group := range bundle.groups {
		p.encoder.TrackBindGroup(group)
	}
	for _, buffer := range bundle.buffers {
		p.encoder.TrackBuffer(buffer)
	}
	if err := bundle.replay(p.core.RawPass()); err != nil {
		return fmt.Errorf("bundle %q: %w", bundle.label, err)
	}
	return nil
}

// checkBundle checks that a bundle can run in this pass: same device, the
// pass's attachment formats and sample count, and read-only where the pass's
// depth/stencil is. Matches the checks of Rust render.rs execute_bundle.
func (p *RenderPassEncoder) checkBundle(bundle *RenderBundle) error {
	if bundle == nil {
		return fmt.Errorf("%w: render bundle is nil", ErrBundleCommand)
	}
	if bundle.released.Load() {
		return fmt.Errorf("bundle %q: %w", bundle.label, core.ErrReleased)
	}
	if bundle.bundleContents == nil {
		return fmt.Errorf("%w: render bundle %q was not created by RenderBundleEncoder.Finish", ErrBundleCommand, bundle.label)
	}
	// The bundle and the pass hold the same core.DeviceView the pass's core
	// encoder records against: one core device per device, so pointer identity
	// is device identity (the pre-move comparison of the two *Device).
	if bundle.device.Core() != p.core.Device() {
		return fmt.Errorf("bundle %q: %w", bundle.label, ErrBundleDevice)
	}
	if ctx := p.core.PassContext(); ctx != nil {
		if err := ctx.CheckCompatible(&bundle.target.context, bundle.label); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.label, err)
		}
	}
	if (p.depthReadOnly && !bundle.target.depthReadOnly) || (p.stencilReadOnly && !bundle.target.stencilReadOnly) {
		return fmt.Errorf("bundle %q: %w", bundle.label, ErrBundleReadOnly)
	}
	return nil
}
