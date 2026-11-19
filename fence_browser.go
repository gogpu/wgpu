//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// Fence is a GPU synchronization primitive. On the browser it is a no-op, but
// the public type still exists for API compatibility.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation.
type Fence struct {
	impl *resource.Fence
}

// Release destroys the fence. A zero-value wrapper panics, as the zero-value
// tests pin.
func (f *Fence) Release() { f.impl.Release() }
