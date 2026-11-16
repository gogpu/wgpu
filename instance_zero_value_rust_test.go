//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The public zero-receiver contract of Instance and Adapter on the rust build
// (ADR-070 S12, commit 3), the PUBLIC row class for this context.
//
// This file is COMPILE-ONLY. The rust build's runtime needs wgpu-native and a
// native library this gate does not load, so the rust gate compiles the root
// test binary (`go test -tags rust -c`) and does not execute it; the other two
// contexts are executed (native on the host, browser under node). Every row
// below is written so that a reader can check it against 17a8c4d, and the rows
// whose pre-move answer was a panic are recorded here rather than invoked.
//
// Categories, kept apart:
//
//   - RESTORED PRE-MOVE DEFAULT: DownlevelCapabilities on a nil receiver. The
//     pre-move method was a constant that ignored the receiver
//     (adapter_rust.go:50 at 17a8c4d); the declaration layer now delegates, so
//     its nil guard answers the same constant.
//   - SAFE-DIRECTION DELTA, carded as P2-CORE-034: GetSurfaceCapabilities on a
//     nil receiver. Pre-move it read a.released (adapter_rust.go:96) and
//     panicked; the declaration layer answers nil instead. NOT restored parity —
//     the Move report's deviation text grouped this with the defaults, and the
//     pre-move evidence shows a panic.
//   - PARITY: the metadata and HAL answers below, the zero struct's
//     RequestDevice/Release, and the zero instance's no-op Release.

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
	if _, err := zero.RequestDevice(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero RequestDevice = %v, want wgpu.ErrReleased", err)
	}
	zero.Release()
	zero.Release() // idempotent

	// Recorded, not invoked: a nil adapter's Info, Features, Limits,
	// RequestDevice and Release read the receiver pre-move (adapter_rust.go:39-45,
	// :51, :118) and read it here; both panics are the same class.
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
	// receiver pre-move (instance_rust.go:70, :121) and do here; the zero
	// struct's RequestAdapter reached a nil wgpu-native instance pre-move and
	// reaches the nil implementation here. Both are the same panic class.
}
