//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// These tests pin the registry contract the root declaration layer relies on
// (ADR-070 S12): adoption rejects nil and a released instance, the device
// registry removes by the shared view, and Release reaches every adopted device
// and surface before it destroys the core instance. The device double is a
// deviceRef without any internal/device type, per the white-box rule.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
	surfacepkg "github.com/gogpu/wgpu/internal/surface"
)

// newMockInstance builds an instance over a mock core instance, without a GPU.
func newMockInstance() *Instance {
	return &Instance{core: core.NewInstanceWithMock(nil)}
}

// newMockSurface builds a surface implementation with no HAL surface: enough
// for the registry, whose entries are released through the surface's own
// nil-safe Destroy path.
func newMockSurface() *surfacepkg.Surface {
	return surfacepkg.NewFromHAL(core.NewSurface(nil, "registry-test"))
}

func TestAdoptDeviceRejectsNilAndReleased(t *testing.T) {
	inst := newMockInstance()

	if err := inst.AdoptDevice(nil); err == nil {
		t.Fatal("AdoptDevice accepted nil")
	}

	device := &fakeDeviceRef{}
	if err := inst.AdoptDevice(device); err != nil {
		t.Fatalf("AdoptDevice: %v", err)
	}

	inst.Release()
	if err := inst.AdoptDevice(&fakeDeviceRef{}); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("AdoptDevice after release = %v, want ErrReleased", err)
	}
}

func TestAdoptSurfaceRejectsNilAndReleased(t *testing.T) {
	inst := newMockInstance()

	if err := inst.AdoptSurface(nil); err == nil {
		t.Fatal("AdoptSurface accepted nil")
	}

	s := newMockSurface()
	if err := inst.AdoptSurface(s); err != nil {
		t.Fatalf("AdoptSurface: %v", err)
	}

	inst.Release()
	if err := inst.AdoptSurface(newMockSurface()); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("AdoptSurface after release = %v, want ErrReleased", err)
	}
}

func TestReleaseReachesAdoptedDevicesAndSurfaces(t *testing.T) {
	inst := newMockInstance()
	device := &fakeDeviceRef{}
	s := newMockSurface()
	if err := inst.AdoptDevice(device); err != nil {
		t.Fatalf("AdoptDevice: %v", err)
	}
	if err := inst.AdoptSurface(s); err != nil {
		t.Fatalf("AdoptSurface: %v", err)
	}

	inst.Release()

	if !device.released {
		t.Fatal("Release did not release the adopted device")
	}
	if !s.Released() {
		t.Fatal("Release did not release the adopted surface")
	}
	if !inst.IsReleased() {
		t.Fatal("Release did not mark the instance released")
	}

	// A second release is a no-op and must not reach the doubles again.
	device.released = false
	inst.Release()
	if device.released {
		t.Fatal("a second Release reached the adopted device")
	}
}

func TestUnregisterDeviceRemovesTheRegistryEntry(t *testing.T) {
	inst := newMockInstance()
	device := &fakeDeviceRef{}
	if err := inst.AdoptDevice(device); err != nil {
		t.Fatalf("AdoptDevice: %v", err)
	}

	inst.UnregisterDevice(device)
	inst.Release()

	if device.released {
		t.Fatal("Release reached a device that had unregistered itself")
	}
}

func TestNilInstanceAnswersReleased(t *testing.T) {
	var inst *Instance
	if !inst.IsReleased() {
		t.Fatal("nil instance did not report released state")
	}
	if err := inst.AdoptDevice(&fakeDeviceRef{}); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("nil AdoptDevice = %v, want ErrReleased", err)
	}
	// Releasing a nil instance is a no-op.
	inst.Release()
}
