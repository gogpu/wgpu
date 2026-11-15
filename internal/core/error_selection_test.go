//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// TestRequestAdapterSentinelNoBackendRegistered pins ADR-083 §2.2's row "the
// mask selects no registered backend (compiled out or excluded)", in its
// software form: the build has no software backend, so a software-only mask
// selects nothing even though another backend is registered and has an adapter.
func TestRequestAdapterSentinelNoBackendRegistered(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
			fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU),
		}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	t.Cleanup(instance.Destroy)

	_, err := instance.RequestAdapter(nil)
	if !errors.Is(err, ErrNoBackendRegistered) {
		t.Fatalf("RequestAdapter error = %v, want ErrNoBackendRegistered", err)
	}
	if errors.Is(err, ErrNoHardwareAdapter) {
		t.Errorf("RequestAdapter error = %v, must not match ErrNoHardwareAdapter", err)
	}
}

// TestRequestAdapterSentinelNoHardwareAdapter pins ADR-083 §2.2's row "backends
// are registered but no adapter exists": the mask admits a registered backend
// whose enumeration yields no adapter.
func TestRequestAdapterSentinelNoHardwareAdapter(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance:  &fakeHALInstance{},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	_, err := instance.RequestAdapter(nil)
	if !errors.Is(err, ErrNoHardwareAdapter) {
		t.Fatalf("RequestAdapter error = %v, want ErrNoHardwareAdapter", err)
	}
	if errors.Is(err, ErrNoBackendRegistered) {
		t.Errorf("RequestAdapter error = %v, must not match ErrNoBackendRegistered", err)
	}
}

// TestRequestAdapterSentinelNoFallbackAdapter pins ADR-083 §2.2's row
// "ForceFallbackAdapter with software compiled out and no other CPU adapter":
// the only adapter is hardware, so nothing can serve a fallback request.
func TestRequestAdapterSentinelNoFallbackAdapter(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
			fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU),
		}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	_, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true})
	if !errors.Is(err, ErrNoFallbackAdapter) {
		t.Fatalf("RequestAdapter error = %v, want ErrNoFallbackAdapter", err)
	}
}

// TestRequestAdapterSentinelSurfaceIncompatible covers the condition ADR-083
// §2.2 has no row for: adapters exist, but none is compatible with the
// requested surface. It wraps ErrNoHardwareAdapter with the detail instead of
// introducing a fourth sentinel.
func TestRequestAdapterSentinelSurfaceIncompatible(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
			fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU),
		}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	// The resolver hands no backend a surface, so every adapter is skipped and
	// the candidate list stays empty.
	_, err := instance.requestAdapterWithSurfaceResolver(nil, func(gputypes.Backend) hal.Surface {
		return nil
	})
	if !errors.Is(err, ErrNoHardwareAdapter) {
		t.Fatalf("RequestAdapter error = %v, want ErrNoHardwareAdapter", err)
	}
	if !strings.Contains(err.Error(), "compatible with surface") {
		t.Errorf("RequestAdapter error = %v, want the surface detail in the message", err)
	}
}
