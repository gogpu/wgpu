//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// Sampler represents a texture sampler.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation. A zero-value Sampler has no implementation: Release panics on
// it, as the pre-move type did (a nil device view); the zero-value tests pin
// that contract.
type Sampler struct {
	impl *resource.Sampler
}

// Release drops the application's reference to the sampler.
func (s *Sampler) Release() { s.impl.Release() }
