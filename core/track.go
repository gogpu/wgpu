//go:build !(js && wasm)

// Package core provides tracker index allocators for resource tracking.
//
// TrackerIndexAllocators manages per-resource-type indices used by the
// resource usage tracker. Each resource type gets unique indices to
// track its state across command buffers.

package core

import "github.com/gogpu/wgpu/core/track"

// TrackerIndexAllocators manages tracker indices per resource type.
//
// This wraps track.TrackerIndexAllocators to provide per-resource-type
// allocators at the device level. Each resource type (Buffer, Texture, etc.)
// gets its own allocator namespace for dense index assignment.
type TrackerIndexAllocators struct {
	inner *track.TrackerIndexAllocators
}

// NewTrackerIndexAllocators creates a new TrackerIndexAllocators with
// allocators for all resource types.
func NewTrackerIndexAllocators() *TrackerIndexAllocators {
	return &TrackerIndexAllocators{
		inner: track.NewTrackerIndexAllocators(),
	}
}

// Textures returns the shared allocator for texture tracker indices.
func (a *TrackerIndexAllocators) Textures() *track.SharedTrackerIndexAllocator {
	if a == nil || a.inner == nil {
		return nil
	}
	return a.inner.Textures
}

// Buffers returns the shared allocator for buffer tracker indices.
func (a *TrackerIndexAllocators) Buffers() *track.SharedTrackerIndexAllocator {
	if a == nil || a.inner == nil {
		return nil
	}
	return a.inner.Buffers
}
