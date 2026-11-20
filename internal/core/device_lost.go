//go:build !(js && wasm)

package core

import (
	"sync/atomic"
	"unsafe"

	"github.com/gogpu/wgpu/internal/loststate"
)

// This file is the core device's half of the device-lost and uncaptured-error
// callbacks (P1-API-042 / #417, cards P1-API-042 and P1-API-039 D2). It owns
// the one-way loss latch, the two registrations and the two-phase dispatch; the
// registration contract itself (first-wins, per-registration delivery, no
// callback under a lock) is internal/loststate's, and the per-build adapters
// convert its payload.
//
// Two-phase dispatch is the whole point of the split (C4): a recording path
// may hold the device's, the queue's or the encoder's lock, so it only QUEUES a
// delivery here. The outermost entry point of the observing call drains the
// queue with DispatchPending after releasing its own locks and invokes it.
//
// The state lives in a one-word pointer field, exactly like errorScopeManager:
// the legacy ID-based Hub passes Device by value, so no noCopy type may appear
// on the struct. The pointer is shared by every value copy, which is what the
// copyable API needs.

// lostState returns the device's device-lost and uncaptured-error state
// machine, installing it on first use with a CAS.
//
// The slot is a plain word and the install is atomic for the same reason
// pendingMapTrackerSlot's is: Device is passed by value in the legacy ID-based
// API, so no noCopy type may appear on the struct, and not every construction
// path runs a core constructor — the HAL-less/mock RequestDevice path builds a
// zeroed Device — so the lazy branch is reachable on a concurrently used
// public device. Without the CAS, two goroutines' first uses could race on the
// slot and one registration would be lost (review P2-1, reproduced under -race).
func (d *Device) lostState() *loststate.State[*GPUError] {
	if p := atomic.LoadPointer(&d.lost); p != nil {
		return (*loststate.State[*GPUError])(p)
	}
	fresh := loststate.New[*GPUError]()
	//nolint:gosec // deliberate: the slot stays a plain word so Device remains copyable for RegisterDevice's value semantics; it only ever holds a *loststate.State[*GPUError]
	if atomic.CompareAndSwapPointer(&d.lost, nil, unsafe.Pointer(fresh)) {
		return fresh
	}
	return (*loststate.State[*GPUError])(atomic.LoadPointer(&d.lost))
}

// SetLostCallback replaces the device's device-lost registration; nil clears it
// without delivering anything.
//
// A registration made on an already-lost device receives the stored info
// exactly once: the delivery is returned here for the caller to invoke after
// unlocking, and a replaced registration never receives a past event again.
// This method itself holds no lock at its return, so a caller may simply invoke
// the returned delivery (the internal/device layer does).
func (d *Device) SetLostCallback(fn func(loststate.Info)) (loststate.LostFire, bool) {
	if d == nil {
		return loststate.LostFire{}, false
	}
	return d.lostState().SetLostCallback(fn)
}

// SetUncapturedErrorCallback replaces the device's uncaptured-error
// registration; nil clears it. It never replays an event already delivered.
func (d *Device) SetUncapturedErrorCallback(fn func(*GPUError)) {
	if d == nil {
		return
	}
	d.lostState().SetUncapturedErrorCallback(fn)
}

// LostInfo reports the device's current lost state; ok is false on a live
// device. It is the late subscriber's answer and the four-case PopErrorScope's
// first check (C2).
func (d *Device) LostInfo() (loststate.Info, bool) {
	if d == nil {
		return loststate.Info{}, false
	}
	return d.lostState().LostInfo()
}

// MarkLost records a loss. The first observation wins (C9/C10): a later call —
// including the (*Device).Release destroy report — never changes the stored
// reason and never re-fires.
//
// The delivery is only QUEUED here; the observing entry point drains it with
// DispatchPending after releasing its locks (C4).
func (d *Device) MarkLost(reason loststate.Reason, message string) {
	if d == nil {
		return
	}
	d.lostState().MarkLost(reason, message)
}

// DispatchPending delivers every event the recording paths queued, in record
// order, and invokes the callbacks. Call it only where no device, queue, core
// or error-scope lock is held: a lost callback may re-enter read-only accessors
// (LostInfo, Features), and an uncaptured-error callback runs on the goroutine
// that observed the event.
func (d *Device) DispatchPending() {
	if d == nil {
		return
	}
	lostFires, errFires := d.lostState().TakePending()
	for _, fire := range lostFires {
		if fire.Fn != nil {
			fire.Fn(fire.Info)
		}
	}
	for _, fire := range errFires {
		if fire.Fn != nil {
			fire.Fn(fire.Err)
		}
	}
}
