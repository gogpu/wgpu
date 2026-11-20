//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

// The device-lost and uncaptured-error contract at the core layer
// (P1-API-042 / #417): the four-case lost-first PopErrorScope (C1/C2), the
// one-way latch (C9/C10), the uncaptured/captured split of one error report
// (D6, C5 of the shape), and the zero-value answers.
//
// Each case names what it fails on the pre-#417 code: PopErrorScope answered
// three cases, so a lost device returned the captured error or
// ErrErrorScopeEmpty, and reportError dropped the uncaptured error entirely.

import (
	"errors"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/loststate"
)

// newLostTestDevice builds a constructed (not zero-value) device: the loss
// state is constructor-initialized, which is the path production devices take.
// The HAL double's zero limits keep indirect-dispatch validation off, so the
// device comes up with no GPU resources.
func newLostTestDevice(t *testing.T) *Device {
	t.Helper()
	d := NewDevice(&mockHALDevice{}, nil, 0, gputypes.Limits{}, "lost-test")
	t.Cleanup(d.Destroy)
	return d
}

// TestPopErrorScopeLostIsNilNil pins the lost-first FOUR-case contract (C1/C2):
// a lost device resolves with (nil, nil) whatever the stack holds, the stack is
// not drained, and ErrErrorScopeEmpty is never the lost device's answer.
func TestPopErrorScopeLostIsNilNil(t *testing.T) {
	t.Run("lost with a captured error answers nil and leaves the stack", func(t *testing.T) {
		d := newLostTestDevice(t)
		if err := d.PushErrorScope(ErrorFilterValidation); err != nil {
			t.Fatalf("PushErrorScope: %v", err)
		}
		if !d.ReportError(ErrorFilterValidation, "captured before the loss") {
			t.Fatal("precondition: the pushed scope did not capture the error")
		}
		d.MarkLost(loststate.ReasonUnknown, "driver gone")

		e, err := d.PopErrorScope()
		if e != nil || err != nil {
			t.Fatalf("PopErrorScope on a lost device = (%v, %v), want (nil, nil) — NOT the captured error and NOT ErrErrorScopeEmpty", e, err)
		}
		if depth := d.errorScopes().ScopeDepth(); depth != 1 {
			t.Fatalf("scope depth after a lost pop = %d, want 1: the stack must stay untouched (C2)", depth)
		}
	})

	t.Run("lost with an empty stack is not the OperationError case", func(t *testing.T) {
		d := newLostTestDevice(t)
		d.MarkLost(loststate.ReasonUnknown, "driver gone")

		e, err := d.PopErrorScope()
		if e != nil || err != nil {
			t.Fatalf("PopErrorScope on a lost device with no scope = (%v, %v), want (nil, nil), never ErrErrorScopeEmpty", e, err)
		}
	})

	t.Run("push after the loss then pop answers nil", func(t *testing.T) {
		d := newLostTestDevice(t)
		d.MarkLost(loststate.ReasonUnknown, "driver gone")
		if err := d.PushErrorScope(ErrorFilterValidation); err != nil {
			t.Fatalf("PushErrorScope on a lost device: %v (the spec pushes and returns, no throw)", err)
		}
		// The scope is dead: an error reported on a lost device never captures,
		// so this is observationally the spec's dead scope.
		if d.ReportError(ErrorFilterValidation, "reported on a lost device") {
			t.Fatal("a scope pushed on a lost device captured an error")
		}
		e, err := d.PopErrorScope()
		if e != nil || err != nil {
			t.Fatalf("PopErrorScope of a post-loss scope = (%v, %v), want (nil, nil)", e, err)
		}
	})
}

// TestPopErrorScopeLiveUnchanged pins the three not-lost cases the reshape must
// leave exactly as they were.
func TestPopErrorScopeLiveUnchanged(t *testing.T) {
	t.Run("captured error is returned", func(t *testing.T) {
		d := newLostTestDevice(t)
		if err := d.PushErrorScope(ErrorFilterValidation); err != nil {
			t.Fatalf("PushErrorScope: %v", err)
		}
		if !d.ReportError(ErrorFilterValidation, "bad usage") {
			t.Fatal("the scope did not capture the reported error")
		}

		e, err := d.PopErrorScope()
		if err != nil {
			t.Fatalf("PopErrorScope error = %v, want nil", err)
		}
		if e == nil || e.Message != "bad usage" || e.Type != ErrorFilterValidation {
			t.Fatalf("PopErrorScope = %+v, want the captured validation error", e)
		}
	})

	t.Run("a scope that captured nothing answers nil", func(t *testing.T) {
		d := newLostTestDevice(t)
		if err := d.PushErrorScope(ErrorFilterInternal); err != nil {
			t.Fatalf("PushErrorScope: %v", err)
		}
		e, err := d.PopErrorScope()
		if e != nil || err != nil {
			t.Fatalf("PopErrorScope = (%v, %v), want (nil, nil)", e, err)
		}
	})

	t.Run("an empty stack is the OperationError case", func(t *testing.T) {
		d := newLostTestDevice(t)
		e, err := d.PopErrorScope()
		if e != nil || !errors.Is(err, ErrErrorScopeEmpty) {
			t.Fatalf("PopErrorScope with no scope = (%v, %v), want (nil, ErrErrorScopeEmpty)", e, err)
		}
	})
}

