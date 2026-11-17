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

// Release destroys the sampler.
func (s *Sampler) Release() { s.impl.Release() }
