// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// LateBufferBindingInfo records the actual buffer binding size for a layout
// entry with MinBindingSize == 0. It matches Rust wgpu-core's
// BindGroupLateBufferBindingInfo (binding_model.rs:1167-1173).
//
// The type is the same in every build context, so it lives in this tag-free
// file rather than with the per-context bind-group declarations.
type LateBufferBindingInfo struct {
	// BindingIndex is the binding number from the layout entry.
	BindingIndex uint32
	// Size is the actual buffer binding size at bind group creation time.
	Size uint64
}
