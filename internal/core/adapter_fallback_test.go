//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// The tests in this file pin ADR-083 §2.3 as implemented (2026-11-16): software
// leaves the default mask, normal enumeration and Auto, and stays reachable
// through ForceFallbackAdapter under the default set (ADR-079's rows). The
// boundary is "enumeration unchanged, candidacy extended": instance.adapters
// never gains a software adapter, and only a force request sees the lazily
// created fallback candidates.

func hardwareTestProvider(adapters ...hal.ExposedAdapter) BackendProvider {
	return &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance:  &fakeHALInstance{adapters: adapters},
	}
}

func softwareTestProvider() BackendProvider {
	return &testProvider{
		variant:   gputypes.BackendSoftware,
		available: true,
		instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
			fakeExposed("Software Renderer", gputypes.BackendSoftware, gputypes.DeviceTypeCPU),
		}},
	}
}

func adapterBackend(t *testing.T, id AdapterID) gputypes.Backend {
	t.Helper()
	adapter, err := GetGlobal().Hub().GetAdapter(id)
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	return adapter.Backend
}

// TestForceFallbackAdapterReachesSoftwareUnderDefaultMask pins the core of the
// fix: with the default (zero) mask the software backend is a fallback
// candidate only — Auto keeps hardware, the enumeration list does not grow, and
// a ForceFallbackAdapter request returns software.
func TestForceFallbackAdapterReachesSoftwareUnderDefaultMask(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, hardwareTestProvider(fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU)), softwareTestProvider())

	instance := NewInstance(&gputypes.InstanceDescriptor{})
	t.Cleanup(instance.Destroy)

	autoID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("auto RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, autoID); got != gputypes.BackendVulkan {
		t.Fatalf("auto picked %v, want the Vulkan hardware adapter", got)
	}
	if got := len(instance.adapters); got != 1 {
		t.Fatalf("enumeration holds %d adapters, want 1: software must never be enumerated", got)
	}

	fallbackID, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("force RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, fallbackID); got != gputypes.BackendSoftware {
		t.Fatalf("ForceFallbackAdapter picked %v, want %v", got, gputypes.BackendSoftware)
	}
	if got := len(instance.adapters); got != 1 {
		t.Fatalf("enumeration grew to %d adapters after the force request: candidacy, not enumeration, was extended", got)
	}
}

// TestDefaultMaskKeepsSoftwareOutOfAuto pins the other half: a machine with no
// hardware adapter still reports the typed sentinel on Auto, and the fallback
// request is the way to reach software.
func TestDefaultMaskKeepsSoftwareOutOfAuto(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, hardwareTestProvider(), softwareTestProvider()) // hardware provider enumerates nothing

	instance := NewInstance(&gputypes.InstanceDescriptor{})
	t.Cleanup(instance.Destroy)

	if _, err := instance.RequestAdapter(nil); !errors.Is(err, ErrNoHardwareAdapter) {
		t.Fatalf("auto RequestAdapter error = %v, want ErrNoHardwareAdapter", err)
	}

	fallbackID, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("force RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, fallbackID); got != gputypes.BackendSoftware {
		t.Fatalf("ForceFallbackAdapter picked %v, want %v", got, gputypes.BackendSoftware)
	}
}

// TestExplicitSoftwareBitKeepsNormalEnumeration pins the explicit path: the bit
// enumerates software normally (hardware still ranks first in Auto), so the
// fallback admission above cannot change what an explicit choice sees.
func TestExplicitSoftwareBitKeepsNormalEnumeration(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, hardwareTestProvider(fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU)), softwareTestProvider())

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan | gputypes.BackendsSoftware})
	t.Cleanup(instance.Destroy)

	if got := len(instance.adapters); got != 2 {
		t.Fatalf("enumeration holds %d adapters, want 2 with the explicit software bit", got)
	}
	autoID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("auto RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, autoID); got != gputypes.BackendVulkan {
		t.Fatalf("auto picked %v, want the Vulkan hardware adapter", got)
	}
}

// TestDefaultSetAsExplicitMaskAdmitsFallback pins the reading of "the default
// set": a descriptor naming exactly the default mask value is the default, so a
// force request reaches software there too. The reduced-mask exclusion stays
// pinned by TestRequestAdapterSentinelNoFallbackAdapter.
func TestDefaultSetAsExplicitMaskAdmitsFallback(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, hardwareTestProvider(fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU)), softwareTestProvider())

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.DefaultInstanceDescriptor().Backends})
	t.Cleanup(instance.Destroy)

	fallbackID, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("force RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, fallbackID); got != gputypes.BackendSoftware {
		t.Fatalf("ForceFallbackAdapter picked %v, want %v", got, gputypes.BackendSoftware)
	}
}

// TestForceFallbackAdapterReachesSoftwareInSurfaceRequest pins the review's F1:
// the caller's surface map is built before the first force request and cannot
// carry a software key, so the fallback adapter must stay a candidate without a
// surface hint (its HAL surface is created on demand at configure).
func TestForceFallbackAdapterReachesSoftwareInSurfaceRequest(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, hardwareTestProvider(fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU)), softwareTestProvider())

	instance := NewInstance(&gputypes.InstanceDescriptor{})
	t.Cleanup(instance.Destroy)

	id, err := instance.RequestAdapterWithSurfaces(
		&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true},
		map[gputypes.Backend]hal.Surface{gputypes.BackendVulkan: nil},
	)
	if err != nil {
		t.Fatalf("force surface RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, id); got != gputypes.BackendSoftware {
		t.Fatalf("force surface request picked %v, want %v", got, gputypes.BackendSoftware)
	}
}

// TestDefaultMaskWithOnlySoftwareRegistered pins the ADR-083 §2.2 split on a
// build whose only registered backend is software: Auto reports
// ErrNoBackendRegistered (the hardware mask admitted nothing), and the force
// request still reaches software.
func TestDefaultMaskWithOnlySoftwareRegistered(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, softwareTestProvider())

	instance := NewInstance(&gputypes.InstanceDescriptor{})
	t.Cleanup(instance.Destroy)

	if _, err := instance.RequestAdapter(nil); !errors.Is(err, ErrNoBackendRegistered) {
		t.Fatalf("auto RequestAdapter error = %v, want ErrNoBackendRegistered", err)
	}
	id, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("force RequestAdapter: %v", err)
	}
	if got := adapterBackend(t, id); got != gputypes.BackendSoftware {
		t.Fatalf("ForceFallbackAdapter picked %v, want %v", got, gputypes.BackendSoftware)
	}
}

// TestMockInstanceDoesNotReachSoftwareFallback pins the review's F3: a mock
// instance is a test double; a force request on it must not create the real
// software backend.
func TestMockInstanceDoesNotReachSoftwareFallback(t *testing.T) {
	GetGlobal().Clear()
	instance := NewInstanceWithMock(nil)
	t.Cleanup(instance.Destroy)

	if _, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true}); !errors.Is(err, ErrNoFallbackAdapter) {
		t.Fatalf("force RequestAdapter on a mock instance error = %v, want ErrNoFallbackAdapter", err)
	}
}
