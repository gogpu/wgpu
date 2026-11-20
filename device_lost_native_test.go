//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// The public face of the device-lost and uncaptured-error callbacks on the
// native build (P1-API-042 / #417): a driver loss observed on the queue's
// submit path reaches the public callback with the public reason, and an
// uncaptured encode-time error reaches it with the public *GPUError class
// identity the error scopes use.
//
// The pre-#417 code compiled in neither test: the three methods did not exist
// and no error escaped the scope manager.

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

// lostSubmitHALQueue is the noop queue with a failing Submit: it wraps the
// real backend queue, so the device around it is a real one and only the submit
// result is the driver loss.
type lostSubmitHALQueue struct {
	hal.Queue
	err error
}

func (q *lostSubmitHALQueue) Submit([]hal.CommandBuffer) (uint64, error) { return 0, q.err }

// lostSubmitDevice builds the noop test device whose queue submits through a
// wrapper failing with err.
func lostSubmitDevice(t *testing.T, err error) *Device {
	t.Helper()
	return newTestDeviceWithHALQueue(t, func(q hal.Queue) hal.Queue {
		return &lostSubmitHALQueue{Queue: q, err: err}
	})
}

// TestSubmitDeviceLossReportsLostThroughThePublicAPI pins D5 end to end: the
// HAL sentinel a backend wraps on submit is classified as a device loss, the
// caller still sees the sentinel through the public error, and the public
// callback fires once with DeviceLostReasonUnknown and the backend's text.
func TestSubmitDeviceLossReportsLostThroughThePublicAPI(t *testing.T) {
	dev := lostSubmitDevice(t, fmt.Errorf("dx12: device removed: %w", hal.ErrDeviceLost))

	var mu sync.Mutex
	var fired []DeviceLostInfo
	dev.SetLostCallback(func(info DeviceLostInfo) {
		mu.Lock()
		defer mu.Unlock()
		fired = append(fired, info)
	})

	if _, err := dev.Queue().Submit(); !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("Submit error = %v, want the HAL device-lost sentinel preserved", err)
	}

	mu.Lock()
	n := len(fired)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("lost callback fired %d times, want 1", n)
	}
	if fired[0].Reason != DeviceLostReasonUnknown {
		t.Fatalf("lost reason = %v, want DeviceLostReasonUnknown for a driver loss", fired[0].Reason)
	}
	if fired[0].Message == "" {
		t.Fatal("lost message is empty: the backend's text is carried into the public info")
	}

	info, ok := dev.LostInfo()
	if !ok || info.Reason != DeviceLostReasonUnknown {
		t.Fatalf("LostInfo after the failed submit = (%+v, %v), want the latched Unknown loss", info, ok)
	}
}

// TestUncapturedErrorReachesThePublicCallback pins D6 and the class identity:
// an encode-time validation error reaches SetUncapturedErrorCallback's function
// with errors.Is(err, ErrValidation) true, and an error a pushed scope captured
// is never delivered to it.
func TestUncapturedErrorReachesThePublicCallback(t *testing.T) {
	_, _, dev := newTestDevice(t)

	var mu sync.Mutex
	var got []*GPUError
	dev.SetUncapturedErrorCallback(func(e *GPUError) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})

	enc, err := dev.CreateCommandEncoder(&CommandEncoderDescriptor{Label: "lost-test"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	// A nil source buffer is a validation error recorded on the encoder, which
	// reports it to the device's error scope sink as it happens.
	enc.CopyBufferToBuffer(nil, 0, nil, 0, 4)

	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("uncaptured callback fired %d times, want 1", n)
	}
	if !errors.Is(got[0], ErrValidation) {
		t.Fatalf("errors.Is(delivered, ErrValidation) = false for %v: the public class identity must match the scopes'", got[0])
	}

	// The same error under a pushed scope is captured and never delivered.
	if err := dev.PushErrorScope(ErrorFilterValidation); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}
	enc.CopyBufferToBuffer(nil, 0, nil, 0, 4)
	mu.Lock()
	n = len(got)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("uncaptured callback fired %d times after a captured error, want still 1", n)
	}
	popped, err := dev.PopErrorScope()
	if err != nil || popped == nil {
		t.Fatalf("PopErrorScope = (%v, %v), want the captured error", popped, err)
	}
	if !errors.Is(popped, ErrValidation) {
		t.Fatalf("errors.Is(popped, ErrValidation) = false for %v", popped)
	}
}

// TestLostDevicePopErrorScopeIsNilNil pins the four-case contract through the
// public declaration layer: a lost device answers (nil, nil) whatever its scope
// stack holds, and the rust-matching nil-device answer is unchanged.
func TestLostDevicePopErrorScopeIsNilNil(t *testing.T) {
	dev := lostSubmitDevice(t, fmt.Errorf("dx12: device removed: %w", hal.ErrDeviceLost))

	if err := dev.PushErrorScope(ErrorFilterValidation); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}
	if _, err := dev.Queue().Submit(); !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("Submit error = %v, want the HAL sentinel", err)
	}
	if _, ok := dev.LostInfo(); !ok {
		t.Fatal("LostInfo does not report the loss the submit observed")
	}

	e, err := dev.PopErrorScope()
	if e != nil || err != nil {
		t.Fatalf("PopErrorScope on a lost device = (%v, %v), want (nil, nil), never ErrErrorScopeEmpty", e, err)
	}

	var nilDevice *Device
	e, err = nilDevice.PopErrorScope()
	if e != nil || err != nil {
		t.Fatalf("PopErrorScope on a nil device = (%v, %v), want (nil, nil)", e, err)
	}
}

// TestLostCallbackNilClearsRegistration pins the setter's nil form: clearing the
// registration means the stored event is never delivered to a later dispatch.
func TestLostCallbackNilClearsRegistration(t *testing.T) {
	dev := lostSubmitDevice(t, hal.ErrDeviceLost)

	fires := 0
	dev.SetLostCallback(func(DeviceLostInfo) { fires++ })
	dev.SetLostCallback(nil)
	if _, err := dev.Queue().Submit(); !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("Submit error = %v, want the HAL sentinel", err)
	}
	if fires != 0 {
		t.Fatalf("a cleared registration was delivered to %d times, want 0", fires)
	}
	if _, ok := dev.LostInfo(); !ok {
		t.Fatal("LostInfo does not report the loss: clearing the callback clears the observation only")
	}
}
