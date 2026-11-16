//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package stencilblit

// This file declares the stencil upload payload's contract with its callers.
// The interfaces are the payload's own, so they live with it: this package
// owns the rule and the payload, the queue and the encoder record the commands
// through these views (ADR-070 §15.1 — the payload only; golden rows
// queue/encoder -> stencilblit, no edge back).
//
// The placement was other way round from the S4 Move until S9: the interfaces
// were declared here because internal/encoder did not exist before its Move,
// and the Move relocated them there. S9's hand-back returns them — the
// encoder imports this package for its recorder adapter (stencil_recorder.go),
// which is the already-golden encoder -> stencilblit row, and this package
// imports neither the root package nor internal/encoder.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

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
// draw verbs are the five RecordLayer drives over the internal pipeline type,
// plus End: a value of this interface type is what BeginLayerPass returns and
// what the caller ends after RecordLayer returns.
type PassRecorder interface {
	SetPipeline(pl *pipeline.RenderPipeline)
	SetStencilReference(reference uint32)
	SetScissorRect(x, y, width, height uint32)
	Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32)
	// End ends the render pass.
	End() error
}

// Releasable is a resource that owns a reference and releases it on Release.
//
// It is also the encoder's own internal-resource contract: the resources the
// frontend creates for its commands (the stencil blit's staging texture,
// views, buffers and bind groups) have no application owner, so the command
// buffer releases them once it is spent. The encoder keeps its own list of
// these when it lowers a stencil copy, so the type lives here, with the
// payload that produces most of them.
type Releasable interface {
	Release()
}
