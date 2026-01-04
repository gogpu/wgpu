// Package core provides tracker index allocators for resource tracking.
//
// TrackerIndexAllocators manages per-resource-type indices used by the
// resource usage tracker. Each resource type gets unique indices to
// track its state across command buffers.

package core

// TrackerIndexAllocators manages tracker indices per resource type.
//
// This is used to assign unique indices to resources for tracking their
// state and usage across command buffer recording and submission.
//
// Stub implementation - will be expanded in CORE-006.
type TrackerIndexAllocators struct{}

// NewTrackerIndexAllocators creates a new TrackerIndexAllocators.
func NewTrackerIndexAllocators() *TrackerIndexAllocators {
	return &TrackerIndexAllocators{}
}
