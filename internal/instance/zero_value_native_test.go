//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// The instance domain's zero-value and released contract (ADR-070 S12, commit
// 3), the INTERNAL row class: the answers of the *Instance and *Adapter
// implementations themselves. The public wrappers callers hold are the other
// row class, pinned by the root tables (instance_zero_value_native_test.go,
// instance_zero_value_browser_test.go, instance_zero_value_rust_test.go); the
// S11 review lesson (N1/N2) is that the two receivers answer differently and one
// table must never stand in for the other.
//
// Every answer below reproduces the pre-move root object at 17a8c4d, cited per
// row. Where the pre-move object panicked, the row is recorded and deliberately
// not invoked: a panic is a recorded fact, not an assertion this file can make
// without aborting the binary.
//
// Nothing here is a blanket-parity claim. The released answers are
// core.ErrReleased by identity (errors.Is is the callers' contract), and the
// zero struct's safe-direction deltas are declared in the root native table
// under P2-CORE-034.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/core"
)

// TestInternalZeroValueInstance pins the nil receiver's answers: the instance
// that is absent answers released (instance_native.go:163-166 pre-move), every
// question it can answer without a core instance answers zero, and Release is a
// no-op (beginRelease returned false, :230-237 pre-move).
func TestInternalZeroValueInstance(t *testing.T) {
	var inst *Instance

	if got := inst.Core(); got != nil {
		t.Fatalf("Core = %p, want nil", got)
	}
	if !inst.IsReleased() {
		t.Fatal("a nil instance is not reported released")
	}
	if _, err := inst.RequestAdapter(nil, nil); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("RequestAdapter = %v, want core.ErrReleased", err)
	}
	if got := inst.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL = %+v, want the zero Handles", got)
	}
	if got := inst.SurfacesForDevice(nil); got != nil {
		t.Fatalf("SurfacesForDevice = %v, want nil", got)
	}
	inst.UnregisterDevice(nil)
	inst.UnregisterSurface(nil)
	inst.Release() // no-op
	inst.Release() // idempotent
}

// TestInternalZeroStructInstance pins the zero struct's answers. It is NOT
// released (the pre-move isReleased read its own flag, :163-170), and the two
// entry points that reach the nil core are recorded, not invoked: RequestAdapter
// dereferences it at :99 pre-move, Release at :261 pre-move (core.(*Instance)
// .Destroy reads its mutex on a nil receiver).
func TestInternalZeroStructInstance(t *testing.T) {
	inst := &Instance{}

	if got := inst.Core(); got != nil {
		t.Fatalf("Core = %p, want nil", got)
	}
	if inst.IsReleased() {
		t.Fatal("the zero struct reports released, but its flag is false and no Release ran")
	}
	if got := inst.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL = %+v, want the zero Handles", got)
	}
	// Recorded, not invoked: inst.RequestAdapter(nil, nil) panicked pre-move at
	// :99 and reaches the same nil core here; inst.Release() panicked pre-move at
	// :261 and reaches the same nil core here.
}

// TestInternalReleasedInstance pins the released answers: IsReleased follows the
// flag, every request answers core.ErrReleased by identity, HAL reports nothing,
// and the second Release is a no-op that does not reach the registry again
// (pinned with its doubles in registry_native_test.go).
func TestInternalReleasedInstance(t *testing.T) {
	inst := newMockInstance()
	inst.Release()

	if !inst.IsReleased() {
		t.Fatal("Release did not mark the instance released")
	}
	if _, err := inst.RequestAdapter(nil, nil); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("RequestAdapter after release = %v, want core.ErrReleased", err)
	}
	if got := inst.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL after release = %+v, want the zero Handles", got)
	}
	inst.Release()
}

// zeroValueAdapters returns the two zero-value receivers, each built fresh per
// case so a release in one case cannot leak into another.
func zeroValueAdapters() []struct {
	name string
	make func() *Adapter
} {
	return []struct {
		name string
		make func() *Adapter
	}{
		{"nil receiver", func() *Adapter { return nil }},
		{"zero struct", func() *Adapter { return &Adapter{} }},
	}
}

// TestInternalZeroValueAdapter pins the adapter's zero-value answers: no
// metadata, no state, no panic on the questions that are read-only, and
// RequestDevice answers core.ErrReleased because such an adapter has no instance
// to adopt the device (adapter_native.go:50 pre-move).
func TestInternalZeroValueAdapter(t *testing.T) {
	for _, recv := range zeroValueAdapters() {
		t.Run(recv.name, func(t *testing.T) {
			a := recv.make()

			if got := a.Info(); got != (gputypes.AdapterInfo{}) {
				t.Fatalf("Info = %+v, want the zero value", got)
			}
			if got := a.Features(); got != 0 {
				t.Fatalf("Features = %#x, want 0", got)
			}
			if got := a.Limits(); got != (gputypes.Limits{}) {
				t.Fatalf("Limits = %+v, want the zero value", got)
			}
			if got := a.DownlevelCapabilities(); got != (gputypes.DownlevelCapabilities{}) {
				t.Fatalf("DownlevelCapabilities = %+v, want the zero value", got)
			}
			if got := a.GetSurfaceCapabilities(nil); got != nil {
				t.Fatalf("GetSurfaceCapabilities(nil) = %+v, want nil", got)
			}
			if got := a.HAL(); got != (publichal.Handles{}) {
				t.Fatalf("HAL = %+v, want the zero Handles", got)
			}
			if _, _, err := a.RequestDevice(nil); !errors.Is(err, core.ErrReleased) {
				t.Fatalf("RequestDevice = %v, want core.ErrReleased", err)
			}
		})
	}
}

// TestInternalReleasedAdapter pins Release's idempotence on a zero-value
// adapter and the answers that follow it: a released adapter requests no more
// devices and reports no handles.
func TestInternalReleasedAdapter(t *testing.T) {
	a := &Adapter{}
	a.Release()
	a.Release() // idempotent

	if _, _, err := a.RequestDevice(nil); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("RequestDevice after release = %v, want core.ErrReleased", err)
	}
	if got := a.GetSurfaceCapabilities(nil); got != nil {
		t.Fatalf("GetSurfaceCapabilities after release = %+v, want nil", got)
	}
	if got := a.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL after release = %+v, want the zero Handles", got)
	}
}
