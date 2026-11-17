// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// MapMode selects the type of access requested for a buffer mapping.
// Values match WebGPU spec §5.3.2 GPUMapMode.
type MapMode uint32

const (
	// MapModeRead requests a read-only mapping. The buffer must have been
	// created with BufferUsageMapRead.
	MapModeRead MapMode = 1
	// MapModeWrite requests a write-only mapping. The buffer must have been
	// created with BufferUsageMapWrite.
	MapModeWrite MapMode = 2
)

// MapState reports the current mapping state of a buffer.
// Matches WebGPU spec §5.3.3 GPUBufferMapState plus the Destroyed
// terminal state used internally to guard against use-after-destroy.
type MapState uint8

const (
	// MapStateUnmapped indicates the buffer is not currently mapped.
	MapStateUnmapped MapState = iota
	// MapStatePending indicates the buffer has a Map or MapAsync in
	// progress but the GPU has not yet resolved the underlying fence.
	MapStatePending
	// MapStateMapped indicates the buffer is CPU-accessible via
	// MappedRange; Unmap returns it to Unmapped.
	MapStateMapped
	// MapStateDestroyed indicates Buffer.Destroy has been called and the
	// buffer can never be mapped again. Terminal state.
	MapStateDestroyed
)

// Typed buffer mapping errors, re-exported from internal/resource so that the
// values returned by the implementation and the public sentinels are the same
// errors.Is targets (ADR-070 §15.1).
var (
	// ErrMapAlreadyPending — a Map or MapAsync was called on a buffer
	// that already has an in-flight map request.
	ErrMapAlreadyPending = resource.ErrMapAlreadyPending

	// ErrMapAlreadyMapped — a Map or MapAsync was called on a buffer
	// that is already in the Mapped state.
	ErrMapAlreadyMapped = resource.ErrMapAlreadyMapped

	// ErrMapNotMapped — Unmap or MappedRange was called on a buffer that
	// is not currently mapped.
	ErrMapNotMapped = resource.ErrMapNotMapped

	// ErrMapRangeOverlap — MappedRange was called with a range that
	// overlaps a previously-returned MappedRange that has not yet been
	// invalidated by Unmap.
	ErrMapRangeOverlap = resource.ErrMapRangeOverlap

	// ErrMapRangeDetached — MappedRange.Bytes was called after the owning
	// buffer was unmapped or destroyed, leaving the range invalid.
	ErrMapRangeDetached = resource.ErrMapRangeDetached

	// ErrMapAlignment — the requested offset is not a multiple of 8
	// bytes, or the requested size is not a multiple of 4 bytes, in
	// violation of WebGPU spec MAP_ALIGNMENT / copy alignment.
	ErrMapAlignment = resource.ErrMapAlignment

	// ErrMapCanceled — a pending map was canceled by a concurrent
	// Unmap, Destroy, or context cancellation before the GPU resolved it.
	// Analogous to WebGPU AbortError.
	ErrMapCanceled = resource.ErrMapCanceled

	// ErrBufferDestroyed — Buffer.Destroy was called while the map was
	// pending, or a Map call targeted an already-destroyed buffer.
	ErrBufferDestroyed = resource.ErrBufferDestroyed

	// ErrMapDeviceLost — the device was lost while the map was pending.
	ErrMapDeviceLost = resource.ErrMapDeviceLost

	// ErrMapInvalidMode — Map or MapAsync was called on a buffer that
	// was not created with the matching MAP_READ or MAP_WRITE usage.
	ErrMapInvalidMode = resource.ErrMapInvalidMode

	// ErrMapRangeOverflow — the requested offset + size exceeds the
	// buffer size.
	ErrMapRangeOverflow = resource.ErrMapRangeOverflow
)
