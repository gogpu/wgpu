//go:build !rust && !(js && wasm)

package core

import "github.com/gogpu/wgpu/internal/hal"

// HalEncoderPool is the encoder domain's seam view of the device's HAL
// command encoder pool (ADR-070 shape B, user decision 2026-11-16).
//
// It declares exactly what the moving encoder files call on the pool and
// nothing of its device-internal lifecycle (retire, destroy): the concrete
// pool stays in the root package until S11 moves it with the Device into
// internal/device. The root host adapts at the call boundary, where it can
// name its own concrete type; the child seam embeds core.DeviceView and adds
// this accessor (the S1 precedent).
//
// No type of internal/encoder or of the root package appears here on
// purpose: a core -> internal/encoder edge would be the S3 import cycle under
// another name, and core cannot name a root type. hal.CommandEncoder is the
// HAL's own type and this package already imports internal/hal.
//
// Contract for an implementation: a host with no pool must return an untyped
// nil, so the encoder's nil checks keep their meaning across the interface.
type HalEncoderPool interface {
	// Acquire returns a command encoder in "closed" state, ready for
	// BeginEncoding: a recycled one when the pool holds one, a newly
	// created one otherwise.
	Acquire() (hal.CommandEncoder, error)
	// Release returns an acquired encoder to the pool after ResetAll. The
	// encoder must be in "closed" state (not recording).
	Release(hal.CommandEncoder)
}

// EncoderRecordings is the encoder domain's seam view of the device's
// recordings registry (ADR-070 shape B, user decision 2026-11-16): the
// command encoders that are neither finished nor discarded, and the command
// buffers that are neither submitted nor released. Until an encoder reaches
// the queue (or is released), nothing else drops the ResourceRefs it took.
//
// The encoder and command buffer types are parameters because a concrete type
// here would be either a root name (forbidden: internal/core cannot import the
// root it serves) or a forward reference to internal/encoder, which does not
// exist yet; they are the root types today and the destination's types after
// S4's Move. The registry keeps them apart for lifetime purposes only and
// never calls back into them.
//
// The device-internal drain (take) is deliberately absent: it belongs to the
// host that owns the registry, never to the encoder domain.
//
// Final home: the concrete registry stays in the root package until S11, when
// it moves with the Device into internal/device; this declaration stays in
// internal/core, the seam package (the DeviceView precedent).
type EncoderRecordings[E, B any] interface {
	// AddEncoder records an encoder that started recording.
	AddEncoder(E)
	// RemoveEncoder forgets an encoder that finished or was discarded.
	RemoveEncoder(E)
	// AddBuffer records a command buffer awaiting submit or release.
	AddBuffer(B)
	// RemoveBuffer forgets a command buffer that was submitted or released.
	RemoveBuffer(B)
}
