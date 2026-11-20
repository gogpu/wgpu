//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package queue

// The queue domain's device-lost evidence (P1-API-042 / #417, D5/D6, C4): a
// HAL submit error that wraps hal.ErrDeviceLost latches the loss first-wins and
// delivers it exactly once, and no callback ever runs under q.mu.
//
// The pre-#417 code compiled in neither test: there was no latch and no
// callback, and Submit held q.mu for its whole body, so any in-lock dispatch
// would deadlock the second case.

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/loststate"
)

// lostSubmitQueue is a HAL queue whose Submit fails with a wrapped
// hal.ErrDeviceLost, the shape DX12's deviceRemovedError produces.
type lostSubmitQueue struct {
	testHALQueue
	err error
}

func (q *lostSubmitQueue) Submit([]hal.CommandBuffer) (uint64, error) {
	q.submitCalls++
	return 0, q.err
}

// TestSubmitDriverLossLatchesUnknownOnce pins D5's Submit classification: a
// HAL submit error wrapping hal.ErrDeviceLost is the driver loss — latched as
// Unknown with the backend's text, the error still handed to the caller with
// its sentinel intact, and the callback fired exactly once, outside q.mu.
func TestSubmitDriverLossLatchesUnknownOnce(t *testing.T) {
	host, coreDev := newTestHost(t, false)
	halQ := &lostSubmitQueue{err: fmt.Errorf("dx12: device removed: %w", hal.ErrDeviceLost)}
	q := New(halQ, nil)
	q.SetDevice(host)

	var fires int
	var got loststate.Info
	// The callback runs on the Submit goroutine after the queue unlocked, so
	// the lock must be free here: dispatching inside submitLocked fails this
	// assertion instead of hanging the test.
	coreDev.SetLostCallback(func(info loststate.Info) {
		fires++
		got = info
		if !q.mu.TryLock() {
			t.Error("the lost callback ran while q.mu was held (C4)")
			return
		}
		q.mu.Unlock()
		if _, ok := coreDev.LostInfo(); !ok {
			t.Error("LostInfo inside the lost callback reports a live device")
		}
	})

	_, err := q.Submit(nil)
	if !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("Submit error = %v, want the HAL sentinel preserved through the wrap", err)
	}
	if fires != 1 {
		t.Fatalf("lost callback fired %d times, want 1", fires)
	}
	if got.Reason != loststate.ReasonUnknown {
		t.Fatalf("lost reason = %v, want Unknown for a driver loss", got.Reason)
	}
	if got.Message == "" {
		t.Fatal("lost message is empty: the backend's text is carried into the info")
	}
	info, ok := coreDev.LostInfo()
	if !ok || info.Reason != loststate.ReasonUnknown {
		t.Fatalf("LostInfo after the failed submit = (%+v, %v), want the latched Unknown loss", info, ok)
	}

	// A second submit that fails the same way must not re-fire: first-wins.
	if _, err := q.Submit(nil); !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("second Submit error = %v, want the HAL sentinel", err)
	}
	if fires != 1 {
		t.Fatalf("lost callback fired %d times after a second failure, want 1", fires)
	}
}

// blockingSubmitQueue is a HAL queue whose Submit parks inside the HAL call
// while the queue lock is held.
type blockingSubmitQueue struct {
	testHALQueue
	entered chan struct{}
	release chan struct{}
}

func (q *blockingSubmitQueue) Submit([]hal.CommandBuffer) (uint64, error) {
	close(q.entered)
	<-q.release
	return 1, nil
}

// TestCallbackRunsWhileAnotherSubmitHoldsTheQueueLock is the C4 probe from the
// other side: while one goroutine is inside Submit (holding q.mu, parked in the
// HAL), a callback dispatched from another goroutine must complete — a
// dispatch path that serialized behind the queue lock would block here until
// the timeout.
func TestCallbackRunsWhileAnotherSubmitHoldsTheQueueLock(t *testing.T) {
	host, coreDev := newTestHost(t, false)
	halQ := &blockingSubmitQueue{entered: make(chan struct{}), release: make(chan struct{})}
	q := New(halQ, nil)
	q.SetDevice(host)

	callbackDone := make(chan loststate.Info, 1)
	coreDev.SetLostCallback(func(info loststate.Info) {
		if _, ok := coreDev.LostInfo(); !ok {
			t.Error("LostInfo inside the lost callback reports a live device")
		}
		callbackDone <- info
	})

	submitDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(nil)
		submitDone <- err
	}()
	<-halQ.entered // the submit goroutine is now inside the HAL call, holding q.mu

	coreDev.MarkLost(loststate.ReasonUnknown, "driver gone")
	coreDev.DispatchPending()

	select {
	case info := <-callbackDone:
		if info.Reason != loststate.ReasonUnknown {
			t.Errorf("lost reason = %v, want Unknown", info.Reason)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the lost callback did not run while another goroutine held q.mu: a dispatch site takes the queue lock")
	}

	close(halQ.release)
	if err := <-submitDone; err != nil {
		t.Fatalf("Submit error = %v, want nil", err)
	}
}
