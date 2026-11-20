//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// Falsifiable tests for the rust build's public device-lost surface
// (P1-API-042 / #417). At the pinned webgpu v0.5.5 the only observable loss is
// the one Release produces, so these tests drive it without the native library:
// the device is built over a nil binding and never crosses the FFI boundary.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/hal"
)

func newRustTestDevice() *Device {
	return &Device{impl: device.NewRust(nil, 0, gputypes.Limits{})}
}

// TestRustPublicLostInfoAfterRelease pins the public LostInfo contract on the
// rust build: false while alive, then (true, Destroyed) after Release, and a
// second Release changes nothing.
func TestRustPublicLostInfoAfterRelease(t *testing.T) {
	d := newRustTestDevice()
	if _, ok := d.LostInfo(); ok {
		t.Fatal("LostInfo() ok = true on a live device")
	}

	d.Release()

	info, ok := d.LostInfo()
	if !ok {
		t.Fatal("LostInfo() ok = false after Release; want Destroyed")
	}
	if info.Reason != DeviceLostReasonDestroyed {
		t.Fatalf("LostInfo() reason = %d; want DeviceLostReasonDestroyed", int(info.Reason))
	}

	d.Release()
	if again, ok := d.LostInfo(); !ok || again != info {
		t.Fatalf("LostInfo() after second Release = %+v, %v; want %+v, true", again, ok, info)
	}
}

// TestRustPublicLostCallbackFiresOnceOnRelease pins the public callback: one
// delivery for the Release, none for a repeat Release, and a replacement on the
// already-lost device receives the stored info once.
func TestRustPublicLostCallbackFiresOnceOnRelease(t *testing.T) {
	d := newRustTestDevice()

	var first int
	d.SetLostCallback(func(info DeviceLostInfo) {
		first++
		if info.Reason != DeviceLostReasonDestroyed {
			t.Errorf("delivered reason %d; want Destroyed", int(info.Reason))
		}
	})

	d.Release()
	d.Release()
	if first != 1 {
		t.Fatalf("lost callback deliveries = %d; want 1", first)
	}

	var second int
	d.SetLostCallback(func(DeviceLostInfo) { second++ })
	if second != 1 {
		t.Fatalf("replacement deliveries = %d; want 1", second)
	}
	if first != 1 {
		t.Fatalf("replaced registration deliveries = %d; want 1 (no replay)", first)
	}
}

// TestRustPublicUncapturedNotFakedByRelease pins that Release reports only the
// loss: with no uncaptured source at this pin, the uncaptured callback must never
// fire (a fake event would fail this test).
func TestRustPublicUncapturedNotFakedByRelease(t *testing.T) {
	d := newRustTestDevice()
	d.SetUncapturedErrorCallback(func(e *GPUError) {
		t.Fatalf("uncaptured callback fired with %+v; no source exists at this pin", e)
	})
	d.Release()
}

// TestRustPublicNilDeviceSafe pins the nil-receiver guards of the three public
// methods and Release.
func TestRustPublicNilDeviceSafe(t *testing.T) {
	var d *Device
	d.SetLostCallback(func(DeviceLostInfo) { t.Fatal("nil device delivered a loss") })
	d.SetUncapturedErrorCallback(func(*GPUError) { t.Fatal("nil device delivered an error") })
	if _, ok := d.LostInfo(); ok {
		t.Fatal("nil device reported a loss")
	}
	d.Release()
}

// TestRustPublicErrDeviceLostBinding pins C8: the loss sentinel is the
// root-bound ErrDeviceLost, distinct from core.ErrDeviceLost and
// hal.ErrDeviceLost, and ErrContextReleased wraps this value.
func TestRustPublicErrDeviceLostBinding(t *testing.T) {
	if ErrDeviceLost == nil {
		t.Fatal("ErrDeviceLost is nil")
	}
	if errors.Is(ErrDeviceLost, core.ErrDeviceLost) {
		t.Fatal("ErrDeviceLost is core.ErrDeviceLost; the public binding must be its own value (C8)")
	}
	if errors.Is(ErrDeviceLost, hal.ErrDeviceLost) {
		t.Fatal("ErrDeviceLost is hal.ErrDeviceLost; the public binding must be its own value (C8)")
	}
	if !errors.Is(ErrContextReleased, ErrDeviceLost) {
		t.Fatal("ErrContextReleased does not wrap the root ErrDeviceLost")
	}
}
