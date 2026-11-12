//go:build !rust && !(js && wasm)

// Package defaultregtest checks the public default-registration contract from
// a binary that imports nothing else in the module: no test helper, no
// wgputest, no backend. That is what a normal program sees.
package defaultregtest

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
)

// TestSoftwareIsNotRegisteredByDefault: Phase 0 registers the platform
// hardware backends only. Software stays an explicit import until v0.35.0,
// where it becomes default-registered but outside the default mask
// (ADR-083 §2.1/§2.3, user decision 2026-10-18).
func TestSoftwareIsNotRegisteredByDefault(t *testing.T) {
	if backend, ok := hal.GetBackend(gputypes.BackendEmpty); ok {
		t.Fatalf("a BackendEmpty backend is registered by default (%T); software must stay an explicit import until v0.35.0", backend)
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
