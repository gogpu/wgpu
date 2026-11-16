//go:build !rust && !(js && wasm)

package encoder

// This file declares the encoder domain's two seams (ADR-070 S4, shape B):
// the contract with the stencil upload payload, and the small view of the
// device the encoder reaches it through. Both name internal types only: a
// root type in a signature would be the wgpu -> internal/encoder -> wgpu
// cycle under another name; the root host adapts at the call boundary.
//
// The payload interfaces (Upload, Recorder, PassRecorder, Releasable) lived in
// internal/stencilblit from the S4 Prepare until the Move, because
// internal/encoder did not exist yet; the Move relocates them here, and
// internal/stencilblit imports gputypes alone again. The stencil upload
// payload itself stays in the root package through S9 (the queue records its
// own blits), so the root package implements Upload and both recorders.

import (
	"github.com/gogpu/gputypes"
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
// draw verbs are the five stencilblit.RecordLayer drives; the pipeline type is
// the one the blit's cache holds. The verbs are declared explicitly rather
// than by embedding stencilblit.Pass: internal/encoder must not import
// internal/stencilblit (the encoder -> stencilblit edge belongs to S6), and a
// value of this interface type is assignable to stencilblit.Pass at the call.
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
// buffer releases them once it is spent.
type Releasable interface {
	Release()
}

// DeviceHost is the encoder domain's seam view of the device: exactly what the
// encoder calls on it and nothing else (ADR-070 shape B, user decision
// 2026-11-16). The concrete pool and recordings registry stay in the root
// package until S11, when they move with the Device into internal/device; the
// root host adapts at the call boundary, where it can name its own types.
//
// Contract for an implementation: a host with no pool must return an untyped
// nil from EncoderPool, so the encoder's nil checks (returnEncoderToPool,
// CommandBuffer.Release) keep their meaning across the interface — a nil
// *encoderPool boxed in the interface would defeat them.
type DeviceHost interface {
	// Recordings returns the device's recordings registry: the command
	// encoders and command buffers that are neither finished nor released.
	Recordings() core.EncoderRecordings[*CommandEncoder, *CommandBuffer]
	// EncoderPool returns the device's HAL command encoder pool, or an
	// untyped nil when the device has none.
	EncoderPool() core.HalEncoderPool
	// NewStencilUpload creates the staging texture an upload of size into the
	// stencil of dst at mipLevel and origin goes through.
	NewStencilUpload(dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (Upload, error)
	// Features returns the device's enabled features. The render pass reads
	// them for the draw feature gates (MultiDrawIndirect and
	// MultiDrawIndirectCount) exactly as the pre-move pass read them from the
	// root Device.
	//
	// A value, not a handle: there is no untyped-nil case here, and a
	// zero-value encoder (nil seam) panics on the call, as the pre-move nil
	// *Device dereference did. A host returns the features it enabled at
	// device creation, never a zero set: a zero set would silently disable
	// gates the device supports.
	Features() gputypes.Features
}
