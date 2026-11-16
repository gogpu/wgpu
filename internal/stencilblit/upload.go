//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package stencilblit

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// This file declares the contract between the stencil upload payload (the root
// package, until S6 of ADR-070 B moves it here) and the command encoder that
// records its passes.
//
// The interfaces live in this package only until internal/encoder exists: S4's
// Move relocates them there (tmp/plans/s4/RECORD.md §9a, lead-approved
// 2026-11-16), which drops the internal/resource, internal/core and
// internal/pipeline imports below — this package then imports gputypes alone
// again. Until then the payload is the only user, and the encoder implements
// them through an adapter.
//
// Condition of the approved seam: every type named here is internal
// (internal/resource, internal/core, internal/pipeline) or a primitive. A root
// type in a signature would be the wgpu -> internal/encoder -> wgpu cycle under
// another name; the root host adapts at the call boundary instead.

// Upload is one prepared upload into the stencil aspect of a combined
// depth-stencil texture: the staging texture the caller fills, and the blit
// that draws it into the destination.
type Upload interface {
	// Staging returns the R8Uint texture of the copy extent that holds the
	// stencil bytes before the blit.
	Staging() *resource.Texture
	// Record records the blit's render passes into recorder's command stream.
	Record(recorder Recorder) error
}

// Recorder is the command encoder the blit records into, as the blit needs it.
type Recorder interface {
	// Own hands a resource the blit created to the encoder, which releases it
	// once the command buffer it belongs to is spent (submitted or released).
	Own(r Releasable)
	// BeginLayerPass opens the render pass of one destination layer, binds
	// group at bind group index 0, and returns the pass's command recorder.
	BeginLayerPass(desc *core.RenderPassDescriptor, group *resource.BindGroup) (PassRecorder, error)
}

// PassRecorder records the commands of one blit layer and ends its pass. Its
// draw verbs are Pass[*pipeline.RenderPipeline], the pipeline type the blit's
// cache holds.
type PassRecorder interface {
	Pass[*pipeline.RenderPipeline]
	// End ends the render pass.
	End() error
}

// Releasable is a resource that owns a reference and releases it on Release.
type Releasable interface {
	Release()
}
