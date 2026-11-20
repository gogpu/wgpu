// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

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
