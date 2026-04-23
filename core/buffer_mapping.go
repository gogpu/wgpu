//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// FEAT-WGPU-MAPPING-001 — WebGPU-compliant Buffer mapping state machine.
//
// This file contains the core per-Buffer and per-Device state that drives
// Buffer.Map / Buffer.MapAsync / Device.Poll. It mirrors Rust wgpu-core's
// BufferMapState (wgpu-core/src/resource.rs) and LifetimeTracker
// (wgpu-core/src/device/life.rs) but with a Go-idiomatic blocking API on
// top (see wgpu/buffer.go for the public layer).
//
// State transitions, concurrency model, and safety invariants are
// documented in docs/dev/research/ADR-BUFFER-MAPPING-API.md §State machine
// and §Safety guarantees.

// Extended BufferMapState values. The original 3-state enum
// (Idle / Pending / Mapped) in resource.go is extended with Destroyed for
// the anti-UAF invariant — a destroyed buffer is terminal and cannot be
// mapped. MappedAtCreation is represented as Mapped + the underlying
// HAL mapping; the state machine does not need a distinct variant for it
// because the user-visible behavior is identical.
const (
	// BufferMapStateDestroyed indicates the buffer has been destroyed and
	// cannot be mapped. Terminal state.
	BufferMapStateDestroyed BufferMapState = 3
)

// MapModeInternal is the internal representation of the requested mapping
// mode. Only two values — read or write — matching WebGPU spec §5.3.2.
type MapModeInternal uint8

const (
	// MapModeInternalRead requests a read-only mapping; buffer must have
	// BufferUsageMapRead.
	MapModeInternalRead MapModeInternal = iota + 1
	// MapModeInternalWrite requests a write-only mapping; buffer must have
	// BufferUsageMapWrite.
	MapModeInternalWrite
)

// PollTypeInternal selects the blocking behavior of Device.PollMaps.
type PollTypeInternal uint8

const (
	// PollTypePoll drains all already-completed submissions and returns
	// immediately without blocking on the GPU.
	PollTypePoll PollTypeInternal = iota
	// PollTypeWait blocks until every submission currently known to the
	// device has completed, then drains.
	PollTypeWait
)

// BufferMapErrorKind classifies an internal map-state-machine error so
// the public API can map it to a typed error.
type BufferMapErrorKind uint8

const (
	// BufferMapErrKindNone is the zero value (no error).
	BufferMapErrKindNone BufferMapErrorKind = iota
	BufferMapErrKindAlreadyPending
	BufferMapErrKindAlreadyMapped
	BufferMapErrKindNotMapped
	BufferMapErrKindAlignment
	BufferMapErrKindInvalidMode
	BufferMapErrKindRangeOverflow
	BufferMapErrKindCancelled
	BufferMapErrKindDestroyed
	BufferMapErrKindDeviceLost
	BufferMapErrKindRangeOverlap
	BufferMapErrKindRangeDetached
	BufferMapErrKindHAL
)

// BufferMapError is the single internal error type for state-machine
// failures. The public layer (wgpu/buffer.go) maps Kind to a named typed
// error so users can use errors.Is without string matching.
type BufferMapError struct {
	Kind    BufferMapErrorKind
	Wrapped error
}

func (e *BufferMapError) Error() string {
	if e == nil {
		return ""
	}
	if e.Wrapped != nil {
		return "wgpu: buffer map: " + e.Wrapped.Error()
	}
	switch e.Kind {
	case BufferMapErrKindAlreadyPending:
		return "wgpu: buffer map already pending"
	case BufferMapErrKindAlreadyMapped:
		return "wgpu: buffer is already mapped"
	case BufferMapErrKindNotMapped:
		return "wgpu: buffer is not mapped"
	case BufferMapErrKindAlignment:
		return "wgpu: map offset/size not aligned"
	case BufferMapErrKindInvalidMode:
		return "wgpu: buffer not created with required MAP_READ/MAP_WRITE usage"
	case BufferMapErrKindRangeOverflow:
		return "wgpu: map range exceeds buffer size"
	case BufferMapErrKindCancelled:
		return "wgpu: map canceled"
	case BufferMapErrKindDestroyed:
		return "wgpu: buffer destroyed"
	case BufferMapErrKindDeviceLost:
		return "wgpu: device lost during map"
	case BufferMapErrKindRangeOverlap:
		return "wgpu: mapped range overlaps existing"
	case BufferMapErrKindRangeDetached:
		return "wgpu: mapped range detached (buffer unmapped)"
	case BufferMapErrKindHAL:
		return "wgpu: buffer map: HAL error"
	}
	return "wgpu: buffer map: unknown error"
}

