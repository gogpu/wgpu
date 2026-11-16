//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package surface holds the surface domain's implementation (ADR-070 S10).
//
// This context is the native one: the Surface implementation and the seam view
// of the device it configures, acquires and presents through live here. The
// root package keeps a declaration layer over it.
package surface

// This file declares the surface domain's seam view of the device. It is the
// measured access set of the moving surface code on tree f84e1bf
// (surface_native.go): Core (:314, :324, :371, :375), IsReleased (:281, :719),
// HALDevice (:759) and HALQueue (:419, :430). The concrete device stays in the
// root package until S11 moves it into internal/device; the root host adapts at
// the call boundary, where it can name its own types.
//
// Supersession (2026-11-16, lead ruling). ADR-070 §15.3 sized this seam as the
// embedded core.DeviceView plus `HALQueue() hal.Queue` plus
// `Instance() core.InstanceHost`. The last method cannot fit this domain:
// core.InstanceHost's stated methods are the device-facing ones
// (SurfacesForDevice, UnregisterDevice), and a leaf package cannot name
// *surface.Surface (ADR-070 §12.4). Measured, the surface needs the instance
// only for three root-side facts — the cross-instance rejection on Configure,
// the HAL instance a backend's raw surface is created on, and the unregister on
// release — all of which the root declaration layer performs, since the root
// owns the surface's instance pointer until S12. The seam is therefore the
// embed plus HALQueue: four methods by §15.3's count (Core, IsReleased,
// HALDevice, HALQueue), one of them declared here. The inverted
// surface → instance edge still holds: the implementation holds no instance
// reference.
//
// Contract for an implementation: every method reports the live device behind
// the surface, and HALQueue returns an untyped nil when there is none, so the
// surface's nil checks keep their meaning across the interface.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// DeviceHost is the surface domain's seam view of the device: the shared
// core.DeviceView (the core state a surface configures and acquires against,
// the released flag, and the HAL device a texture view is created on) plus the
// HAL queue presentation goes through. Every device facility the surface calls
// is here; the root host is the only implementation until S11.
type DeviceHost interface {
	core.DeviceView

	// HALQueue returns the HAL queue presentation goes through, or an untyped
	// nil when the device has no queue. The surface presents only through it
	// and rejects a present when it is missing. Returning an untyped nil is the
	// contract, not a convenience: a nil *queue.Queue boxed in this interface
	// would defeat the surface's nil check.
	HALQueue() hal.Queue
}
