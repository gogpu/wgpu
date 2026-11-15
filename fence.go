//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// Fence is a GPU synchronization primitive.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation. A zero-value Fence has no implementation: Release panics on it,
// as the pre-move type did (a nil device view).
type Fence struct {
	impl *resource.Fence
}

// Release destroys the fence.
func (f *Fence) Release() { f.impl.Release() }
