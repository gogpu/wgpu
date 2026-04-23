//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"sync"
	"sync/atomic"

	"github.com/gogpu/wgpu/hal"
)

// FEAT-WGPU-MAPPING-001 — per-Device pending-map triage.
//
// Mirrors the relevant slice of Rust wgpu-core's LifetimeTracker
// (wgpu-core/src/device/life.rs:319). For every submission that has
// in-flight Buffer.Map requests we keep a list of buffers to resolve
// once the HAL fence advances past that submission index.
//
// Concurrency: pendingMapsMu is a leaf lock. It MUST NOT be held while
// calling any BufferMapData.mu methods — resolve() drops it before
// touching per-buffer state. This avoids the textbook lock-inversion
// deadlock.

// deviceMapTracker is reached from core.Device through an indirection
// slot so the outer Device struct remains copyable (required by the
// legacy ID-based Hub API). Lazily allocated on first RegisterPendingMap
// call.
//
// The tracker maintains a free-list of empty buffer slices so that
// PollMaps can recycle the slice capacity back to RegisterPendingMap —
// this keeps the steady-state per-op allocation count at zero once the
// working set is warm.
type deviceMapTracker struct {
	mu          sync.Mutex
	buckets     map[uint64][]*Buffer // submissionIndex -> waiting buffers
	free        [][]*Buffer          // recycled empty slices (cap > 0)
	maxKnownSub uint64               // highest submission index seen via Register
}

// mapTrackerSlot holds an atomic.Pointer[deviceMapTracker]. The outer
// Device holds *mapTrackerSlot (copyable), and the atomic lives inside
// the slot for lock-free lazy initialization.
type mapTrackerSlot struct {
	ptr atomic.Pointer[deviceMapTracker]
}

func newDeviceMapTracker() *deviceMapTracker {
	return &deviceMapTracker{
		buckets: make(map[uint64][]*Buffer, 8),
	}
}

// mapTracker returns the device's pending-map tracker, creating it on
// first use via compare-and-swap. Called from RegisterPendingMap;
// PollMaps can then observe the already-initialized pointer.
func (d *Device) mapTracker() *deviceMapTracker {
	slot := d.pendingMapTrackerSlot
	if slot == nil {
		slot = &mapTrackerSlot{}
		d.pendingMapTrackerSlot = slot
	}
	if t := slot.ptr.Load(); t != nil {
		return t
	}
	fresh := newDeviceMapTracker()
	if slot.ptr.CompareAndSwap(nil, fresh) {
		return fresh
	}
	return slot.ptr.Load()
}

// RegisterPendingMap queues a Buffer for map resolution after the GPU
// completes the given submission index. Called by wgpu.Buffer.MapAsync
// right after the state machine transitions to Pending.
//
// If submissionIndex is 0 the mapping can be resolved immediately — this
// is the path for buffers that have never been submitted to a queue, for
// example a MAP_READ staging buffer that the caller writes to before any
// Submit call. In that case the caller should follow up with
// PollMaps(completedIdx=0) or device.Poll which will drain the bucket.
func (d *Device) RegisterPendingMap(submissionIndex uint64, buf *Buffer) {
	t := d.mapTracker()
	t.mu.Lock()
	existing, ok := t.buckets[submissionIndex]
	if !ok {
		// Reuse a slice from the free-list when available. This keeps
		// the per-op allocation count at zero after the first few
		// cycles once the working set is warm.
		if n := len(t.free); n > 0 {
			existing = t.free[n-1]
			t.free = t.free[:n-1]
		}
	}
	t.buckets[submissionIndex] = append(existing, buf)
	if submissionIndex > t.maxKnownSub {
		t.maxKnownSub = submissionIndex
	}
	t.mu.Unlock()
}

// loadMapTracker returns the tracker if one has been installed or nil
// otherwise. Read-only accessor used by PollMaps / HasPendingMaps /
// MaxKnownSubmissionIndex.
func (d *Device) loadMapTracker() *deviceMapTracker {
	if d.pendingMapTrackerSlot == nil {
		return nil
	}
	return d.pendingMapTrackerSlot.ptr.Load()
}

// PollMaps resolves every pending map whose submission index is <=
// completedIdx. For each drained buffer the HAL MapBuffer call is issued
// and the buffer's waiter is signaled.
//
// Returns true if any pending maps were resolved.
//
// Thread-safe for concurrent calls: the first goroutine to observe a
// bucket drains it; subsequent goroutines see an empty bucket.
func (d *Device) PollMaps(completedIdx uint64) (didWork bool) {
	t := d.loadMapTracker()
	if t == nil {
		return false
	}

	// Process each completed bucket in turn. We drop and re-acquire the
	// tracker lock around the HAL MapBuffer calls so long resolves do
	// not block RegisterPendingMap calls from other goroutines.
	if d.snatchLock == nil || d.raw == nil {
		return false
	}
	guard := d.snatchLock.Read()
	defer guard.Release()
	hd := d.raw.Get(guard)
	if hd == nil {
		// Device destroyed — drain everything and signal DeviceLost.
		t.mu.Lock()
		for idx, bufs := range t.buckets {
			if idx > completedIdx {
				continue
			}
			for _, buf := range bufs {
				if buf == nil {
					continue
				}
				md := buf.ensureMapData()
				md.waiter.Signal(&BufferMapError{Kind: BufferMapErrKindDeviceLost})
			}
			t.free = append(t.free, bufs[:0])
			delete(t.buckets, idx)
			didWork = true
		}
		t.mu.Unlock()
		return didWork
	}
	halDevice := *hd

	// Drain buckets in a loop: extract one bucket under the lock,
	// resolve it outside the lock, then recycle its backing slice.
	for {
		var bucket []*Buffer
		var found bool
		t.mu.Lock()
		for idx, bufs := range t.buckets {
			if idx > completedIdx {
				continue
			}
			bucket = bufs
			delete(t.buckets, idx)
			found = true
			break
		}
		t.mu.Unlock()
		if !found {
			return didWork
		}
		for _, buf := range bucket {
			if buf == nil {
				continue
			}
			buf.ResolveMap(guard, halDevice)
		}
		t.mu.Lock()
		t.free = append(t.free, bucket[:0])
		t.mu.Unlock()
		didWork = true
	}
}

// HasPendingMaps reports whether the device has any outstanding map
// requests. Used by tests and by wgpu.Device.Poll(PollWait) to decide
// whether it's worth walking the fence.
func (d *Device) HasPendingMaps() bool {
	t := d.loadMapTracker()
	if t == nil {
		return false
	}
	t.mu.Lock()
	n := len(t.buckets)
	t.mu.Unlock()
	return n > 0
}

// MaxKnownSubmissionIndex returns the highest submission index that has
// ever been registered for pending map resolution. Used by the public
// Device.Poll(PollWait) to determine what fence value to wait on.
func (d *Device) MaxKnownSubmissionIndex() uint64 {
	t := d.loadMapTracker()
	if t == nil {
		return 0
	}
	t.mu.Lock()
	m := t.maxKnownSub
	t.mu.Unlock()
	return m
}

// HalDeviceHandle returns a HAL device pointer via a short-lived snatch
// guard. Used by wgpu.Device.Poll(PollWait) to invoke WaitIdle when the
// caller wants a blocking drain.
func (d *Device) HalDeviceHandle() (hal.Device, bool) {
	if d.snatchLock == nil || d.raw == nil {
		return nil, false
	}
	guard := d.snatchLock.Read()
	defer guard.Release()
	h := d.raw.Get(guard)
	if h == nil {
		return nil, false
	}
	return *h, true
}
