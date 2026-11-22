//go:build windows && (amd64 || arm64) && !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestIdentityMatchesNVMLOnHost is the empirical half of ADR-084 §3: on real
// NVIDIA hardware the adapter identity the Vulkan backend reports must render
// exactly the UUID and PCI bus id nvidia-smi prints, and the Vulkan deviceLUID
// must equal the DXGI adapter LUID (§6). It also checks that an Intel adapter
// on the same instance reports a UUID without claiming NVML correspondence.
//
// It needs a real NVIDIA GPU and nvidia-smi, which CI has neither: the test
// skips unless WGPU_REQUIRE lists identity, and fails instead of passing
// vacuously on a host that promises the coverage (LEAD-013).
func TestIdentityMatchesNVMLOnHost(t *testing.T) {
	if !requireEnvironmentItem("identity") {
		t.Skip("needs a real NVIDIA GPU and nvidia-smi (absent in CI); set WGPU_REQUIRE=identity to make this a failure")
	}

	vulkanAdapters := enumerateAdapters(t, gputypes.BackendsVulkan)
	// The same physical card can be enumerated twice on Windows (the mining
	// card appears under a second name with an all-zero UUID), so the pick is
	// "an NVIDIA-named adapter that actually reports a UUID", never "the first".
	nvidia := findVulkanAdapterWithUUID(t, vulkanAdapters, "NVIDIA")
	identity := nvidia.Identity()

	if identity.PCIBusID.IsZero() {
		t.Errorf("Vulkan %q PCIBusID is zero, want its PCI slot", nvidia.Info().Name)
	}
	if identity.AdapterLUID == 0 {
		t.Errorf("Vulkan %q AdapterLUID is zero, want a valid deviceLUID", nvidia.Info().Name)
	}

	gotUUID := identity.DeviceUUID.NVMLString()
	gotPCI := identity.PCIBusID.NVMLString()
	gpus := queryNvidiaSMI(t)
	line := selectNVMLGPU(gpus, gotUUID, gotPCI)
	if line == nil {
		t.Fatalf("no nvidia-smi line matches the Vulkan %q (uuid %q, pci %q); nvidia-smi lines: %v",
			nvidia.Info().Name, gotUUID, gotPCI, gpus)
	}
	if !strings.EqualFold(gotUUID, line.uuid) {
		t.Errorf("DeviceUUID.NVMLString() = %q, nvidia-smi uuid = %q", gotUUID, line.uuid)
	}
	if !strings.EqualFold(gotPCI, line.pci) {
		t.Errorf("PCIBusID.NVMLString() = %q, nvidia-smi pci.bus_id = %q", gotPCI, line.pci)
	}

	// The Vulkan spec ties deviceLUID to the corresponding DXGI adapter LUID, so
	// the proof is "a DX12 adapter carries exactly this LUID" — matching by LUID,
	// not by name or order.
	dx12Adapters := enumerateAdapters(t, gputypes.BackendsDX12)
	dx12NVIDIA := findAdapterByLUID(t, dx12Adapters, identity.AdapterLUID)
	if got := dx12NVIDIA.Identity().AdapterLUID; got != identity.AdapterLUID {
		t.Errorf("DX12 %q AdapterLUID = 0x%X, want the Vulkan deviceLUID 0x%X",
			dx12NVIDIA.Info().Name, got, identity.AdapterLUID)
	}

	// An Intel adapter reports a UUID too; NVMLString renders any Vulkan UUID,
	// so it must not panic and says nothing about NVML management. The check is
	// conditional: not every host that promises identity has an Intel GPU.
	var intelUUID, intelPCI string
	for _, a := range vulkanAdapters {
		if strings.Contains(a.Info().Name, "Iris") {
			intelID := a.Identity()
			if intelID.DeviceUUID.IsZero() {
				t.Errorf("Intel %q DeviceUUID is zero, want a reported 16-byte UUID", a.Info().Name)
			}
			intelUUID = intelID.DeviceUUID.NVMLString()
			intelPCI = intelID.PCIBusID.NVMLString()
		}
	}

	t.Logf("NVIDIA: uuid=%q pci=%q luid=0x%X; nvidia-smi: uuid=%q pci=%q; DX12 LUID=0x%X (matched); Intel: uuid=%q pci=%q",
		gotUUID, gotPCI, identity.AdapterLUID, line.uuid, line.pci, identity.AdapterLUID, intelUUID, intelPCI)
}

