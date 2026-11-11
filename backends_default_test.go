//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestDefaultRegistrationKeepsAdapterPick guards ADR-083 §2.1: default-on
// backend registration must not change the adapter an existing program picks.
//
// A program that blank-imported only Vulkan registered one backend; under
// CreateInstance(nil) its default mask (gputypes.BackendsPrimary) therefore
// saw Vulkan alone and picked the first Vulkan adapter. With every platform
// backend registered, the same adapter must still win: providerPriority puts
// Vulkan first, so the extra backends must not outrank it.
func TestDefaultRegistrationKeepsAdapterPick(t *testing.T) {
	all, err := CreateInstance(nil)
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

	// The pick must not change when only the picked backend is visible.
	assertSamePick(t, "picked backend only", baseInfo, backendMask(baseInfo.Backend))

	// A program that registered only Vulkan saw Vulkan alone. Where a real
	// Vulkan adapter exists, the full registration must pick the same one.
	// The mask still admits the software backend in this release (ADR-083
	// removes the bypass in v0.35.0), so the adapter must be checked for
	// Backend == Vulkan, not merely for existing.
	if hasAdapterFor(t, gputypes.BackendVulkan) {
		assertSamePick(t, "vulkan only", baseInfo, backendMask(gputypes.BackendVulkan))
	}
}

// backendMask returns the Backends mask that selects exactly one backend.
// The software/noop provider is registered under BackendEmpty and is selected
// through the empty mask in this release (ADR-083 removes that bypass).
func backendMask(b gputypes.Backend) gputypes.Backends {
	if b == gputypes.BackendEmpty {
		return 0
	}
	return 1 << b
}

// hasAdapterFor reports whether the pick for a single-backend mask really is
// that backend. The software backend is still admitted by every mask in this
// release, so existence alone does not prove the backend is present.
func hasAdapterFor(t *testing.T, backend gputypes.Backend) bool {
	t.Helper()
	inst, err := CreateInstance(&InstanceDescriptor{Backends: backendMask(backend)})
	if err != nil {
		return false
	}
	defer inst.Release()
	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		return false
	}
	defer adapter.Release()
	return adapter.Info().Backend == backend
}

func assertSamePick(t *testing.T, name string, want gputypes.AdapterInfo, mask gputypes.Backends) {
	t.Helper()
	inst, err := CreateInstance(&InstanceDescriptor{Backends: mask})
	if err != nil {
		t.Fatalf("%s: CreateInstance: %v", name, err)
	}
	defer inst.Release()
	got, err := inst.RequestAdapter(nil)
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
