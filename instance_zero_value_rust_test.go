//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The public zero-receiver contract of Instance and Adapter on the rust build
// (ADR-070 S12, commit 3), the PUBLIC row class for this context.
//
// This file is executed wherever the rust build can load wgpu-native, which the
// development machine does: `GOWORK=off go test -tags rust .` runs it, and that
// is how the degraded-into-panic row below was measured. A machine or gate
// without the native library compiles the root test binary only
// (`go test -tags rust -c`); the other two contexts are executed in their gates
// (native on the host, browser under node). Every row is written so that a
// reader can check it against 17a8c4d, and a pre-move panic is recorded rather
// than invoked.
//
// Categories, kept apart (the three-category rule of the ADR-070 B window):
//
//   - (a) RESTORED PRE-MOVE ANSWER: DownlevelCapabilities on a nil receiver.
//     The pre-move method was a constant that ignored the receiver
//     (adapter_rust.go:50 at 17a8c4d); the declaration layer now delegates, so
//     its nil guard answers the same constant.
//   - (b) DEGRADED-INTO-PANIC, FIXED: a zero-value Instance.RequestAdapter.
//     Pre-move it answered the rust binding's defined nil-or-released error; the
//     move turned that answer into a panic on the nil implementation receiver,
//     and the guard added with this fix answers wgpu.ErrReleased instead.
//     Answer→panic is always a defect; a defined answer is restored, and it is
//     the released answer so a zero and a released handle answer one value.
//   - (c) DECLARED DEFINED→DEFINED DEVIATION, carded P2-CORE-034: the zero
//     struct's RequestDevice. The answer is defined on both sides, so there is no
//     defect; restoring the exact pre-move text would mean manufacturing it (the
//     zero handle no longer reaches the binding), so the deviation is declared,
//     carded, and stated in caller terms. The browser build declares its own
//     safe-direction rows in instance_zero_value_browser_test.go.
//   - SAFE-DIRECTION DELTA, carded as P2-CORE-034: GetSurfaceCapabilities on a
//     nil receiver. Pre-move it read a.released (adapter_rust.go:96) and
//     panicked; the declaration layer answers nil instead. NOT restored parity —
//     the Move report's deviation text grouped this with the defaults, and the
//     pre-move evidence shows a panic.
//   - PARITY: the metadata and HAL answers below, the zero struct's Release,
//     and the zero instance's no-op Release.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	publichal "github.com/gogpu/wgpu/hal"
)

// TestPublicZeroValueRustAdapter pins the adapter's zero-value answers.
func TestPublicZeroValueRustAdapter(t *testing.T) {
	var nilAdapter *wgpu.Adapter
	if got := nilAdapter.DownlevelCapabilities(); got != gputypes.DefaultDownlevelCapabilities() {
		t.Fatalf("nil DownlevelCapabilities = %+v, want the default capabilities", got)
	}
	if got := nilAdapter.GetSurfaceCapabilities(nil); got != nil {
		t.Fatalf("nil GetSurfaceCapabilities(nil) = %+v, want nil (P2-CORE-034)", got)
	}
	if got := nilAdapter.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("nil adapter HAL = %+v, want the zero Handles", got)
	}

	zero := &wgpu.Adapter{}
	if got := zero.DownlevelCapabilities(); got != gputypes.DefaultDownlevelCapabilities() {
		t.Fatalf("zero DownlevelCapabilities = %+v, want the default capabilities", got)
	}
	if got := zero.GetSurfaceCapabilities(nil); got != nil {
		t.Fatalf("zero GetSurfaceCapabilities(nil) = %+v, want nil", got)
	}
	if got := zero.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("zero adapter HAL = %+v, want the zero Handles", got)
	}
	// Declared deviation, defined→defined, carded P2-CORE-034. Pre-move the zero
	// struct called the nil rust binding handle, whose nil guard answers
	// "adapter is nil or released" (go-webgpu/webgpu@v0.5.5 wgpu/device.go:75-76),
	// wrapped as "wgpu: failed to request device: …" (adapter_rust.go:67-69 at
	// 17a8c4d). The moved implementation answers core.ErrReleased before the
	// binding is reached. For callers, errors.Is(err, wgpu.ErrReleased) is true
	// where it was false before; the binding's text is not reproduced on purpose.
	if _, err := zero.RequestDevice(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero RequestDevice = %v, want wgpu.ErrReleased", err)
	}
	zero.Release()
	zero.Release() // idempotent

	// Recorded, not invoked: a nil adapter's Info, Features, Limits,
	// RequestDevice and Release read the receiver pre-move (adapter_rust.go:34-40,
	// :51, :117-119 at 17a8c4d) and read it here; both panics are the same class.
}

// TestPublicZeroValueRustInstance pins the instance's zero-value answers.
func TestPublicZeroValueRustInstance(t *testing.T) {
	var nilInstance *wgpu.Instance
	if got := nilInstance.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL on a nil instance = %+v, want the zero Handles", got)
	}

	zero := &wgpu.Instance{}
	if got := zero.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL on the zero instance = %+v, want the zero Handles", got)
	}
	zero.Release() // no-op: the pre-move zero struct had no instance handle either
	zero.Release() // idempotent

	// Recorded, not invoked: a nil instance's RequestAdapter and Release read the
	// receiver pre-move (instance_rust.go:67, :121 at 17a8c4d) and read the
	// declaration layer's receiver here, so both still panic.
	//
	// Correction. This file used to say the zero struct's RequestAdapter "reached
	// a nil wgpu-native instance pre-move and reaches the nil implementation
	// here. Both are the same panic class." That was wrong, and it hid the
	// regression the S12 review proved. Measured against 17a8c4d: a zero-value
	// Instance.RequestAdapter answered the binding's nil guard — "wgpu: RequestAdapter:
	// instance is nil or released" (go-webgpu/webgpu@v0.5.5 wgpu/adapter.go:117-118),
	// wrapped as "wgpu: failed to request adapter: …" (instance_rust.go:82-85) — a
	// defined error; after the move and before this fix it PANICKED on the nil
	// implementation receiver (internal/instance/instance_rust.go:63); the guard
	// added with this fix answers core.ErrReleased. So the zero struct was never
	// parity with the nil receiver: it is a degraded-into-panic row (category (b))
	// that is now fixed and pinned below.
}

// TestPublicZeroValueRustInstanceRequestAdapter pins category (b): a zero-value
// Instance.RequestAdapter answers a defined error and does not panic, before and
// after its own Release. It is the row that failed on the tree the S12 review
// blocked: there the call panicked on the nil implementation receiver.
func TestPublicZeroValueRustInstanceRequestAdapter(t *testing.T) {
	zero := &wgpu.Instance{}
	if _, err := zero.RequestAdapter(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero RequestAdapter = %v, want wgpu.ErrReleased", err)
	}
	zero.Release()
	if _, err := zero.RequestAdapter(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("RequestAdapter after zero Release = %v, want wgpu.ErrReleased", err)
	}
}
