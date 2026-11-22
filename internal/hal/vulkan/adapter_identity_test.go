//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
)

// TestIdentityFromProperties pins the assembly of the adapter identity from
// the queried Vulkan property structs (ADR-084 §2.3, §6): the UUID is copied
// byte for byte, the LUID only when deviceLUIDValid is set, and the PCI
// locator only when the struct was chained.
func TestIdentityFromProperties(t *testing.T) {
	// The CMP 30HX on the development host (ADR-084 §3):
	// nvidia-smi uuid GPU-288bbc7b-41e0-9cf2-8ff0-5918509a0240 and
	// deviceLUID d5e26d2b-00000000, i.e. the 8 bytes below little-endian.
	uuid := [16]uint8{0x28, 0x8b, 0xbc, 0x7b, 0x41, 0xe0, 0x9c, 0xf2, 0x8f, 0xf0, 0x59, 0x18, 0x50, 0x9a, 0x02, 0x40}
	luidBytes := [8]uint8{0x2b, 0x6d, 0xe2, 0xd5, 0, 0, 0, 0}
	const luid = uint64(0xd5e26d2b)

	tests := []struct {
		name string
		id   *vk.PhysicalDeviceIDProperties
		pci  *vk.PhysicalDevicePCIBusInfoPropertiesEXT
		want gputypes.AdapterIdentity
	}{
		{
			name: "full",
			id: &vk.PhysicalDeviceIDProperties{
				DeviceUUID:      uuid,
				DeviceLUID:      luidBytes,
				DeviceLUIDValid: 1,
			},
			pci: &vk.PhysicalDevicePCIBusInfoPropertiesEXT{
				PciDomain:   0,
				PciBus:      4,
				PciDevice:   0,
				PciFunction: 0,
			},
			want: gputypes.AdapterIdentity{
				DeviceUUID:  gputypes.DeviceUUID(uuid),
				PCIBusID:    gputypes.PCIBusID{Domain: 0, Bus: 4, Device: 0, Function: 0},
				AdapterLUID: luid,
			},
		},
		{
			name: "zero UUID and invalid LUID",
			id: &vk.PhysicalDeviceIDProperties{
				DeviceLUID:      luidBytes,
				DeviceLUIDValid: 0,
			},
			pci:  nil,
			want: gputypes.AdapterIdentity{},
		},
		{
			name: "PCI without ID properties",
			pci: &vk.PhysicalDevicePCIBusInfoPropertiesEXT{
				PciDomain: 0xFFFF, PciBus: 0xAB, PciDevice: 0xCD, PciFunction: 0xA,
			},
			want: gputypes.AdapterIdentity{
				PCIBusID: gputypes.PCIBusID{Domain: 0xFFFF, Bus: 0xAB, Device: 0xCD, Function: 0xA},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identityFromProperties(tt.id, tt.pci)
			if got != tt.want {
				t.Fatalf("identityFromProperties() = %+v, want %+v", got, tt.want)
			}
			if tt.name == "zero UUID and invalid LUID" && !got.IsZero() {
				t.Fatalf("identityFromProperties() = %+v, want all-zero", got)
			}
		})
	}
}

// TestIdentityLUIDPacking pins that Vulkan's 8 deviceLUID bytes read
// little-endian equal uint64(uint32(HighPart))<<32 | uint64(LowPart), the
// packing DX12 uses for the same LUID (ADR-084 §2.1, §6).
func TestIdentityLUIDPacking(t *testing.T) {
	tests := []struct {
		name string
		high int32
		low  uint32
	}{
		{name: "host CMP 30HX", high: 0, low: 0xd5e26d2b},
		{name: "both parts set", high: 0x12345678, low: 0x9abcdef0},
		{name: "high sign bit", high: -2147483648, low: 1},
		{name: "all ones", high: -1, low: 0xffffffff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := uint64(uint32(tt.high))<<32 | uint64(tt.low)

			id := &vk.PhysicalDeviceIDProperties{DeviceLUIDValid: 1}
			for j := range 4 {
				id.DeviceLUID[j] = byte(tt.low >> (8 * j))
				id.DeviceLUID[4+j] = byte(uint32(tt.high) >> (8 * j))
			}

			got := identityFromProperties(id, nil).AdapterLUID
			if got != want {
				t.Fatalf("AdapterLUID = %#016x, want %#016x", got, want)
			}
		})
	}
}
