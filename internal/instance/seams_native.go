//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file declares the instance domain's view of the devices it adopts: the
// registry it keeps of them. It is the measured access set of the moving
// instance code on tree 17a8c4d (instance_native.go): the registry is scanned
// for the teardown, which calls each device's Release before the core instance
// is destroyed (:260-266), and a device removes itself when it releases itself
// (:189-196). The concrete device stays in internal/device; the instance is its
// parent, so this is the one seam the parent declares for the child, and it
// names no internal/device type — the caller hands an adopted *device.Device in
// at its creation site.
//
// The other direction is already declared: internal/device's InstanceHost
// (seams_native.go) is what this package implements, and the device's
// SetInstance parameter never names *instance.Instance. This file is the return
// path.

import "github.com/gogpu/wgpu/internal/core"

// deviceRef is what the instance keeps per adopted device: the shared view is
// the registry key, Release is the teardown call.
//
// The registry is map[core.DeviceView]deviceRef, declared with the type at the
// Move. core.DeviceView is the map key the ADR fixes, and Release is the one
// device method the instance calls on the value. Everything else the instance
// does with a device — SetInstance at adoption — happens at the adapter's
// construction site, where the concrete constructor is already in hand, so it
// is not part of the seam.
//
// Supersession (2026-11-16, ADR-070 §15.3). §15.3 (:1081) fixes the registry as
// map[core.DeviceView]struct{}. That literal cannot release a device: the
// instance's Release iterates the registry and calls device.Release() before it
// destroys the core instance (instance_native.go:260-266), and
// core.DeviceView has no Release. A struct{} value would force a second
// registry or a type assertion on the value, and both are the divergent state
// the ADR's single keyed map exists to avoid. The value type is therefore a
// deviceRef. The seam measure is unchanged: one method is called on the value
// (Release), plus the shared view's six counted once (§15.3).
type deviceRef interface {
	core.DeviceView
	Release()
}