// TestMarkLostFirstWins pins the one-way latch (C9/C10): the first observation
// is the stored one, a later reason is ignored, and the loss event fires once.
func TestMarkLostFirstWins(t *testing.T) {
	d := newLostTestDevice(t)
	var mu sync.Mutex
	var fired []loststate.Info
	if fire, ok := d.SetLostCallback(func(info loststate.Info) {
		mu.Lock()
		defer mu.Unlock()
		fired = append(fired, info)
	}); ok {
		t.Fatalf("a live device produced a late-subscriber delivery: %+v", fire)
	}

	d.MarkLost(loststate.ReasonUnknown, "driver gone")
	d.DispatchPending()
	d.MarkLost(loststate.ReasonDestroyed, "device released")
	d.DispatchPending()

	info, ok := d.LostInfo()
	if !ok || info.Reason != loststate.ReasonUnknown || info.Message != "driver gone" {
		t.Fatalf("LostInfo = (%+v, %v), want the first (Unknown) observation", info, ok)
	}
	mu.Lock()
	n := len(fired)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("lost callback fired %d times, want exactly 1: a later reason never re-fires", n)
	}
}

// TestLateSubscriberDeliveryIsPerRegistration pins the C5 registration rule at
// the core setter: a registration made after the loss is handed the stored info
// exactly once for the CALLER to invoke, and a replaced registration gets no
// second delivery. A global (not per-registration) delivered flag fails the
// second assertion: the new registration would be handed nothing.
func TestLateSubscriberDeliveryIsPerRegistration(t *testing.T) {
	d := newLostTestDevice(t)
	firstFires := 0
	if _, ok := d.SetLostCallback(func(loststate.Info) { firstFires++ }); ok {
		t.Fatal("a live device delivered the stored info to a registration")
	}
	d.MarkLost(loststate.ReasonUnknown, "driver gone")
	d.DispatchPending()
	if firstFires != 1 {
		t.Fatalf("the first registration fired %d times, want 1", firstFires)
	}

	secondFires := 0
	var secondInfo loststate.Info
	fire, ok := d.SetLostCallback(func(info loststate.Info) { secondFires++; secondInfo = info })
	if !ok {
		t.Fatal("a registration made after the loss was handed no delivery: lostDelivered must reset per registration (C5), not once per device")
	}
	fire.Fn(fire.Info) // what the device layer does at the setter's return
	if secondFires != 1 {
		t.Fatalf("the late registration fired %d times, want 1", secondFires)
	}
	if secondInfo.Reason != loststate.ReasonUnknown || secondInfo.Message != "driver gone" {
		t.Fatalf("late registration info = %+v, want the stored Unknown observation", secondInfo)
	}

	d.DispatchPending()
	if firstFires != 1 || secondFires != 1 {
		t.Fatalf("after another dispatch: first=%d second=%d, want 1 and 1 (the replaced registration never fires again)", firstFires, secondFires)
	}
}

// TestUncapturedErrorReachesTheCallbackWithClassIdentity pins D6: the
// uncaptured callback receives the SAME *GPUError class the scopes produce
// (errors.Is against the category sentinel), and an error a scope captured is
// never delivered to it.
func TestUncapturedErrorReachesTheCallbackWithClassIdentity(t *testing.T) {
	d := newLostTestDevice(t)
	var mu sync.Mutex
	var got []*GPUError
	d.SetUncapturedErrorCallback(func(e *GPUError) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})

	if d.ReportError(ErrorFilterValidation, "uncaptured bad usage") {
		t.Fatal("an error reported with no scope open was captured")
	}
	d.DispatchPending()

	mu.Lock()
	if len(got) != 1 {
		mu.Unlock()
		t.Fatalf("uncaptured callback fired %d times, want 1", len(got))
	}
	first := got[0]
	mu.Unlock()
	if !errors.Is(first, ErrValidation) {
		t.Fatalf("errors.Is(delivered, ErrValidation) = false for %v: the callback must share the scope class identity", first)
	}
	if first.Message != "uncaptured bad usage" {
		t.Fatalf("delivered message = %q, want the reported text", first.Message)
	}

	// A captured error is delivered to the scope only.
	if err := d.PushErrorScope(ErrorFilterValidation); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}
	if !d.ReportError(ErrorFilterValidation, "captured bad usage") {
		t.Fatal("the open scope did not capture the reported error")
	}
	d.DispatchPending()
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("uncaptured callback fired %d times after a captured error, want still 1", n)
	}
	e, err := d.PopErrorScope()
	if err != nil || e == nil || e.Message != "captured bad usage" {
		t.Fatalf("PopErrorScope = (%+v, %v), want the captured error", e, err)
	}
}

// TestLostStateZeroValueContract pins the nil-safe answers the lazy accessor
// gives a zero-value (or nil) device, matching the errorScopeManager precedent.
func TestLostStateZeroValueContract(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var d *Device
		if _, ok := d.LostInfo(); ok {
			t.Fatal("a nil device reported a loss")
		}
		d.MarkLost(loststate.ReasonUnknown, "never recorded")
		d.SetUncapturedErrorCallback(nil)
		d.DispatchPending()
		if fire, ok := d.SetLostCallback(nil); ok {
			t.Fatalf("a nil device produced a delivery: %+v", fire)
		}
		e, err := d.PopErrorScope()
		if e != nil || err != nil {
			t.Fatalf("PopErrorScope on a nil device = (%v, %v), want (nil, nil)", e, err)
		}
	})

	t.Run("zero struct", func(t *testing.T) {
		d := &Device{}
		if _, ok := d.LostInfo(); ok {
			t.Fatal("a zero-value device reported a loss")
		}
		d.MarkLost(loststate.ReasonUnknown, "zero value")
		info, ok := d.LostInfo()
		if !ok || info.Message != "zero value" {
			t.Fatalf("LostInfo after MarkLost on a zero-value device = (%+v, %v)", info, ok)
		}
	})
}