// findVulkanAdapterWithUUID returns the adapter whose name contains substr and
// whose DeviceUUID is non-zero, or fails listing every adapter with its UUID
// state — the host enumerates some cards twice, once without a UUID.
func findVulkanAdapterWithUUID(t *testing.T, adapters []*wgpu.Adapter, substr string) *wgpu.Adapter {
	t.Helper()
	for _, a := range adapters {
		if strings.Contains(a.Info().Name, substr) && !a.Identity().DeviceUUID.IsZero() {
			return a
		}
	}
	var seen []string
	for _, a := range adapters {
		seen = append(seen, fmt.Sprintf("%q(uuid-zero=%v)", a.Info().Name, a.Identity().DeviceUUID.IsZero()))
	}
	t.Fatalf("no %s adapter with a non-zero DeviceUUID in the Vulkan enumeration (%v), but WGPU_REQUIRE lists identity", substr, seen)
	return nil
}

// findAdapterByLUID returns the adapter carrying exactly luid, or fails listing
// the names and LUIDs that were present.
func findAdapterByLUID(t *testing.T, adapters []*wgpu.Adapter, luid uint64) *wgpu.Adapter {
	t.Helper()
	for _, a := range adapters {
		if a.Identity().AdapterLUID == luid {
			return a
		}
	}
	var seen []string
	for _, a := range adapters {
		seen = append(seen, fmt.Sprintf("%q(luid=0x%X)", a.Info().Name, a.Identity().AdapterLUID))
	}
	t.Fatalf("no DX12 adapter carries the Vulkan deviceLUID 0x%X (%v); the Vulkan spec requires the corresponding IDXGIAdapter1 LUID to be equal", luid, seen)
	return nil
}

// requireEnvironmentItem reports whether the comma-separated WGPU_REQUIRE list
// names name, case-insensitively (the ecosystem's <REPO>_REQUIRE switch).
func requireEnvironmentItem(name string) bool {
	for _, item := range strings.Split(os.Getenv("WGPU_REQUIRE"), ",") {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return true
		}
	}
	return false
}

// enumerateAdapters creates an instance for backends and returns its adapters.
// The instance is released when the test ends; the adapters belong to it.
func enumerateAdapters(t *testing.T, backends gputypes.Backends) []*wgpu.Adapter {
	t.Helper()
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: backends})
	if err != nil {
		t.Fatalf("CreateInstance(%v): %v", backends, err)
	}
	t.Cleanup(inst.Release)
	return inst.EnumerateAdapters()
}

// nvmlGPU is one nvidia-smi line: the UUID and PCI bus id of a GPU.
type nvmlGPU struct {
	uuid string
	pci  string
}

// String renders the pair for a failure message.
func (g nvmlGPU) String() string {
	return fmt.Sprintf("{uuid:%q pci:%q}", g.uuid, g.pci)
}

// queryNvidiaSMI asks nvidia-smi for the UUID and PCI bus id of every GPU it
// reports. A missing binary is a failure here: the test only runs gated on
// WGPU_REQUIRE=identity, where absence cannot pass silently.
func queryNvidiaSMI(t *testing.T) []nvmlGPU {
	t.Helper()
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		t.Fatalf("nvidia-smi not found on PATH, but WGPU_REQUIRE lists identity: %v", err)
	}
	out, err := exec.Command(path, "--query-gpu=uuid,pci.bus_id", "--format=csv,noheader").Output()
	if err != nil {
		t.Fatalf("nvidia-smi --query-gpu: %v", err)
	}
	var gpus []nvmlGPU
	for _, raw := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			t.Fatalf("nvidia-smi line %q: got %d fields, want uuid,pci.bus_id", line, len(parts))
		}
		gpus = append(gpus, nvmlGPU{uuid: strings.TrimSpace(parts[0]), pci: strings.TrimSpace(parts[1])})
	}
	if len(gpus) == 0 {
		t.Fatalf("nvidia-smi reported no GPUs, but an NVIDIA Vulkan adapter was enumerated")
	}
	return gpus
}

// selectNVMLGPU picks the nvidia-smi line for the adapter: the only line, or the
// one matching by UUID and then by PCI bus id when several are present.
func selectNVMLGPU(gpus []nvmlGPU, uuid, pci string) *nvmlGPU {
	if len(gpus) == 1 {
		return &gpus[0]
	}
	for i := range gpus {
		if strings.EqualFold(gpus[i].uuid, uuid) {
			return &gpus[i]
		}
	}
	for i := range gpus {
		if strings.EqualFold(gpus[i].pci, pci) {
			return &gpus[i]
		}
	}
	return nil
}
