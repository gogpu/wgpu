// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/loststate"

// DeviceLostReason reports why a device was lost, mirroring the WebGPU
// GPUDeviceLostReason concept. Only the two spec values exist on every build;
// a driver-initiated loss is DeviceLostReasonUnknown.
type DeviceLostReason int

const (
	// DeviceLostReasonUnknown is a loss reported by a driver or backend
	// without a more specific cause.
	DeviceLostReasonUnknown DeviceLostReason = iota
	// DeviceLostReasonDestroyed is the loss a (*Device).Release produces.
	DeviceLostReasonDestroyed
)

// DeviceLostInfo is one device-lost event's payload: the reason and a
// human-readable message from the backend.
type DeviceLostInfo struct {
	Reason  DeviceLostReason
	Message string
}

// lostReasonTable maps the internal loss reason onto the public one, keyed by
// the internal constant: a renumbered or added internal reason cannot silently
// re-map, and one outside the table falls to Unknown rather than inventing a
// third public value (the shape's "an unexpected reason maps to Unknown").
// Shared by every build's conversion in this package.
var lostReasonTable = [...]DeviceLostReason{
	loststate.ReasonUnknown:   DeviceLostReasonUnknown,
	loststate.ReasonDestroyed: DeviceLostReasonDestroyed,
}

// publicLostReason maps the internal reason to the public one. It is a bounds
// check and a table read, not a switch: the public-layer rule reads a switch in
// this package as logic (ADR-070 §15.10).
func publicLostReason(reason loststate.Reason) DeviceLostReason {
	if int(reason) < 0 || int(reason) >= len(lostReasonTable) {
		return DeviceLostReasonUnknown
	}
	return lostReasonTable[reason]
}
