//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The public zero-receiver contract of Instance and Adapter on the native build
// (ADR-070 S12, commit 3), the PUBLIC row class. The implementation's own zero
// value is the other class, pinned by internal/instance/zero_value_native_test.go;
// the S11 review lesson is that the receiver callers hold must be pinned
// separately, because the declaration layer forwards to an implementation the
// public zero value does not have.
//
// Two categories, kept apart on purpose:
//
//   - PARITY rows reproduce the pre-move public object at 17a8c4d exactly: the
//     nil Instance answers ErrReleased from every create path and no-ops on
//     Release (instance_native.go:90, :163-170, :230-237 there), the nil Adapter
//     answers ErrReleased from RequestDevice (:50) and no-ops on Release (:185),
//     and the zero Adapter answers the same from RequestDevice (:50, "instance
//     == nil").
//   - DECLARED DEVIATIONS, carded as P2-CORE-034, are the SAFE direction of the
//     zero *Instance* struct: RequestAdapter and CreateSurface* answer
//     ErrReleased and Release is a no-op, where the pre-move zero struct passed
//     isReleased (its flag was false) and then dereferenced a nil core — a panic
//     at :99, :114, :147, :205 and :261 there. The implementation's zero value
//     answers released (internal/instance/instance_native.go:106-111), so the
//     public wrapper turns that panic into the sentinel. This is a behavior
//     change, not parity, and no other row here inherits it.
//
// Recorded, not invoked: the nil Adapter's Info, Features, Limits and
// DownlevelCapabilities read the receiver pre-move (adapter_native.go:35-47) and
// read it here too; both panic, so no row invokes them.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	publichal "github.com/gogpu/wgpu/hal"
)

// zeroValueInstances returns the two receivers callers can hold, built fresh per
// case so a Release in one case cannot leak into another.
func zeroValueInstances() []struct {
	name string
	make func() *wgpu.Instance
} {
	return []struct {
		name string
		make func() *wgpu.Instance
	}{
		{"nil receiver", func() *wgpu.Instance { return nil }},
		{"zero struct", func() *wgpu.Instance { return &wgpu.Instance{} }},
	}
}

// TestPublicZeroValueInstanceAnswers pins the create-path and teardown answers
// of a nil and a zero-value public Instance.
func TestPublicZeroValueInstanceAnswers(t *testing.T) {
	for _, recv := range zeroValueInstances() {
		t.Run(recv.name, func(t *testing.T) {
			inst := recv.make()

			if _, err := inst.RequestAdapter(nil); !errors.Is(err, wgpu.ErrReleased) {
				t.Fatalf("RequestAdapter = %v, want wgpu.ErrReleased", err)
			}
			if _, err := inst.CreateSurface(0, 0); !errors.Is(err, wgpu.ErrReleased) {
				t.Fatalf("CreateSurface = %v, want wgpu.ErrReleased", err)
			}
			if _, err := inst.CreateSurfaceUnsafe(wgpu.SurfaceTargetUnsafe{}); !errors.Is(err, wgpu.ErrReleased) {
				t.Fatalf("CreateSurfaceUnsafe = %v, want wgpu.ErrReleased", err)
			}
			if _, err := inst.CreateSurfaceFromTarget(nil); !errors.Is(err, wgpu.ErrReleased) {
				t.Fatalf("CreateSurfaceFromTarget = %v, want wgpu.ErrReleased", err)
			}
			if got := inst.HAL(); got != (publichal.Handles{}) {
				t.Fatalf("HAL = %+v, want the zero Handles", got)
			}

			inst.Release()
			inst.Release() // idempotent
		})
	}
}

// TestPublicZeroValueAdapterAnswers pins the adapter answers that are read-only
// on a zero-value receiver, plus the two entry points the pre-move object
// guarded: RequestDevice answers ErrReleased and Release is a no-op.
func TestPublicZeroValueAdapterAnswers(t *testing.T) {
	var zero wgpu.Adapter

	if got := zero.Info(); got != (gputypes.AdapterInfo{}) {
		t.Fatalf("Info = %+v, want the zero value", got)
	}
	if got := zero.Features(); got != 0 {
		t.Fatalf("Features = %#x, want 0", got)
	}
	if got := zero.Limits(); got != (gputypes.Limits{}) {
		t.Fatalf("Limits = %+v, want the zero value", got)
	}
	if got := zero.DownlevelCapabilities(); got != (gputypes.DownlevelCapabilities{}) {
		t.Fatalf("DownlevelCapabilities = %+v, want the zero value", got)
	}
	if got := zero.GetSurfaceCapabilities(nil); got != nil {
		t.Fatalf("GetSurfaceCapabilities(nil) = %+v, want nil", got)
	}
	if got := zero.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL = %+v, want the zero Handles", got)
	}
	if _, err := zero.RequestDevice(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("RequestDevice on the zero adapter = %v, want wgpu.ErrReleased", err)
	}

	zero.Release()
	zero.Release() // idempotent

	var nilAdapter *wgpu.Adapter
	if _, err := nilAdapter.RequestDevice(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("RequestDevice on a nil adapter = %v, want wgpu.ErrReleased", err)
	}
	if got := nilAdapter.GetSurfaceCapabilities(nil); got != nil {
		t.Fatalf("GetSurfaceCapabilities on a nil adapter = %+v, want nil", got)
	}
	if got := nilAdapter.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL on a nil adapter = %+v, want the zero Handles", got)
	}
	nilAdapter.Release()
}
