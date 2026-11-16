//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file declares the device domain's view of the instance that owns it. It
// is the measured access set of the moving device code on tree 1d1778e
// (device_native.go): the teardown scan that collects the surfaces configured
// against the device (Release, :1678) and the unregister that removes the
// device from the instance's registry (:1689). The concrete instance stays in
// the root package until S12 moves it into internal/instance; the root host
// adapts at the call boundary, where it can name its own types.
//
// SetInstance on the device (declared with the type, next commit) takes the
// host and never *instance.Instance, so the cut holds in both directions: the
// device names no instance type, and the instance domain declares no seam of
// its own for this. The root wires the host at construction, in
// Instance.adoptDevice, which is the one place a device learns its instance.
//
// Supersession (2026-11-16, lead ruling). ADR-070 §15.3 sized this seam as
// `core.InstanceHost`, expressed in core-level types. That shape cannot fit
// this domain. SurfacesForDevice must return objects the device can discard
// and retire, and the retire is not a logical core call: it recycles the
// swapchain texture's TrackerIndex while the backend device is still alive, so
// it lives on *surface.Surface (surface_native.go, RetireDevice) and a
// []*core.Surface list would lose it. A core-declared interface could carry
// it only through a core.DeviceSurface interface whose sole implementation is
// *surface.Surface — an interface that exists to satisfy a wording, and it
// would leave the golden row internal/device → internal/surface (native)
// ungenerated. The seam is therefore device-local and names *surface.Surface
// directly, exactly as the S10 surface seam named its own types instead of
// core.InstanceHost (internal/surface/seams_native.go). The row stays
// generated, and the inverted device → instance edge still holds: the device
// names no instance type.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/surface"
)

// InstanceHost is the device domain's view of the instance that owns it: the
// surfaces configured against this device, and the removal of the device from
// the instance's registry. Every instance facility the device calls is here;
// the root host is the only implementation until S12 moves the instance.
//
// The device view is the shared core.DeviceView for the same reason every
// other child seam takes one: the host filters the instance's surfaces by the
// core device identity (Surface.SameDevice), which is the identity a surface
// configures and acquires against.
type InstanceHost interface {
	// SurfacesForDevice returns the surface implementations configured against
	// this device, as a teardown snapshot. The device discards each one's
	// acquired texture while its backend device is still alive and retires it
	// after the backend device is gone; both calls are no-ops for a surface
	// configured against another device. A nil slice means no surface.
	SurfacesForDevice(core.DeviceView) []*surface.Surface

	// UnregisterDevice removes the device from the instance's device registry,
	// once, at the end of its release. It is the last instance-side fact the
	// teardown reports.
	UnregisterDevice(core.DeviceView)
}
