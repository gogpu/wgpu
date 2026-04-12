// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"errors"

	"github.com/gogpu/wgpu/core"
)

// FEAT-WGPU-MAPPING-001 — public WebGPU-compliant Buffer mapping API.
//
// See docs/dev/research/ADR-BUFFER-MAPPING-API.md for the full design
// rationale. This file contains the public types (enums + typed errors)
// used by Buffer.Map / Buffer.MapAsync / Device.Poll.

// MapMode selects the type of access requested for a buffer mapping.
// Values match WebGPU spec §5.3.2 GPUMapMode.
type MapMode uint32

const (
	// MapModeRead requests a read-only mapping. The buffer must have been
	// created with BufferUsageMapRead.
	MapModeRead MapMode = 1
	// MapModeWrite requests a write-only mapping. The buffer must have
	// been created with BufferUsageMapWrite.
	MapModeWrite MapMode = 2
)

func (m MapMode) toInternal() core.MapModeInternal {
	switch m {
	case MapModeRead:
		return core.MapModeInternalRead
	case MapModeWrite:
		return core.MapModeInternalWrite
	}
	return 0
}

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

func mapStateFromCore(s core.BufferMapState) MapState {
	switch s {
	case core.BufferMapStateIdle:
		return MapStateUnmapped
	case core.BufferMapStatePending:
		return MapStatePending
	case core.BufferMapStateMapped:
		return MapStateMapped
	case core.BufferMapStateDestroyed:
		return MapStateDestroyed
	}
	return MapStateUnmapped
}

// PollType selects the blocking behavior of Device.Poll.
type PollType uint8

const (
	// PollPoll drains any already-completed submissions and returns
	// immediately without blocking on the GPU. This is the fast path
	// for game loops that call Poll every frame.
	PollPoll PollType = iota
	// PollWait blocks until every submission currently known to the
	// device has completed, then drains. Used by Buffer.Map and by
	// Device.Poll(PollWait) for shutdown drains.
	PollWait
)

// Typed buffer mapping errors. Callers can use errors.Is to branch on
// specific failure modes without matching strings.
var (
	// ErrMapAlreadyPending — a Map or MapAsync was called on a buffer
	// that already has an in-flight map request.
	ErrMapAlreadyPending = errors.New("wgpu: buffer map already pending")

	// ErrMapAlreadyMapped — a Map or MapAsync was called on a buffer
	// that is already in the Mapped state.
	ErrMapAlreadyMapped = errors.New("wgpu: buffer is already mapped")

	// ErrMapNotMapped — Unmap or MappedRange was called on a buffer that
	// is not currently mapped.
	ErrMapNotMapped = errors.New("wgpu: buffer is not mapped")

	// ErrMapRangeOverlap — MappedRange was called with a range that
	// overlaps a previously-returned MappedRange that has not yet been
	// invalidated by Unmap.
	ErrMapRangeOverlap = errors.New("wgpu: mapped range overlaps existing")

	// ErrMapRangeDetached — MappedRange.Bytes was called after the owning
	// buffer was unmapped or destroyed, leaving the range invalid.
	ErrMapRangeDetached = errors.New("wgpu: mapped range detached (buffer unmapped)")

	// ErrMapAlignment — the requested offset is not a multiple of 8
	// bytes, or the requested size is not a multiple of 4 bytes, in
	// violation of WebGPU spec MAP_ALIGNMENT / copy alignment.
	ErrMapAlignment = errors.New("wgpu: map offset/size not aligned")

	// ErrMapCanceled — a pending map was canceled by a concurrent
	// Unmap, Destroy, or context cancellation before the GPU resolved it.
	// Analogous to WebGPU AbortError.
	ErrMapCanceled = errors.New("wgpu: map canceled")

	// ErrBufferDestroyed — Buffer.Destroy was called while the map was
	// pending, or a Map call targeted an already-destroyed buffer.
	ErrBufferDestroyed = errors.New("wgpu: buffer destroyed")

	// ErrMapDeviceLost — the device was lost while the map was pending.
	ErrMapDeviceLost = errors.New("wgpu: device lost during map")

	// ErrMapInvalidMode — Map or MapAsync was called on a buffer that
	// was not created with the matching MAP_READ or MAP_WRITE usage.
	ErrMapInvalidMode = errors.New("wgpu: buffer not created with required MAP_READ/MAP_WRITE usage")

	// ErrMapRangeOverflow — the requested offset + size exceeds the
	// buffer size.
	ErrMapRangeOverflow = errors.New("wgpu: map range exceeds buffer size")
)

// coreErrToTyped converts a *core.BufferMapError into the corresponding
// named typed error. Nil in → nil out.
func coreErrToTyped(e *core.BufferMapError) error {
	if e == nil {
		return nil
	}
	switch e.Kind {
	case core.BufferMapErrKindAlreadyPending:
		return ErrMapAlreadyPending
	case core.BufferMapErrKindAlreadyMapped:
		return ErrMapAlreadyMapped
	case core.BufferMapErrKindNotMapped:
		return ErrMapNotMapped
	case core.BufferMapErrKindAlignment:
		return ErrMapAlignment
	case core.BufferMapErrKindInvalidMode:
		return ErrMapInvalidMode
	case core.BufferMapErrKindRangeOverflow:
		return ErrMapRangeOverflow
	case core.BufferMapErrKindCancelled:
		return ErrMapCanceled
	case core.BufferMapErrKindDestroyed:
		return ErrBufferDestroyed
	case core.BufferMapErrKindDeviceLost:
		return ErrMapDeviceLost
	case core.BufferMapErrKindRangeOverlap:
		return ErrMapRangeOverlap
	case core.BufferMapErrKindRangeDetached:
		return ErrMapRangeDetached
	case core.BufferMapErrKindHAL:
		return e // preserve wrapped HAL error for errors.Unwrap
	}
	return e
}
