//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package browser

// Seam tests for the browser device-lost and uncaptured-error wiring
// (P1-API-042 / #417). The state machine itself is proven build-independently
// in internal/loststate; these tests pin the browser glue: the JS reason
// mapping, the handler latch, the record-then-dispatch split and C2's
// lost-first PopErrorScope answer. They run under node
// (GOOS=js GOARCH=wasm go test) but NOT in this repo's CI, which executes only
// the root and encoder wasm suites (P2-INFRA-014); the live GPUDevice promise
// and event path is a documented limit, exercised only in a real browser.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/internal/loststate"
)

// lostTestDevice builds the in-package seam fixture: a device with a live lost
// state machine and no JS object. Every method under test that does not read a
// JS property works on it.
func lostTestDevice() *Device {
	return &Device{lost: loststate.New[*GPUErrorInfo]()}
}

func TestLostReasonFromJS(t *testing.T) {
	cases := []struct {
		reason string
		want   loststate.Reason
	}{
		{"destroyed", loststate.ReasonDestroyed},
		{"unknown", loststate.ReasonUnknown},
		{"", loststate.ReasonUnknown},
		{"something-else", loststate.ReasonUnknown},
	}
	for _, c := range cases {
		if got := lostReasonFromJS(c.reason); got != c.want {
			t.Errorf("lostReasonFromJS(%q) = %d; want %d", c.reason, int(got), int(c.want))
		}
	}
}

// TestLostCallbackLatchOnce pins the handler latch and first-wins: the loss is
// recorded once, a later destroyed reason never overwrites it and never
// re-fires.
func TestLostCallbackLatchOnce(t *testing.T) {
	d := lostTestDevice()
	var delivered int
	d.SetLostCallback(func(info loststate.Info) {
		delivered++
		if info.Reason != loststate.ReasonUnknown {
			t.Errorf("delivered reason %d; want Unknown", int(info.Reason))
		}
	})

	d.markLost(loststate.ReasonUnknown, "driver reset")
	d.markLost(loststate.ReasonDestroyed, "destroyed after")

	if delivered != 1 {
		t.Fatalf("deliveries = %d; want 1", delivered)
	}
	info, ok := d.LostInfo()
	if !ok || info.Reason != loststate.ReasonUnknown || info.Message != "driver reset" {
		t.Fatalf("LostInfo() = %+v, %v; want Unknown/driver reset, true", info, ok)
	}
}

// TestLateSubscriberFiresOnce pins the late-subscriber rule through the browser
// seam: a registration on an already-lost device receives the stored info once
// at the setter's return, and a replacement receives it once too.
func TestLateSubscriberFiresOnce(t *testing.T) {
	d := lostTestDevice()
	d.markLost(loststate.ReasonDestroyed, "gone")

	var first int
	d.SetLostCallback(func(loststate.Info) { first++ })
	if first != 1 {
		t.Fatalf("late registration deliveries = %d; want 1", first)
	}

	var second int
	d.SetLostCallback(func(loststate.Info) { second++ })
	if second != 1 {
		t.Fatalf("replacement deliveries = %d; want 1", second)
	}
	if first != 1 {
		t.Fatalf("replaced registration deliveries = %d; want 1 (no replay)", first)
	}
}

// TestUncapturedRouting pins the record-then-dispatch path and the no-replay
// rule for the uncaptured-error registration.
func TestUncapturedRouting(t *testing.T) {
	d := lostTestDevice()
	var first, second int
	d.SetUncapturedErrorCallback(func(info *GPUErrorInfo) {
		first++
		if info == nil || info.Type != ErrorFilterValidation || info.Message != "boom" {
			t.Errorf("delivered %+v; want a Validation/boom error", info)
		}
	})
	d.recordUncaptured(&GPUErrorInfo{Type: ErrorFilterValidation, Message: "boom"})
	if first != 1 {
		t.Fatalf("first registration deliveries = %d; want 1", first)
	}

	d.SetUncapturedErrorCallback(func(*GPUErrorInfo) { second++ })
	if first != 1 || second != 0 {
		t.Fatalf("after replacement: first = %d, second = %d; want 1, 0", first, second)
	}
}

// TestPopErrorScopeLostFirst pins C2 on the browser: a lost device's pop answers
// (nil, nil) — the spec's null — and never ErrErrorScopeEmpty, before any JS
// promise is awaited. It is the browser half of the four-case pop contract.
func TestPopErrorScopeLostFirst(t *testing.T) {
	d := lostTestDevice()
	d.markLost(loststate.ReasonDestroyed, "gone")

	info, err := d.PopErrorScope()
	if info != nil {
		t.Fatalf("PopErrorScope() info = %+v; want nil", info)
	}
	if err != nil {
		t.Fatalf("PopErrorScope() error = %v; want nil on a lost device", err)
	}
	if errors.Is(err, ErrErrorScopeEmpty) {
		t.Fatal("PopErrorScope() matched ErrErrorScopeEmpty on a lost device")
	}
}

// TestNilDeviceSeamIsSafe pins the nil-receiver guards the root layer relies on.
func TestNilDeviceSeamIsSafe(t *testing.T) {
	var d *Device
	d.SetLostCallback(func(loststate.Info) { t.Fatal("nil device delivered a loss") })
	d.SetUncapturedErrorCallback(func(*GPUErrorInfo) { t.Fatal("nil device delivered an error") })
	d.DispatchPending()
	if _, ok := d.LostInfo(); ok {
		t.Fatal("nil device reported a loss")
	}
}
