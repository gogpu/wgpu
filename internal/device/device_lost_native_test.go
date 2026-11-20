//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// The device domain's device-lost and uncaptured-error seams
// (P1-API-042 / #417): the destroyed reason Release records (D4, C9/C10), the
// per-registration late-subscriber rule as the device layer applies it (C5),
// and the lock contract — a callback that re-enters read-only accessors while
// other goroutines register, latch and pop must neither deadlock nor race (C4).
//
// Each case names what it fails on the pre-#417 code: none of these methods
// existed, Release reported nothing, and there was no registration to deliver
// to.

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/loststate"
)

// TestReleaseReportsDestroyedExactlyOnce pins D4: a healthy device's Release
// records DeviceLostReasonDestroyed and invokes the callback exactly once, with
// the HAL teardown already done, and a second Release neither fires again nor
// changes the stored reason.
func TestReleaseReportsDestroyedExactlyOnce(t *testing.T) {
	events := make([]string, 0, 2)
	d := newTestDevice(&events)

	var fires atomic.Int64
	var stored atomic.Pointer[loststate.Info]
	d.SetLostCallback(func(info loststate.Info) {
		snapshot := info
		fires.Add(1)
		stored.Store(&snapshot)
	})

	d.Release()

	if got := fires.Load(); got != 1 {
		t.Fatalf("lost callback fired %d times on Release, want exactly 1", got)
	}
	info := stored.Load()
	if info == nil || info.Reason != loststate.ReasonDestroyed {
		t.Fatalf("stored info = %+v, want ReasonDestroyed", info)
	}
	if len(events) != 2 || events[0] != "wait-idle" || events[1] != "destroy" {
		t.Fatalf("HAL lifecycle events = %v, want [wait-idle destroy] before the callback", events)
	}
	got, ok := d.LostInfo()
	if !ok || got.Reason != loststate.ReasonDestroyed {
		t.Fatalf("LostInfo after Release = (%+v, %v), want Destroyed", got, ok)
	}

	d.Release() // idempotent: no second delivery
	if got := fires.Load(); got != 1 {
		t.Fatalf("lost callback fired %d times after a second Release, want 1", got)
	}
}

// TestDriverLossKeepsItsReasonAcrossRelease pins C9/C10: a driver loss recorded
// first is never overwritten by the destroyed reason and never re-fires.
func TestDriverLossKeepsItsReasonAcrossRelease(t *testing.T) {
	d := newTestDevice(nil)

	var fires atomic.Int64
	d.SetLostCallback(func(loststate.Info) { fires.Add(1) })

	d.MarkDeviceLost("dx12: DRED says the GPU hung")
	d.DispatchDeviceCallbacks()
	if got := fires.Load(); got != 1 {
		t.Fatalf("lost callback fired %d times on the driver loss, want 1", got)
	}

	d.Release() // records Destroyed: first-wins keeps the driver's record
	info, ok := d.LostInfo()
	if !ok {
		t.Fatal("LostInfo reports a live device after the loss")
	}
	if info.Reason != loststate.ReasonUnknown || info.Message != "dx12: DRED says the GPU hung" {
		t.Fatalf("LostInfo after Release = %+v, want the first (driver) observation", info)
	}
	if got := fires.Load(); got != 1 {
		t.Fatalf("lost callback fired %d times across the release, want 1: a later reason never re-fires", got)
	}
}