func (e *BufferMapError) Unwrap() error { return e.Wrapped }

// MapWaiter is the internal signaling primitive for a pending Map.
// It is reused across resolution to keep Map in the zero-alloc path:
// the same waiter lives on the Buffer for its lifetime.
type MapWaiter struct {
	mu   sync.Mutex
	cond *sync.Cond
	done bool
	err  *BufferMapError
}

func newMapWaiter() *MapWaiter {
	w := &MapWaiter{}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// Reset puts the waiter back into the pending state. Called under the
// Buffer mapping mutex when initiating a new Map.
func (w *MapWaiter) Reset() {
	w.mu.Lock()
	w.done = false
	w.err = nil
	w.mu.Unlock()
}

// Signal marks the waiter as done and wakes all Wait callers. err may be
// nil on success.
func (w *MapWaiter) Signal(err *BufferMapError) {
	w.mu.Lock()
	w.done = true
	w.err = err
	w.cond.Broadcast()
	w.mu.Unlock()
}

// Wait blocks until signaled. Returns the recorded error (nil on
// success). Does NOT respect cancellation — the public layer wraps this
// with a context-aware goroutine.
func (w *MapWaiter) Wait() *BufferMapError {
	w.mu.Lock()
	for !w.done {
		w.cond.Wait()
	}
	err := w.err
	w.mu.Unlock()
	return err
}

// Status returns the current completion state non-blockingly.
func (w *MapWaiter) Status() (done bool, err *BufferMapError) {
	w.mu.Lock()
	done = w.done
	err = w.err
	w.mu.Unlock()
	return
}

// mappedRangeRec tracks one outstanding MappedRange for overlap detection.
type mappedRangeRec struct {
	offset uint64
	size   uint64
}

// mapDataSlot is an indirection that lets us embed an atomic.Pointer in
// something copyable. core.Buffer holds *mapDataSlot; the slot wraps the
// actual atomic pointer to the lazily-allocated BufferMapData.
type mapDataSlot struct {
	ptr atomic.Pointer[BufferMapData]
}

// BufferMapData holds per-Buffer state machine fields. It is embedded as a
// pointer on core.Buffer so existing struct initializers don't change.
type BufferMapData struct {
	// mu guards all fields in this struct and the parent Buffer's
	// mapState. Acquire before Device.pendingMapsMu.
	mu sync.Mutex

	// waiter is the single per-Buffer waiter object. It is reused across
	// Map cycles — Reset at the start of each Map, Signal at resolution,
	// Wait by callers.
	waiter *MapWaiter

	// generation is bumped atomically on every Unmap and Destroy so that
	// stale MappedRange handles can detect detachment. Loaded lock-free
	// by MappedRange.Bytes().
	generation atomic.Uint64

	// pendingMode / pendingOffset / pendingSize describe the active
	// Map request while mapState is Pending or Mapped. Cleared on Unmap.
	pendingMode   MapModeInternal
	pendingOffset uint64
	pendingSize   uint64

	// mapping is the result of hal.Device.MapBuffer, valid while
	// mapState == BufferMapStateMapped. Cleared on Unmap/Destroy.
	mapping hal.BufferMapping

	// mappedRanges records every currently-live MappedRange for overlap
	// detection (WebGPU spec §5.3.4). Typical case is 1 or 2 entries; a
	// slice keeps this zero-alloc for the common case after the initial
	// capacity is established.
	mappedRanges []mappedRangeRec
}

// Generation returns the current mapping generation. Called by
// MappedRange.Bytes() to detect stale handles without locking.
func (b *Buffer) Generation() uint64 {
	md := b.loadMapData()
	if md == nil {
		return 0
	}
	return md.generation.Load()
}

// ensureMapData lazily allocates the state-machine scratch area via a
// compare-and-swap so the outer Buffer struct can remain copyable.
// Map/MapAsync stay allocation-free after the first call per buffer.
//
// First call on any Buffer may allocate both the mapDataSlot container
// and the BufferMapData. Subsequent calls are lock-free loads.
func (b *Buffer) ensureMapData() *BufferMapData {
	slot := b.mapDataSlot
	if slot == nil {
		// Slot itself missing — this happens for Buffer values created
		// through the legacy zero-value path (the Hub's value-typed
		// registry). Upgrade the receiver by allocating a fresh slot
		// on the outer pointer. Because the slot is a pointer, this is
		// racy only in the sense that two concurrent ensureMapData
		// calls may both allocate; whichever one wins the CAS stays.
		slot = &mapDataSlot{}
		// Cannot use atomic.CompareAndSwap on a non-atomic field; but
		// Buffer values reachable through the HAL path always have a
		// pre-populated slot (see NewBuffer). Values reached only via
		// the legacy zero-value path never call ensureMapData because
		// they never trigger Map/MapAsync. Assign unconditionally.
		b.mapDataSlot = slot
	}
	if md := slot.ptr.Load(); md != nil {
		return md
	}
	fresh := &BufferMapData{waiter: newMapWaiter()}
	if slot.ptr.CompareAndSwap(nil, fresh) {
		return fresh
	}
	return slot.ptr.Load()
}

// loadMapData returns the state area (may be nil if never initialized).
// Shortcut used by read-only accessors.
func (b *Buffer) loadMapData() *BufferMapData {
	if b.mapDataSlot == nil {
		return nil
	}
	return b.mapDataSlot.ptr.Load()
}

// Waiter returns the per-buffer waiter (public for the wgpu package).
func (b *Buffer) Waiter() *MapWaiter { return b.ensureMapData().waiter }

// BeginMap requests a new mapping on the buffer.
//
// On success the buffer transitions to Pending; the caller must enqueue
// the Buffer on Device.RegisterPendingMap so a later Poll resolves it.
//
// Validation is performed synchronously — WebGPU spec §5.3.5 requires
// argument errors to surface from the request, not at resolution time.
//
// Returns nil on success (state is Pending), or a *BufferMapError that
// the public layer maps to a typed error.
func (b *Buffer) BeginMap(mode MapModeInternal, offset, size uint64) *BufferMapError {
	md := b.ensureMapData()
	md.mu.Lock()
	defer md.mu.Unlock()

	// Validate state.
	switch b.mapState {
	case BufferMapStateDestroyed:
		return &BufferMapError{Kind: BufferMapErrKindDestroyed}
	case BufferMapStatePending:
		return &BufferMapError{Kind: BufferMapErrKindAlreadyPending}
	case BufferMapStateMapped:
		return &BufferMapError{Kind: BufferMapErrKindAlreadyMapped}
	}

	// Validate alignment (WebGPU MAP_ALIGNMENT=8, size multiple of 4).
	if offset%8 != 0 || size%4 != 0 {
		return &BufferMapError{Kind: BufferMapErrKindAlignment}
	}

	// Validate range.
	if offset > b.size || size > b.size || offset+size > b.size {
		return &BufferMapError{Kind: BufferMapErrKindRangeOverflow}
	}

	// Mode vs usage check.
	switch mode {
	case MapModeInternalRead:
		if !b.usage.Contains(gputypes.BufferUsageMapRead) {
			return &BufferMapError{Kind: BufferMapErrKindInvalidMode}
		}
	case MapModeInternalWrite:
		if !b.usage.Contains(gputypes.BufferUsageMapWrite) {
			return &BufferMapError{Kind: BufferMapErrKindInvalidMode}
		}
	default:
		return &BufferMapError{Kind: BufferMapErrKindInvalidMode}
	}

	// Record pending request and transition to Pending.
	md.pendingMode = mode
	md.pendingOffset = offset
	md.pendingSize = size
	md.waiter.Reset()
	b.mapState = BufferMapStatePending
	return nil
}

// ResolveMap transitions a Pending buffer to Mapped and populates the
// BufferMapping. Called by Device.PollMaps once the GPU fence has
// advanced past the submission index this buffer was registered on.
//
// If the HAL MapBuffer call fails, the buffer transitions back to Idle
// and the error is recorded for any waiter.
//
// Must be called with a SnatchGuard already held so the HAL buffer
// pointer cannot be snatched underneath us.
func (b *Buffer) ResolveMap(guard SnatchGuard, halDevice hal.Device) {
	md := b.ensureMapData()
	md.mu.Lock()

	if b.mapState != BufferMapStatePending {
		// Unmapped-during-pending or Destroyed-during-pending — deliver
		// the appropriate error to any waiter.
		var err *BufferMapError
		if b.mapState == BufferMapStateDestroyed {
			err = &BufferMapError{Kind: BufferMapErrKindDestroyed}
		} else {
			err = &BufferMapError{Kind: BufferMapErrKindCancelled}
		}
		waiter := md.waiter
		md.mu.Unlock()
		waiter.Signal(err)
		return
	}

	hbuf := b.raw.Get(guard)
	if hbuf == nil {
		b.mapState = BufferMapStateIdle
		waiter := md.waiter
		md.mu.Unlock()
		waiter.Signal(&BufferMapError{Kind: BufferMapErrKindDestroyed})
		return
	}
	mapping, err := halDevice.MapBuffer(*hbuf, md.pendingOffset, md.pendingSize)
	if err != nil {
		b.mapState = BufferMapStateIdle
		waiter := md.waiter
		md.mu.Unlock()
		waiter.Signal(&BufferMapError{Kind: BufferMapErrKindHAL, Wrapped: err})
		return
	}

	md.mapping = mapping
	b.mapState = BufferMapStateMapped
	waiter := md.waiter
	md.mu.Unlock()
	waiter.Signal(nil)
}

// UnmapBuffer transitions the buffer from Mapped or Pending back to Idle.
//
//   - Mapped  → Idle : hal.Device.UnmapBuffer is called, generation bumped,
//     outstanding MappedRange handles are detached.
//   - Pending → Idle : the pending request is canceled; the waiter
//     receives ErrMapCancelled.
//   - Idle    : returns ErrMapNotMapped.
//   - Destroyed : returns ErrBufferDestroyed.
func (b *Buffer) UnmapBuffer(guard SnatchGuard, halDevice hal.Device) *BufferMapError {
	md := b.ensureMapData()
	md.mu.Lock()

	switch b.mapState {
	case BufferMapStateDestroyed:
		md.mu.Unlock()
		return &BufferMapError{Kind: BufferMapErrKindDestroyed}
	case BufferMapStateIdle:
		md.mu.Unlock()
		return &BufferMapError{Kind: BufferMapErrKindNotMapped}
	case BufferMapStatePending:
		b.mapState = BufferMapStateIdle
		md.pendingMode = 0
		md.pendingOffset = 0
		md.pendingSize = 0
		md.generation.Add(1)
		waiter := md.waiter
		md.mu.Unlock()
		waiter.Signal(&BufferMapError{Kind: BufferMapErrKindCancelled})
		return nil
	case BufferMapStateMapped:
		// Detach all outstanding ranges and unmap the HAL buffer.
		md.mappedRanges = md.mappedRanges[:0]
		md.generation.Add(1)
		var hbuf *hal.Buffer
		if b.raw != nil {
			hbuf = b.raw.Get(guard)
		}
		md.mapping = hal.BufferMapping{}
		md.pendingOffset = 0
		md.pendingSize = 0
		md.pendingMode = 0
		b.mapState = BufferMapStateIdle
		md.mu.Unlock()

		if halDevice != nil && hbuf != nil {
			_ = halDevice.UnmapBuffer(*hbuf)
		}
		return nil
	}
	md.mu.Unlock()
	return &BufferMapError{Kind: BufferMapErrKindNotMapped}
}

// MarkDestroyed transitions the buffer to the terminal Destroyed state,
// detaches any outstanding MappedRange handles, and signals any waiter
// with ErrBufferDestroyed.
//
// Idempotent. Called from Buffer.Destroy before the HAL buffer is
// actually released.
func (b *Buffer) MarkDestroyed() {
	md := b.ensureMapData()
	md.mu.Lock()
	if b.mapState == BufferMapStateDestroyed {
		md.mu.Unlock()
		return
	}
	wasPending := b.mapState == BufferMapStatePending
	b.mapState = BufferMapStateDestroyed
	md.mappedRanges = md.mappedRanges[:0]
	md.generation.Add(1)
	waiter := md.waiter
	md.mu.Unlock()
	if wasPending {
		waiter.Signal(&BufferMapError{Kind: BufferMapErrKindDestroyed})
	}
}

// MappingInfo returns the base pointer, active offset, and active size
// for the mapped region. Returns ok=false if the buffer is not mapped.
func (b *Buffer) MappingInfo() (base unsafe.Pointer, offset, size uint64, ok bool) {
	md := b.loadMapData()
	if md == nil {
		return nil, 0, 0, false
	}
	md.mu.Lock()
	defer md.mu.Unlock()
	if b.mapState != BufferMapStateMapped {
		return nil, 0, 0, false
	}
	return md.mapping.Ptr, md.pendingOffset, md.pendingSize, true
}

// TryRegisterMappedRange records a new outstanding MappedRange with
// overlap detection. Must be called while the buffer is Mapped.
func (b *Buffer) TryRegisterMappedRange(offset, size uint64) *BufferMapError {
	md := b.ensureMapData()
	md.mu.Lock()
	defer md.mu.Unlock()
	if b.mapState != BufferMapStateMapped {
		return &BufferMapError{Kind: BufferMapErrKindNotMapped}
	}
	if offset < md.pendingOffset ||
		offset+size > md.pendingOffset+md.pendingSize ||
		offset+size < offset {
		return &BufferMapError{Kind: BufferMapErrKindRangeOverflow}
	}
	for _, r := range md.mappedRanges {
		if offset < r.offset+r.size && r.offset < offset+size {
			return &BufferMapError{Kind: BufferMapErrKindRangeOverlap}
		}
	}
	md.mappedRanges = append(md.mappedRanges, mappedRangeRec{offset: offset, size: size})
	return nil
}

// CurrentMapState returns the current state for diagnostics. This uses
// the mapData mutex if available so callers see a coherent snapshot.
func (b *Buffer) CurrentMapState() BufferMapState {
	md := b.loadMapData()
	if md == nil {
		return b.mapState
	}
	md.mu.Lock()
	s := b.mapState
	md.mu.Unlock()
	return s
}

// InstallMappedAtCreation is called by CreateBuffer when the buffer is
// created with MappedAtCreation=true. It transitions the new buffer to
// the Mapped state with an already-populated hal.BufferMapping so the
// caller can start writing via MappedRange immediately without going
// through Poll or waiting on a submission.
//
// The caller must hold the device snatch guard since this calls
// hal.Device.MapBuffer directly.
func (b *Buffer) InstallMappedAtCreation(guard SnatchGuard, halDevice hal.Device) error {
	md := b.ensureMapData()
	md.mu.Lock()
	defer md.mu.Unlock()
	if b.raw == nil {
		return &BufferMapError{Kind: BufferMapErrKindDestroyed}
	}
	hbuf := b.raw.Get(guard)
	if hbuf == nil {
		return &BufferMapError{Kind: BufferMapErrKindDestroyed}
	}
	mapping, err := halDevice.MapBuffer(*hbuf, 0, b.size)
	if err != nil {
		return &BufferMapError{Kind: BufferMapErrKindHAL, Wrapped: err}
	}
	md.mapping = mapping
	md.pendingOffset = 0
	md.pendingSize = b.size
	md.pendingMode = MapModeInternalWrite
	b.mapState = BufferMapStateMapped
	return nil
}
