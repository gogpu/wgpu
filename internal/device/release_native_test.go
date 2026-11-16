//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import "testing"

// TestReleaseWaitsBeforeHALDestroy ports the one assertion of the root
// device_release_test.go (device_release_test.go:55) that needs no noop
// backend: Release drains the device with WaitIdle before the HAL device is
// destroyed, in that order (ADR-070 S11, map §10.3). The root keeps its
// noop-driven copy — the S8 lesson forbids noop in the destination — and this
// copy runs on the local HAL double (testsupport_native_test.go), so the
// ordering stays pinned on the moved implementation itself.
func TestReleaseWaitsBeforeHALDestroy(t *testing.T) {
	events := make([]string, 0, 2)
	d := newTestDevice(&events)

	d.Release()

	if len(events) != 2 || events[0] != "wait-idle" || events[1] != "destroy" {
		t.Fatalf("HAL lifecycle events = %v, want [wait-idle destroy]", events)
	}
}