// TestLateSubscriberGetsStoredInfoOnce pins the per-registration rule through
// the device layer, which is where the late delivery is invoked: a registration
// made after the loss fires once with the stored info, and the replaced
// registration never fires again. An implementation with a GLOBAL delivered
// flag fails here — the second registration receives no delivery — and one that
// does not mark the delivery fails the opposite way, firing it twice.
func TestLateSubscriberGetsStoredInfoOnce(t *testing.T) {
	d := newTestDevice(nil)

	first := atomic.Int64{}
	d.SetLostCallback(func(loststate.Info) { first.Add(1) })
	d.MarkDeviceLost("driver gone")
	d.DispatchDeviceCallbacks()
	if got := first.Load(); got != 1 {
		t.Fatalf("first registration fired %d times, want 1", got)
	}

	second := atomic.Int64{}
	var secondInfo loststate.Info
	d.SetLostCallback(func(info loststate.Info) {
		second.Add(1)
		secondInfo = info
	})
	if got := second.Load(); got != 1 {
		t.Fatalf("the late registration fired %d times at registration, want exactly 1 with the stored info", got)
	}
	if secondInfo.Reason != loststate.ReasonUnknown || secondInfo.Message != "driver gone" {
		t.Fatalf("late registration info = %+v, want the stored observation", secondInfo)
	}

	d.DispatchDeviceCallbacks()
	d.MarkDeviceLost("driver gone again") // first-wins: nothing new to deliver
	d.DispatchDeviceCallbacks()
	if got := first.Load(); got != 1 {
		t.Fatalf("replaced registration fired %d times, want 1: it must never receive a past event again", got)
	}
	if got := second.Load(); got != 1 {
		t.Fatalf("late registration fired %d times, want 1: a registration is delivered once", got)
	}
}

// TestLostCallbackReentersReadOnlyAccessors is the lock-contract probe (C4):
// the lost callback calls LostInfo and Features — reads that take the state
// machine's own lock — while other goroutines register, latch, dispatch and
// pop error scopes. Under -race it also proves the registration handoff between
// the delivering goroutine and the setters. Running the callback under a device
// lock, or the setters under the state lock, deadlocks or races here.
func TestLostCallbackReentersReadOnlyAccessors(t *testing.T) {
	d := newTestDevice(nil)

	var reentry atomic.Int64
	d.SetLostCallback(func(loststate.Info) {
		if _, ok := d.LostInfo(); !ok {
			t.Error("LostInfo inside the lost callback reports a live device")
		}
		_ = d.Features()
		_ = d.Limits()
		reentry.Add(1)
	})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				d.SetUncapturedErrorCallback(nil)
				d.SetLostCallback(func(loststate.Info) { _, _ = d.LostInfo() })
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				if err := d.PushErrorScope(core.ErrorFilterValidation); err == nil {
					_, _ = d.PopErrorScope()
				}
				if i%16 == 0 {
					d.DispatchDeviceCallbacks()
				}
			}
		}
	}()

	// The registrations churn, so the delivering goroutine races them; the
	// latch is first-wins, so exactly one dispatch carries the event.
	for range 8 {
		d.MarkDeviceLost("driver gone")
		d.DispatchDeviceCallbacks()
	}
	close(stop)
	wg.Wait()

	if info, ok := d.LostInfo(); !ok || info.Reason != loststate.ReasonUnknown {
		t.Fatalf("LostInfo after the stress = (%+v, %v), want the latched Unknown loss", info, ok)
	}
}

// TestZeroValueLostSeam pins the nil-safe answers of the seam on the two
// zero-value receivers, matching the accessors' contract.
func TestZeroValueLostSeam(t *testing.T) {
	for _, d := range []*Device{nil, new(Device)} {
		if _, ok := d.LostInfo(); ok {
			t.Fatal("a zero-value device reported a loss")
		}
		d.SetLostCallback(func(loststate.Info) { t.Fatal("a zero-value device delivered a lost event") })
		d.SetUncapturedErrorCallback(func(*core.GPUError) { t.Fatal("a zero-value device delivered an error") })
		d.MarkDeviceLost("never recorded")
		d.DispatchDeviceCallbacks()
	}
}

// TestUncapturedErrorClassIdentityThroughTheSeam pins that the device seam
// hands the callback the core class, so errors.Is classifies it: the public
// *GPUError is a conversion of this value, never a re-classification.
func TestUncapturedErrorClassIdentityThroughTheSeam(t *testing.T) {
	d := newTestDevice(nil)
	var mu sync.Mutex
	var got []*core.GPUError
	d.SetUncapturedErrorCallback(func(e *core.GPUError) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})

	d.Core().ReportError(core.ErrorFilterOutOfMemory, "allocation failed")
	d.DispatchPending()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("uncaptured callback fired %d times, want 1", len(got))
	}
	if !errors.Is(got[0], core.SentinelForFilter(core.ErrorFilterOutOfMemory)) {
		t.Fatalf("delivered error %v does not classify as the OutOfMemory class", got[0])
	}
}
