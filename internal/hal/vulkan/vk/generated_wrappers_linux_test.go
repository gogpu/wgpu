//go:build linux

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"syscall"
	"testing"
	"unsafe"
)

// structureTypeExportMemoryAllocateInfo is VK_STRUCTURE_TYPE_EXPORT_MEMORY_ALLOCATE_INFO.
const structureTypeExportMemoryAllocateInfo StructureType = 1000072002

// TestGetMemoryFdPropertiesKHR is Linux-only: dma-buf file descriptors exist
// only there, and syscall.Close takes an fd only on Unix.
func TestGetMemoryFdPropertiesKHR(t *testing.T) {
	c, instance := newTestInstance(t, "VK_KHR_surface")
	pd, device, _ := newTestDevice(t, c, instance, "VK_KHR_external_memory_fd", "VK_EXT_external_memory_dma_buf")
	c.getMemoryFdKHR = GetDeviceProcAddr(device, "vkGetMemoryFdKHR")
	c.getMemoryFdPropertiesKHR = GetDeviceProcAddr(device, "vkGetMemoryFdPropertiesKHR")

	export := ExportMemoryAllocateInfo{
		SType:       structureTypeExportMemoryAllocateInfo,
		HandleTypes: ExternalMemoryHandleTypeFlags(ExternalMemoryHandleTypeDmaBufBitExt),
	}
	memory := allocateHostVisible(t, c, pd, device, MemoryRequirements{Size: 1 << 16, MemoryTypeBits: ^uint32(0)}, unsafe.Pointer(&export))

	getFd := MemoryGetFdInfoKHR{SType: StructureTypeMemoryGetFdInfoKhr, Memory: memory, HandleType: ExternalMemoryHandleTypeDmaBufBitExt}
	fd := int32(-1)
	if r := c.GetMemoryFdKHR(device, &getFd, &fd); r != Success {
		t.Fatalf("vkGetMemoryFdKHR: %d", r)
	}
	defer syscall.Close(int(fd))

	props := MemoryFdPropertiesKHR{SType: StructureTypeMemoryFdPropertiesKhr}
	if r := c.GetMemoryFdPropertiesKHR(device, ExternalMemoryHandleTypeDmaBufBitExt, fd, &props); r != Success {
		t.Fatalf("vkGetMemoryFdPropertiesKHR(fd %d): %d", fd, r)
	}
	if props.MemoryTypeBits == 0 {
		t.Fatal("vkGetMemoryFdPropertiesKHR reported no memory types for an exported dma-buf")
	}
}
