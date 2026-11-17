// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package instance holds the GPU instance and adapter domain's implementation
// (ADR-070 S12).
//
// An instance creates the backend instances, enumerates adapters and adopts the
// devices and surfaces built on them; it is the device's parent and releases
// the adopted devices before its own core instance. At S12 the implementation
// leaves the root package, which keeps the public Instance and Adapter
// declaration layers over it.
//
// The instance's view of an adopted device, which is all its teardown needs, is
// declared per context in seams_native.go. The rust and browser contexts
// declare none: there the instance implements the device's own seam
// (internal/device's InstanceHost and InstanceR) and calls no device method, so
// no return seam exists.
//
// The public Instance and Adapter types stay in the root package as declaration
// layers: they hold this package's implementation handle, convert every public
// descriptor to scalars and internal handles at the boundary, and keep the root
// identity caches the surface cross-check reads. The surface creation glue
// stays there too (S10's ruling): the public target types and their sentinels
// cannot be named here.
package instance
