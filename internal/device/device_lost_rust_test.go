//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// Tests for the rust build's device-lost wiring (P1-API-042 / #417). At the
// pinned webgpu v0.5.5 the binding exposes neither a device-lost nor an
// uncaptured-error callback, so the observable source is Release; the tests
// below are runnable without the native library because they never reach the
// FFI boundary (NewRust(nil, ...) and the lost-first pop guard see to that).

import (
	"errors"
	"testing"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/loststate"
)

func TestRustLostInfoAfterRelease(t *testing.T) {
	d := NewRust(nil, 0, gputypes.Limits{})
	if _, ok := d.LostInfo(); ok {
		t.Fatal("LostInfo() ok = true on a live device")
	}

	d.Release()

	info, ok := d.LostInfo()
	if !ok {
		t.Fatal("LostInfo() ok = false after Release; want the Destroyed loss")
	}
	if info.Reason != loststate.ReasonDestroyed {
		t.Fatalf("LostInfo() reason = %d; want Destroyed", int(info.Reason))
	}
	if info.Message == "" {
		t.Fatal("LostInfo() message is empty; want the release reason text")
	}
}

// TestRustLostCallbackFiresOnceOnRelease pins C10 and the per-registration
// bound: Release delivers once, a second Release delivers nothing, and a
// replacement on the lost device receives the stored info once (no replay of
// the replaced registration).
func TestRustLostCallbackFiresOnceOnRelease(t *testing.T) {
	d := NewRust(nil, 0, gputypes.Limits{})

	var first int
	d.SetLostCallback(func(info loststate.Info) {
		first++
		if info.Reason != loststate.ReasonDestroyed {
			t.Errorf("delivered reason %d; want Destroyed", int(info.Reason))
		}
	})

	d.Release()
	if first != 1 {
		t.Fatalf("deliveries after Release = %d; want 1", first)
	}
	d.Release()
	if first != 1 {
		t.Fatalf("deliveries after second Release = %d; want 1", first)
	}

	var second int
	d.SetLostCallback(func(loststate.Info) { second++ })
	if second != 1 {
		t.Fatalf("late registration deliveries = %d; want 1", second)
	}
	if first != 1 {
		t.Fatalf("replaced registration deliveries = %d; want 1 (no replay)", first)
	}
}

// TestRustFirstLossWinsOverRelease pins C9: a loss recorded earlier keeps its
// reason when Release follows, and Release does not re-deliver.
func TestRustFirstLossWinsOverRelease(t *testing.T) {
	d := NewRust(nil, 0, gputypes.Limits{})
	var delivered int
	d.SetLostCallback(func(loststate.Info) { delivered++ })

	d.markLost(loststate.ReasonUnknown, "driver reset")
	d.Release()

	info, ok := d.LostInfo()
	if !ok || info.Reason != loststate.ReasonUnknown || info.Message != "driver reset" {
		t.Fatalf("LostInfo() = %+v, %v; want the earlier Unknown/driver reset loss", info, ok)
	}
	if delivered != 1 {
		t.Fatalf("deliveries = %d; want 1", delivered)
	}
}

// TestRustPopLostFirst pins C2 on the rust seam: a lost device's pop answers
// (nil, nil) and never the empty-stack sentinel, before any binding call.
func TestRustPopLostFirst(t *testing.T) {
	d := NewRust(nil, 0, gputypes.Limits{})
	d.Release()

	e, err := d.PopErrorScope()
	if e != nil || err != nil {
		t.Fatalf("PopErrorScope() = (%v, %v); want (nil, nil) on a lost device", e, err)
	}
	if errors.Is(err, core.ErrErrorScopeEmpty) {
		t.Fatal("PopErrorScope() matched ErrErrorScopeEmpty on a lost device")
	}
}

// TestRustPopClassifier pins the four-case classification on a settled pop:
// captured nothing, captured, and the binding's own failure (where this build's
// not-lost empty stack arrives).
func TestRustPopClassifier(t *testing.T) {
	e, err := classifyRustPop(rwgpu.ErrorTypeNoError, "", nil)
	if e != nil || err != nil {
		t.Fatalf("no-error case = (%v, %v); want (nil, nil)", e, err)
	}

	e, err = classifyRustPop(rwgpu.ErrorTypeValidation, "bad", nil)
	if err != nil {
		t.Fatalf("captured case error = %v; want nil", err)
	}
	if e == nil || e.Type != core.ErrorFilterValidation || e.Message != "bad" {
		t.Fatalf("captured case = %+v; want Validation/bad", e)
	}

	bindingErr := errors.New("binding: empty stack")
	e, err = classifyRustPop(rwgpu.ErrorTypeNoError, "", bindingErr)
	if e != nil {
		t.Fatalf("binding-failure case error payload = %+v; want nil", e)
	}
	if !errors.Is(err, bindingErr) {
		t.Fatalf("binding-failure case error = %v; want it to wrap the binding error", err)
	}
}

// TestRustNilDeviceSeamIsSafe pins the nil-receiver guards.
func TestRustNilDeviceSeamIsSafe(t *testing.T) {
	var d *Device
	d.SetLostCallback(func(loststate.Info) { t.Fatal("nil device delivered a loss") })
	d.SetUncapturedErrorCallback(func(*core.GPUError) { t.Fatal("nil device delivered an error") })
	d.DispatchPending()
	if _, ok := d.LostInfo(); ok {
		t.Fatal("nil device reported a loss")
	}
	if e, err := d.PopErrorScope(); e != nil || err != nil {
		t.Fatalf("nil PopErrorScope() = (%v, %v); want (nil, nil)", e, err)
	}
	d.Release()
}
