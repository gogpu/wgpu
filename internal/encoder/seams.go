//go:build !rust && !(js && wasm)

package encoder

// This file declares the encoder domain's seam view of the device (ADR-070
// shape B): exactly what the encoder calls on it and nothing else — the
// recordings registry, the encoder pool, the stencil upload and the enabled
// features. The concrete pool and recordings registry stay in the root
// package until S11, when they move with the Device into internal/device; the
// root host adapts at the call boundary, where it can name its own types.
//
// The stencil payload's interfaces (Upload, Recorder, PassRecorder,
// Releasable) are internal/stencilblit's: they were declared here from the S4
// Move until S9, when the hand-back returned them to the package that owns the
// payload. Their contracts, and the reasons for each method, are documented
// there.
//
// Contract for an implementation: a host with no pool must return an untyped
// nil from EncoderPool, so the encoder's nil checks (returnEncoderToPool,
// CommandBuffer.Release) keep their meaning across the interface — a nil
// *encoderPool boxed in the interface would defeat them.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// DeviceHost is the encoder domain's seam view of the device: the shared
// registry and pool the encoder creates and recycles through, the stencil
// upload it lowers into, and the features its draw gates read. The root host
// is the only implementation until S11.
type DeviceHost interface {
	// Recordings returns the device's recordings registry: the command
	// encoders and command buffers that are neither finished nor released.
	Recordings() core.EncoderRecordings[*CommandEncoder, *CommandBuffer]
	// EncoderPool returns the device's HAL command encoder pool, or an
	// untyped nil when the device has none.
	EncoderPool() core.HalEncoderPool
	// NewStencilUpload creates the staging texture an upload of size into the
	// stencil of dst at mipLevel and origin goes through. The payload lives in
	// internal/stencilblit: it owns the type and the blit it records.
	NewStencilUpload(dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (*stencilblit.Upload, error)
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
