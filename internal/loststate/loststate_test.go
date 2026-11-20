// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package loststate

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestFirstLossWins pins C9: the first MarkLost fixes the stored reason, and a
// later one never overwrites it and never queues a second delivery.
func TestFirstLossWins(t *testing.T) {
	s := New[string]()
	s.SetLostCallback(func(Info) {})
	s.MarkLost(ReasonUnknown, "driver")
	s.MarkLost(ReasonDestroyed, "release")

	info, ok := s.LostInfo()
	if !ok || info.Reason != ReasonUnknown || info.Message != "driver" {
		t.Fatalf("LostInfo() = %+v, %v; want Unknown/driver, true", info, ok)
	}
	lost, _ := s.TakePending()
	if len(lost) != 1 {
		t.Fatalf("TakePending() lost = %d deliveries; want 1", len(lost))
	}
	lost, _ = s.TakePending()
	if len(lost) != 0 {
		t.Fatalf("second TakePending() lost = %d; want 0", len(lost))
	}
}

// TestLiveDeviceLostInfo pins the not-lost answer.
func TestLiveDeviceLostInfo(t *testing.T) {
	s := New[string]()
	if _, ok := s.LostInfo(); ok {
		t.Fatal("LostInfo() ok = true on a live device")
	}
}

// TestLateSubscriberFiresOnce pins the late-subscriber rule: a registration on
// an already-lost device receives the stored info exactly once, at the setter's
// return, and a replacement is a fresh registration that receives it once too.
func TestLateSubscriberFiresOnce(t *testing.T) {
	s := New[string]()
	s.MarkLost(ReasonUnknown, "lost early")

	var first int
	fire, ok := s.SetLostCallback(func(i Info) {
		first++
		if i.Reason != ReasonUnknown || i.Message != "lost early" {
			t.Errorf("delivered %+v; want the stored info", i)
		}
	})
	if !ok {
		t.Fatal("SetLostCallback on a lost device returned no delivery")
	}
	fire.Fn(fire.Info)
	if first != 1 {
		t.Fatalf("first registration deliveries = %d; want 1", first)
	}

	// A replacement on the lost device is a new registration: it receives the
	// stored info once, and the replaced one is not delivered again.
	var second int
	fire, ok = s.SetLostCallback(func(Info) { second++ })
	if !ok {
		t.Fatal("replacement registration returned no delivery")
	}
	fire.Fn(fire.Info)
	if second != 1 {
		t.Fatalf("replacement deliveries = %d; want 1", second)
	}
	if first != 1 {
		t.Fatalf("replaced registration deliveries = %d; want 1 (no replay)", first)
	}
}

// TestClearWithoutDelivery pins: a nil setter clears the registration and the
// stored info is never delivered to nothing.
func TestClearWithoutDelivery(t *testing.T) {
	s := New[string]()
	s.MarkLost(ReasonDestroyed, "released")

	if _, ok := s.SetLostCallback(nil); ok {
		t.Fatal("SetLostCallback(nil) delivered")
	}
	if _, ok := s.SetLostCallback(func(Info) {}); !ok {
		t.Fatal("registration after a cleared one missed the stored info")
	}
}

// TestNoRegistrationNoQueue pins that a loss recorded with no registration
// queues nothing, while LostInfo still answers for future subscribers.
func TestNoRegistrationNoQueue(t *testing.T) {
	s := New[string]()
	s.MarkLost(ReasonUnknown, "lost")
	if lost, _ := s.TakePending(); len(lost) != 0 {
		t.Fatalf("queued %d deliveries with no registration; want 0", len(lost))
	}
	if info, ok := s.LostInfo(); !ok || info.Message != "lost" {
		t.Fatalf("LostInfo() = %+v, %v; want the stored loss", info, ok)
	}
}

// TestUncapturedCapturedAtRecordTime pins B: an event is delivered to the
// registration current when it was RECORDED, so replacing the callback between
// record and dispatch never hands a past event to the new registration.
func TestUncapturedCapturedAtRecordTime(t *testing.T) {
	s := New[string]()
	var first, second []string
	s.SetUncapturedErrorCallback(func(e string) { first = append(first, e) })
	s.RecordUncaptured("boom")
	s.SetUncapturedErrorCallback(func(e string) { second = append(second, e) })

	_, errs := s.TakePending()
	for _, f := range errs {
		f.Fn(f.Err)
	}
	if len(first) != 1 || first[0] != "boom" {
		t.Fatalf("first registration got %v; want [boom]", first)
	}
	if len(second) != 0 {
		t.Fatalf("replacement got %v; want nothing (no replay)", second)
	}
}

// TestUncapturedDroppedWithoutRegistration pins the no-registration drop.
func TestUncapturedDroppedWithoutRegistration(t *testing.T) {
	s := New[string]()
	s.RecordUncaptured("dropped")
	if _, errs := s.TakePending(); len(errs) != 0 {
		t.Fatalf("queued %d errors with no registration; want 0", len(errs))
	}
}

// TestConcurrentLatchAndRegistration runs the registration/latch/dispatch
// paths against each other for the race detector and pins the per-registration
// bound: one registration is delivered at most once even when MarkLost is
// hammered from many goroutines.
func TestConcurrentLatchAndRegistration(t *testing.T) {
	s := New[string]()

	var delivered atomic.Int32
	s.SetLostCallback(func(Info) { delivered.Add(1) })

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.MarkLost(ReasonUnknown, "concurrent")
			s.MarkLost(ReasonDestroyed, "concurrent second")
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			if lost, _ := s.TakePending(); len(lost) > 0 {
				for _, f := range lost {
					f.Fn(f.Info)
				}
			}
			s.LostInfo()
		}
	}()
	wg.Wait()

	// Drain whatever raced past the consumer loop.
	lost, _ := s.TakePending()
	for _, f := range lost {
		f.Fn(f.Info)
	}

	if got := delivered.Load(); got != 1 {
		t.Fatalf("deliveries = %d; want exactly 1 for the one registration", got)
	}
	if info, ok := s.LostInfo(); !ok || info.Reason != ReasonUnknown {
		t.Fatalf("LostInfo() = %+v, %v; want the first reason to have won", info, ok)
	}
}
