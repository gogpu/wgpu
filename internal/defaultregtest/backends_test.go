//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package defaultregtest_test checks the public default-registration contract
// from a binary that imports nothing else in the module: no test helper, no
// wgputest, no backend. That is what a normal program sees.
//
// It is an external test package on purpose (ADR-070 §15.10): a white-box test
// in a child package is a finding, an external one is a test-only consumer.
package defaultregtest_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// emptyMask is the backend flag a registration for gputypes.BackendEmpty would
// answer to: every flag follows BackendsX == 1 << BackendX, and the no-op
// backend is the only one that reports BackendEmpty.
const emptyMask = gputypes.Backends(1 << gputypes.BackendEmpty)

// TestNoopIsNotRegisteredByDefault: the no-op backend is a test double and
// belongs to no default binary. Since v0.35.0 it owns the BackendEmpty variant
// alone — software reports BackendSoftware (ADR-083 §2.3) — so a BackendEmpty
// registration here would be a no-op leaking into production. Registration is
// observable through the public selection: the mask below admits no registered
// backend, while a registration answering to it would leave the backend
// registered with no adapter on this machine (ErrNoHardwareAdapter).
func TestNoopIsNotRegisteredByDefault(t *testing.T) {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: emptyMask})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer instance.Release()

	_, err = instance.RequestAdapter(nil)
	if !errors.Is(err, wgpu.ErrNoBackendRegistered) {
		t.Fatalf("RequestAdapter error = %v, want ErrNoBackendRegistered; a backend registered for BackendEmpty would make this ErrNoHardwareAdapter", err)
	}
	if errors.Is(err, wgpu.ErrNoHardwareAdapter) {
		t.Errorf("RequestAdapter error = %v, must not match ErrNoHardwareAdapter", err)
	}
}

// TestVulkanOnlyMaskWithoutVulkanHasNoAdapter: a program that imported only
// Vulkan got no adapter on a machine without Vulkan before Phase 0 and must
// still get none. The software backend's mask bypass made this the exact place
// where a silent CPU fallback would appear.
func TestVulkanOnlyMaskWithoutVulkanHasNoAdapter(t *testing.T) {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return // the expected result on machines without Vulkan (CI macOS and Windows)
	}
	defer adapter.Release()
	if got := adapter.Info().Backend; got != gputypes.BackendVulkan {
		t.Fatalf("Vulkan-only mask returned backend %v (%q); software must never be a silent fallback",
			got, adapter.Info().Name)
	}
}

// TestDefaultRegistrationKeepsAdapterPick: registering every platform backend
// must not change the adapter an existing program picks. With software not
// registered, the mask of the picked backend contains that backend alone.
func TestDefaultRegistrationKeepsAdapterPick(t *testing.T) {
	all, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance(nil): %v", err)
	}
	defer all.Release()

	base, err := all.RequestAdapter(nil)
	if err != nil {
		t.Skipf("no adapter on this machine: %v", err)
	}
	defer base.Release()
	baseInfo := base.Info()

	assertSamePick(t, "picked backend only", baseInfo, maskOf(baseInfo.Backend))
	if hasVulkanAdapter(t) {
		assertSamePick(t, "vulkan only", baseInfo, gputypes.BackendsVulkan)
	}
}

func maskOf(backend gputypes.Backend) gputypes.Backends {
	if backend == gputypes.BackendEmpty {
		return 0
	}
	return 1 << backend
}

func hasVulkanAdapter(t *testing.T) bool {
	t.Helper()
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	if err != nil {
		return false
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return false
	}
	defer adapter.Release()
	return adapter.Info().Backend == gputypes.BackendVulkan
}

func assertSamePick(t *testing.T, name string, want gputypes.AdapterInfo, mask gputypes.Backends) {
	t.Helper()
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: mask})
	if err != nil {
		t.Fatalf("%s: CreateInstance: %v", name, err)
	}
	defer instance.Release()
	got, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("%s: RequestAdapter: %v", name, err)
	}
	defer got.Release()
	info := got.Info()
	if info.Backend != want.Backend || info.Name != want.Name ||
		info.VendorID != want.VendorID || info.DeviceID != want.DeviceID {
		t.Errorf("%s: pick = %s (%s, vendor %#x, device %#x, backend %s), want %s (%s, vendor %#x, device %#x, backend %s)",
			name,
			info.Name, info.Driver, info.VendorID, info.DeviceID, info.Backend,
			want.Name, want.Driver, want.VendorID, want.DeviceID, want.Backend)
	}
}
