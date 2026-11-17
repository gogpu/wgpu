//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

import "github.com/gogpu/wgpu/internal/core"

// fakeDeviceRef is a minimal implementer of deviceRef: the embedded shared view
// carries the six core methods, and Release is the only method the seam adds.
// The assertion below is the seam's compile link for this commit — it pins that
// the 7-method view is satisfiable by a small implementer rather than being an
// interface no type can meet — and commit 3 replaces this fake with the registry
// tests' double, which observes the release.
type fakeDeviceRef struct {
	core.DeviceView

	released bool
}

// Release reports that the instance's teardown reached this device.
func (f *fakeDeviceRef) Release() { f.released = true }

var _ deviceRef = (*fakeDeviceRef)(nil)
