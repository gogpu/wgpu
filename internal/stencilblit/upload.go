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
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// Device is the upload payload's creation seam: the pipelines of a Key and the
// four resource creations the blit's commands need. The pipeline cache and the
// label of every object stay in the root package (the cache is device-owned
// state, core.StencilBlitResources, and holds the public wrappers that own the
// pipelines' references); this seam receives the internal handles and the
// stored blit state — the nine pipelines and the bind group layout their
// groups are built on.
type Device interface {
	// Pipelines returns the blit pipelines of key and the layout of their bind
	// groups, building them on first use. The cache belongs to the device and
	// is disposed with it.
	Pipelines(key Key) (Blit, error)
	// CreateTexture creates a texture. The staging texture of an upload is
	// created through it.
	CreateTexture(desc TextureDescriptor) (*resource.Texture, error)
	// CreateTextureView creates a view of texture.
	CreateTextureView(texture *resource.Texture, desc TextureViewDescriptor) (*resource.TextureView, error)
	// CreateBuffer creates a GPU buffer. A MappedAtCreation buffer is mapped
	// when the call returns, as the public creation path's is.
	CreateBuffer(desc BufferDescriptor) (*resource.Buffer, error)
	// CreateBindGroup creates a bind group over desc.Layout.
	CreateBindGroup(desc BindGroupDescriptor) (*resource.BindGroup, error)
}

// Blit is the cached state of one Key: the nine pipelines the layer draws run
// through (the clear pass plus one per stencil bit) and the bind group layout
// their groups are built on.
type Blit struct {
	Pipelines Pipelines[*pipeline.RenderPipeline]
	Layout    *resource.BindGroupLayout
}

// The descriptor mirrors of the creation seam. The public descriptors carry the
// public wrappers; the payload may name only internal types, so these mirrors
// declare the same shapes over the implementation handles (ADR-070 R9, the
// internal/encoder/descriptor.go precedent). A mirror carries exactly the
// fields the payload sets: the root host converts field by field at the call
// boundary, and a field that is absent cannot be silently dropped there.

// TextureDescriptor describes texture creation parameters.
type TextureDescriptor struct {
	Label         string
	Size          gputypes.Extent3D
	MipLevelCount uint32
	SampleCount   uint32
	Dimension     gputypes.TextureDimension
	Format        gputypes.TextureFormat
	Usage         gputypes.TextureUsage
}

// TextureViewDescriptor describes texture view creation parameters.
type TextureViewDescriptor struct {
	Label           string
	Format          gputypes.TextureFormat
	Dimension       gputypes.TextureViewDimension
	Aspect          gputypes.TextureAspect
	BaseMipLevel    uint32
	MipLevelCount   uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
}

// BufferDescriptor describes buffer creation parameters.
type BufferDescriptor struct {
	Label            string
	Size             uint64
	Usage            gputypes.BufferUsage
	MappedAtCreation bool
}

// BindGroupDescriptor describes bind group creation over an internal layout.
type BindGroupDescriptor struct {
	Label   string
	Layout  *resource.BindGroupLayout
	Entries []BindGroupEntry
}

// BindGroupEntry describes one binding of a bind group: a buffer range or a
// texture view.
type BindGroupEntry struct {
	Binding     uint32
	Buffer      *resource.Buffer
	Offset      uint64
	Size        uint64
	TextureView *resource.TextureView
}

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
