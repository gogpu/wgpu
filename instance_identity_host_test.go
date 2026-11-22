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
	nvidia := mustFindAdapter(t, vulkanAdapters, "NVIDIA", "Vulkan")
	identity := nvidia.Identity()

	if identity.DeviceUUID.IsZero() {
		t.Errorf("Vulkan %q DeviceUUID is zero, want the nvidia-smi UUID", nvidia.Info().Name)
	}
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

	dx12Adapters := enumerateAdapters(t, gputypes.BackendsDX12)
	dx12NVIDIA := mustFindAdapter(t, dx12Adapters, "NVIDIA", "DX12")
	dx12LUID := dx12NVIDIA.Identity().AdapterLUID
	if dx12LUID == 0 {
		t.Errorf("DX12 %q AdapterLUID is zero, want the DXGI adapter LUID", dx12NVIDIA.Info().Name)
	}
	if dx12LUID != identity.AdapterLUID {
		t.Errorf("LUID mismatch across APIs: Vulkan deviceLUID 0x%X, DXGI AdapterLuid 0x%X; the Vulkan spec requires them equal",
			identity.AdapterLUID, dx12LUID)
	}

	// An Intel adapter reports a UUID too; NVMLString renders any Vulkan UUID,
	// so it must not panic and says nothing about NVML management (§3).
	intel := mustFindAdapter(t, vulkanAdapters, "Iris", "Vulkan")
	intelID := intel.Identity()
	if intelID.DeviceUUID.IsZero() {
		t.Errorf("Intel %q DeviceUUID is zero, want a reported 16-byte UUID", intel.Info().Name)
	}
	_ = intelID.DeviceUUID.NVMLString()

	t.Logf("NVIDIA: uuid=%q pci=%q luid=0x%X; nvidia-smi: uuid=%q pci=%q; DXGI LUID=0x%X; Intel: uuid=%q pci=%q",
		gotUUID, gotPCI, identity.AdapterLUID, line.uuid, line.pci, dx12LUID,
		intelID.DeviceUUID.NVMLString(), intelID.PCIBusID.NVMLString())
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

// mustFindAdapter returns the first adapter whose Info().Name contains substr,
// or fails the test naming the adapters that were present.
func mustFindAdapter(t *testing.T, adapters []*wgpu.Adapter, substr, backend string) *wgpu.Adapter {
	t.Helper()
	for _, a := range adapters {
		if strings.Contains(a.Info().Name, substr) {
			return a
		}
	}
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = a.Info().Name
	}
	t.Fatalf("no %s adapter in the %s enumeration (%v), but WGPU_REQUIRE lists identity", substr, backend, names)
	return nil
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
