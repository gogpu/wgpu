//go:build rust

package wgpu

// This file is the public declaration layer over internal/instance on the rust
// build (ADR-070 S12). The implementation moved to internal/instance; what stays
// here is the frozen public Adapter type, the descriptor boundary and the root
// instance identity cache the returned device records.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/instance"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// Adapter represents a physical GPU.
// On Rust backend, this wraps go-webgpu/webgpu Adapter.
//
// The implementation lives in internal/instance (ADR-070 S12); this type is the
// declaration layer the public API is frozen on. It keeps the root instance the
// adapter was requested from, written once at construction: the returned device
// records it as its identity cache.
type Adapter struct {
	impl     *instance.Adapter
	instance *Instance
}

// Info returns adapter metadata.
func (a *Adapter) Info() gputypes.AdapterInfo { return a.impl.Info() }

// Features returns supported features.
func (a *Adapter) Features() gputypes.Features { return a.impl.Features() }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() gputypes.Limits { return a.impl.Limits() }

// DownlevelCapabilities returns backend capability flags for downlevel adapters.
// Rust wgpu handles real values internally; from Go FFI we return fully compliant defaults.
//
// A nil receiver answers them too: the pre-move method was a constant and
// ignored the receiver, and the declaration layer must not turn that into a
// panic (ADR-070 S12 settle).
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	if a == nil {
		return gputypes.DefaultDownlevelCapabilities()
	}
	return a.impl.DownlevelCapabilities()
}

// Identity returns the hardware identity a backend can report for this adapter
// (ADR-084). This is a native extension beyond the WebGPU specification, which
// defines no per-device identifier: the zero value means the backend reported
// none, and that is never an error. The browser, GLES, the software backend and
// the -tags rust (wgpu-native) binding report zeros on every field.
//
// The fields have different lifetimes, so the promise is per identifier:
//
// | Identifier | Promise | Evidence |
// |---|---|---|
// | `DeviceUUID` (Vulkan) | **Persistent**: "must be immutable for a given device across instances, processes, driver APIs, driver versions, and system reboots"; per-device, not per-model; **not** a serial (may change if hardware is moved between slots while powered off); conformance cannot prove uniqueness → all-zero = unknown, callers guard collisions | Vulkan `VkPhysicalDeviceIDProperties` refpage; `VK_UUID_SIZE`=16 |
// | `PCIBusID` (Vulkan) | **Persistent while the card stays in its slot**; per-slot, so it distinguishes identical cards. Optional: "not all physical devices have PCI bus information" — zero when the extension is absent | `VK_EXT_pci_bus_info` refpage (device extension, ratified, not core) |
// | `AdapterLUID` (DX12; Vulkan on Windows; MoltenVK) | **Session-local only**: "changes whenever the system is rebooted / the adapter's driver is updated / the adapter is disabled / the adapter is disconnected"; stable across processes within one boot; on Windows Vulkan `deviceLUID` "must be equal to" the corresponding `IDXGIAdapter1` LUID; on MoltenVK it is the registry ID | WDK `D3DKMT_ADAPTERINFO`; DXCore docs; Vulkan `deviceLUID` wording; MoltenVK PR #1908 |
// | `MetalRegistryID` (native Metal) | **Instance-local**: an IORegistry handle, "global to all tasks" (Apple) but MoltenVK's maintainers found it **not constant across OS reboots** — a same-process identifier, never a rental matcher | Apple `MTLDevice.registryID`; MoltenVK #1880 / PR #1908 |
// | NVML correspondence (NVIDIA) | `NVMLString()` renders **the form NVML uses** (`nvidia-smi`, `NVIDIA_VISIBLE_DEVICES`); it is not a claim that the adapter is an NVML device — every Vulkan adapter's bytes render this way. **Byte order is an empirical contract**: NVIDIA documents Vulkan `deviceUUID` == CUDA `uuid` byte-for-byte (CUDA C Programming Guide, "Matching device UUIDs"); NVML renders that as `GPU-` + 8-4-4-4-12. Verified on our hardware (O1 closed, §3) | NVIDIA CUDA guide; `nvml.h` `NVML_DEVICE_UUID_ASCII_LEN 41` / `_BINARY_LEN 16`; NVML `nvmlDeviceGetUUID` |
//
// Callers match on DeviceUUID first and fall back to PCIBusID when two adapters
// collide (conformance cannot guarantee uniqueness); keep adapters distinct by
// enumeration index as the last resort — the index is not an identity, only a
// tie-breaker. "NVML" in the helper names means the form NVML uses, never "the
// NVML name of this adapter": those bytes exist for every Vulkan adapter,
// including Intel and AMD ones that NVML does not manage.
//
// On macOS through MoltenVK, DeviceUUID is a synthetic vendor+device+location
// value, so two identical single-GPU Macs report the same bytes.
func (a *Adapter) Identity() gputypes.AdapterIdentity { return a.impl.Identity() }

// RequestDevice creates a logical device from this adapter.
// If desc is nil, default features and limits are used.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
	// The descriptor boundary (shape 1): the public descriptor becomes the
	// implementation's mirror, and a nil pointer stays nil.
	var implDesc *instance.DeviceDescriptor
	if desc != nil {
		implDesc = &instance.DeviceDescriptor{
			Label:            desc.Label,
			RequiredFeatures: desc.RequiredFeatures,
			RequiredLimits:   desc.RequiredLimits,
		}
	}

	impl, q, err := a.impl.RequestDevice(implDesc)
	if err != nil {
		return nil, err
	}
	return &Device{impl: impl, instance: a.instance, queue: &Queue{impl: q}}, nil
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	Formats      []gputypes.TextureFormat
	PresentModes []gputypes.PresentMode
	AlphaModes   []gputypes.CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
//
// A nil receiver answers nil. Unlike DownlevelCapabilities, the pre-move method
// read its receiver (a.released) and panicked on a nil one, so this guard is a
// safe-direction delta, not a restored default (ADR-070 S12 settle, P2-CORE-034).
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	if a == nil || surface == nil {
		return nil
	}
	caps := a.impl.GetSurfaceCapabilities(surface.impl)
	if caps == nil {
		return nil
	}
	return &SurfaceCapabilities{
		Formats:      caps.Formats,
		PresentModes: caps.PresentModes,
		AlphaModes:   caps.AlphaModes,
	}
}

// Release releases the adapter.
func (a *Adapter) Release() {
	a.impl.Release()
}
