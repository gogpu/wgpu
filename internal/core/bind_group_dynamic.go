//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import "github.com/gogpu/gputypes"

// DynamicBinding describes one buffer binding of a bind group whose layout
// entry has HasDynamicOffset (WebGPU GPUProgrammablePassEncoder.setBindGroup
// dynamic offsets). A bind group keeps them sorted by binding number, the
// order of the dynamic offsets it is given. It matches Rust wgpu-core's
// BindGroupDynamicBindingData.
//
// It lives in core, next to BindGroupUsages, because it is the shared shape of
// a bind group's derived state: the bind group holds it while it moves to an
// extraction package, and the frontend device derives it from the public
// creation descriptor, which stays in the root package (ADR-070 §11, S2).
type DynamicBinding struct {
	// Binding is the binding number from the layout entry.
	Binding uint32
	// BindingType is the buffer binding's type, which sets the dynamic
	// offset alignment.
	BindingType gputypes.BufferBindingType
	// BufferSize is the bound buffer's size in bytes.
	BufferSize uint64
	// BindingEnd is the binding's offset plus its size in the buffer.
	BindingEnd uint64
	// MaxDynamicOffset is the largest dynamic offset that keeps the binding
	// inside the buffer: BufferSize - BindingEnd.
	MaxDynamicOffset uint64
}
