//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// Sampler represents a texture sampler.
//
// The implementation lives in internal/resource (ADR-070); Release is one
// delegation.
type Sampler struct {
	impl *resource.Sampler
}

// Release destroys the sampler. A zero-value wrapper panics, as the zero-value
// tests pin.
func (s *Sampler) Release() { s.impl.Release() }
