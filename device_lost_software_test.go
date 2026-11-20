//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The device-lost callback on a real backend device (P1-API-042 / #417): the
// software backend is the one every CI runner has, so this is the end-to-end
// proof that (*Device).Release reports DeviceLostReasonDestroyed exactly once,
// and that a subscriber registering afterwards still learns the outcome.
//
// The pre-#417 code compiled in neither test: the three methods did not exist.

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// openSoftwareDevice opens a software-backend device for the lifetime tests.
func openSoftwareDevice(t *testing.T) (*wgpu.Instance, *wgpu.Adapter, *wgpu.Device) {
	t.Helper()
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance(software): %v", err)
	}
	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		inst.Release()
		t.Fatalf("RequestAdapter: %v", err)
	}
	dev, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		inst.Release()
		t.Fatalf("RequestDevice: %v", err)
	}
	return inst, adapter, dev
}

// TestReleaseReportsDestroyedOnASoftwareDevice pins D4 end to end: releasing a
// live device invokes the callback once with DeviceLostReasonDestroyed, a
// second Release does not fire again, and a registration made after the loss is
// delivered the stored info exactly once at registration time.
func TestReleaseReportsDestroyedOnASoftwareDevice(t *testing.T) {
	inst, adapter, dev := openSoftwareDevice(t)
	defer inst.Release()
	defer adapter.Release()

	if _, ok := dev.LostInfo(); ok {
		t.Fatal("a live device reports a loss")
	}

	var fired []wgpu.DeviceLostInfo
	dev.SetLostCallback(func(info wgpu.DeviceLostInfo) { fired = append(fired, info) })

	dev.Release()

	if len(fired) != 1 {
		t.Fatalf("lost callback fired %d times on Release, want exactly 1", len(fired))
	}
	if fired[0].Reason != wgpu.DeviceLostReasonDestroyed {
		t.Fatalf("lost reason = %v, want DeviceLostReasonDestroyed", fired[0].Reason)
	}
	info, ok := dev.LostInfo()
	if !ok || info.Reason != wgpu.DeviceLostReasonDestroyed {
		t.Fatalf("LostInfo after Release = (%+v, %v), want Destroyed", info, ok)
	}

	dev.Release() // idempotent: no second delivery
	if len(fired) != 1 {
		t.Fatalf("lost callback fired %d times after a second Release, want 1", len(fired))
	}

	// A subscriber that registers after the loss is handed the stored info once.
	late := 0
	var lateInfo wgpu.DeviceLostInfo
	dev.SetLostCallback(func(i wgpu.DeviceLostInfo) { late++; lateInfo = i })
	if late != 1 {
		t.Fatalf("late registration fired %d times, want exactly 1 with the stored info", late)
	}
	if lateInfo.Reason != wgpu.DeviceLostReasonDestroyed {
		t.Fatalf("late registration info = %+v, want the stored Destroyed info", lateInfo)
	}
	dev.SetLostCallback(func(wgpu.DeviceLostInfo) { late++ })
	if late != 2 {
		t.Fatalf("a second late registration left the count at %d, want 2", late)
	}
	// Replacing it again does not replay: one delivery per registration.
	for range 3 {
		dev.SetLostCallback(nil)
		dev.SetLostCallback(func(wgpu.DeviceLostInfo) { late++ })
	}
	if late != 5 {
		t.Fatalf("late registration count = %d, want 5: exactly one delivery per registration", late)
	}
}
