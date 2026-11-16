//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package queue holds the queue domain's implementation (ADR-070 S9).
//
// Today it declares the domain's seam only: the device view the queue submits
// through. The Queue implementation moves here in the S9 Move; the root
// package keeps a declaration layer over it.
package queue

// This file declares the queue domain's seam view of the device (ADR-070
// shape B, user decision 2026-11-16): exactly what the moving queue calls on
// the device it submits through, and nothing else. The concrete device and
// its encoder pool stay in the root package until S11 moves them into
// internal/device; the root host adapts at the call boundary, where it can
// name its own types.
//
// Count, corrected for the move (2026-11-16): ADR-070 §12.3 sizes this seam at
// five methods (Core, IsReleased, EncoderPool, Recordings, CreateCommandEncoder).
// Measured on tree b061674 it is four: Recordings disappeared when S4 replaced
// the queue's registry lookup with CommandBuffer.MarkSubmitted, IsReleased is
// never read by any queue file, and NewStencilUpload appeared with S4's
// stencil lowering. core.DeviceView's six methods come with the embed and are
// not counted; the queue reads Core, the rest come by §15.3's design.
//
// Contract for an implementation: a host with no pool returns an untyped nil
// from EncoderPool, so the queue's nil checks keep their meaning across the
// interface — a nil concrete pool boxed in the interface would defeat them.
// No root type appears in a signature below: a root name here would be the
// wgpu -> internal/queue -> wgpu cycle under another name, and the root host
// converts at the call boundary.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// DeviceHost is the queue domain's seam view of the device: the shared
// core.DeviceView (the core state the queue records against) plus the three
// device facilities the queue uses. Every method the queue calls on the
// device is here; the adapter in the root package is the only implementation
// until S11.
type DeviceHost interface {
	core.DeviceView

	// EncoderPool returns the device's HAL command encoder pool, or an untyped
	// nil when the device has none. The queue acquires the barrier command
	// encoder from it, releases every encoder back to it (after ResetAll), and
	// records no barriers at all when the host has no pool. Returning an
	// untyped nil is the contract, not a convenience: the queue's nil checks
	// must hold across the interface.
	EncoderPool() core.HalEncoderPool

	// CreateCommandEncoder creates a user command encoder under label, exactly
	// as the public Device.CreateCommandEncoder does: the device registers it
	// in its recordings and attaches the test hooks, so the queue can submit
	// or discard it like one a caller created. label is the encoder's debug
	// label (the stencil upload's, today).
	CreateCommandEncoder(label string) (*encoder.CommandEncoder, error)

	// NewStencilUpload creates the staging R8Uint texture an upload of size
	// into the stencil of dst at mipLevel and origin goes through, and returns
	// the payload the queue records the blit from. dst is an implementation
	// handle; the host maps it back to whatever its own payload works on. The
	// payload stays in the root package through this step (S9 hands it back to
	// internal/stencilblit), so the type this method names is the encoder
	// domain's view of it today.
	NewStencilUpload(dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (encoder.Upload, error)
}
