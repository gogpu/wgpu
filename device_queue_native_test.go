//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import "testing"

// TestDeviceQueueIdentity pins the public Device.Queue contract the S11
// declaration layer must keep (ADR-070 S11, map §2.3): Queue returns the
// device's stored queue, so two calls give the same *Queue and the call
// allocates nothing. Pre-move it returned the stored field too
// (device_native.go:68-71 at 1d1778e); consumers call it per frame, so a
// wrapper built per call would be a regression no API golden can see.
func TestDeviceQueueIdentity(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	queue := device.Queue()
	if queue == nil {
		t.Fatal("Queue() = nil, want the device's queue")
	}
	if got := device.Queue(); got != queue {
		t.Fatalf("Queue() = %p on the second call, want the first %p", got, queue)
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = device.Queue() }); allocs != 0 {
		t.Errorf("Queue() allocates %v per call, want 0", allocs)
	}
}
