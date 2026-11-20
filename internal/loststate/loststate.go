// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package loststate is the build-independent registration state machine for
// the public device-lost and uncaptured-error callbacks (P1-API-042, #417).
//
// The frozen contract (C5 of the shape document) is one linearization point
// for the latch and the registrations:
//   - the first MarkLost wins: a later reason never overwrites the stored one
//     and never re-fires;
//   - a setter takes effect for events after it returns and never replays a
//     delivered event to a replaced registration;
//   - a registration made on an already-lost device receives the stored info
//     exactly once, at the setter's return (the late-subscriber rule);
//   - an event captured for a registration is delivered to it at most once,
//     and a Set racing a dispatch never double-fires (both read the callback
//     and the delivered flag under the same lock that latches);
//   - no callback is invoked while the state's lock is held: setters return
//     the delivery for the caller to invoke after unlocking, and the recording
//     paths queue deliveries that TakePending drains at the observer's
//     top-level entry point, outside every lock a callback can re-enter.
//
// The native, rust and browser builds instantiate State with their own
// uncaptured-error payload type, so this code and its tests are the
// build-independent evidence of the registration contract.
package loststate

import "sync"

// Reason is why a device was lost, mirroring the WebGPU GPUDeviceLostReason
// concept (two values on every build; a driver loss is Unknown).
type Reason int

const (
	// ReasonUnknown is a driver-initiated loss.
	ReasonUnknown Reason = iota
	// ReasonDestroyed is the loss a (*Device).Release produces.
	ReasonDestroyed
)

// Info is one device-lost event's payload.
type Info struct {
	Reason  Reason
	Message string
}

// LostFire is one device-lost delivery the caller must invoke after releasing
// its locks.
type LostFire struct {
	Fn   func(Info)
	Info Info
}

// ErrorFire is one uncaptured-error delivery the caller must invoke after
// releasing its locks. E is the build's GPU error payload type.
type ErrorFire[E any] struct {
	Fn  func(E)
	Err E
}

// State is the per-device registration state machine. Construct it with New;
// the zero value is not usable by design (a nil *State is a programming
// error, never a silently unguarded device).
type State[E any] struct {
	mu sync.Mutex

	// lost and info hold the one-way latch. The first MarkLost wins.
	lost bool
	info Info

	// lostCb is the current lost registration; lostDelivered reports whether
	// the current registration already received the stored info, so a
	// registration is never delivered twice by racing dispatch paths.
	lostCb        func(Info)
	lostDelivered bool

	// errCb is the current uncaptured-error registration. An event is
	// captured with the registration current at RECORD time, so a replacement
	// between record and dispatch never receives a past event.
	errCb func(E)

	// pending queues deliveries recorded on paths that may hold locks; the
	// observer's top-level entry drains them with TakePending and invokes them
	// after its own locks are released.
	pendingLost []LostFire
	pendingErr  []ErrorFire[E]
}

// New returns an empty state machine.
func New[E any]() *State[E] {
	return &State[E]{}
}

// SetLostCallback replaces the lost registration (nil clears it without
// delivering anything). On an already-lost device it returns the stored-info
// delivery for the NEW registration, which the caller invokes after unlocking;
// a registration receives the stored info exactly once, and a replaced
// registration never receives a past event again.
func (s *State[E]) SetLostCallback(fn func(Info)) (LostFire, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lostCb = fn
	s.lostDelivered = false
	if !s.lost || fn == nil {
		return LostFire{}, false
	}
	s.lostDelivered = true
	return LostFire{Fn: fn, Info: s.info}, true
}

// MarkLost records a loss. The first observation wins: a later call does not
// change the stored reason and does not queue anything. When a registration
// is current and has not been delivered, the delivery is queued for the
// observer's TakePending — never invoked here, because the recording path may
// hold the device, queue or encoder lock.
func (s *State[E]) MarkLost(reason Reason, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lost {
		return
	}
	s.lost = true
	s.info = Info{Reason: reason, Message: message}
	if s.lostCb != nil && !s.lostDelivered {
		s.lostDelivered = true
		s.pendingLost = append(s.pendingLost, LostFire{Fn: s.lostCb, Info: s.info})
	}
}

// LostInfo reports the current lost state; ok is false on a live device.
func (s *State[E]) LostInfo() (Info, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info, s.lost
}

// SetUncapturedErrorCallback replaces the uncaptured-error registration (nil
// clears it). It never replays an event already delivered.
func (s *State[E]) SetUncapturedErrorCallback(fn func(E)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errCb = fn
}

// RecordUncaptured queues err for the registration current at record time.
// With no registration the event is dropped (the spec's console-report case,
// with no console here). The delivery is drained by TakePending.
func (s *State[E]) RecordUncaptured(err E) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errCb == nil {
		return
	}
	s.pendingErr = append(s.pendingErr, ErrorFire[E]{Fn: s.errCb, Err: err})
}

// TakePending drains the queued deliveries in record order. The caller invokes
// them after releasing every lock it holds, on the goroutine of the observing
// call that drains them — a concurrent observing call may drain first, and they
// never run on a background thread.
func (s *State[E]) TakePending() (lost []LostFire, errors []ErrorFire[E]) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lost = s.pendingLost
	errors = s.pendingErr
	s.pendingLost = nil
	s.pendingErr = nil
	return lost, errors
}
