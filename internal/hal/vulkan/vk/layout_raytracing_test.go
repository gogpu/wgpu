//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"testing"
	"unsafe"
)

// TestRayTracingStructLayout pins the ray tracing structs passed to the driver
// to their C ABI layout on 64-bit platforms. A mismatch here corrupts every
// acceleration structure build silently, so the offsets are checked explicitly.
func TestRayTracingStructLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout expectations are for 64-bit platforms")
	}

	check := func(name string, got, want uintptr) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}

	// VkAccelerationStructureGeometryDataKHR is a union whose largest member
	// (the triangles data) contains 64-bit fields, so it must be 8-byte aligned.
	var geomData AccelerationStructureGeometryDataKHR
	check("sizeof(AccelerationStructureGeometryDataKHR)", unsafe.Sizeof(geomData), 64)
	check("alignof(AccelerationStructureGeometryDataKHR)", unsafe.Alignof(geomData), 8)

	var geom AccelerationStructureGeometryKHR
	check("sizeof(AccelerationStructureGeometryKHR)", unsafe.Sizeof(geom), 96)
	check("offsetof(AccelerationStructureGeometryKHR.GeometryType)", unsafe.Offsetof(geom.GeometryType), 16)
	check("offsetof(AccelerationStructureGeometryKHR.Geometry)", unsafe.Offsetof(geom.Geometry), 24)
	check("offsetof(AccelerationStructureGeometryKHR.Flags)", unsafe.Offsetof(geom.Flags), 88)

	var tri AccelerationStructureGeometryTrianglesDataKHR
	check("sizeof(AccelerationStructureGeometryTrianglesDataKHR)", unsafe.Sizeof(tri), 64)
	check("offsetof(Triangles.VertexFormat)", unsafe.Offsetof(tri.VertexFormat), 16)
	check("offsetof(Triangles.VertexData)", unsafe.Offsetof(tri.VertexData), 24)
	check("offsetof(Triangles.VertexStride)", unsafe.Offsetof(tri.VertexStride), 32)
	check("offsetof(Triangles.MaxVertex)", unsafe.Offsetof(tri.MaxVertex), 40)
	check("offsetof(Triangles.IndexType)", unsafe.Offsetof(tri.IndexType), 44)
	check("offsetof(Triangles.IndexData)", unsafe.Offsetof(tri.IndexData), 48)
	check("offsetof(Triangles.TransformData)", unsafe.Offsetof(tri.TransformData), 56)

	var aabbs AccelerationStructureGeometryAabbsDataKHR
	check("sizeof(AccelerationStructureGeometryAabbsDataKHR)", unsafe.Sizeof(aabbs), 32)

	var inst AccelerationStructureGeometryInstancesDataKHR
	check("sizeof(AccelerationStructureGeometryInstancesDataKHR)", unsafe.Sizeof(inst), 32)
	check("offsetof(Instances.Data)", unsafe.Offsetof(inst.Data), 24)

	var info AccelerationStructureBuildGeometryInfoKHR
	check("sizeof(AccelerationStructureBuildGeometryInfoKHR)", unsafe.Sizeof(info), 80)
	check("offsetof(BuildGeometryInfo.Type)", unsafe.Offsetof(info.Type), 16)
	check("offsetof(BuildGeometryInfo.Flags)", unsafe.Offsetof(info.Flags), 20)
	check("offsetof(BuildGeometryInfo.Mode)", unsafe.Offsetof(info.Mode), 24)
	check("offsetof(BuildGeometryInfo.SrcAccelerationStructure)", unsafe.Offsetof(info.SrcAccelerationStructure), 32)
	check("offsetof(BuildGeometryInfo.DstAccelerationStructure)", unsafe.Offsetof(info.DstAccelerationStructure), 40)
	check("offsetof(BuildGeometryInfo.GeometryCount)", unsafe.Offsetof(info.GeometryCount), 48)
	check("offsetof(BuildGeometryInfo.PGeometries)", unsafe.Offsetof(info.PGeometries), 56)
	check("offsetof(BuildGeometryInfo.PpGeometries)", unsafe.Offsetof(info.PpGeometries), 64)
	check("offsetof(BuildGeometryInfo.ScratchData)", unsafe.Offsetof(info.ScratchData), 72)

	var rng AccelerationStructureBuildRangeInfoKHR
	check("sizeof(AccelerationStructureBuildRangeInfoKHR)", unsafe.Sizeof(rng), 16)

	var create AccelerationStructureCreateInfoKHR
	check("sizeof(AccelerationStructureCreateInfoKHR)", unsafe.Sizeof(create), 64)
	check("offsetof(CreateInfo.Buffer)", unsafe.Offsetof(create.Buffer), 24)
	check("offsetof(CreateInfo.Type)", unsafe.Offsetof(create.Type), 48)
	check("offsetof(CreateInfo.DeviceAddress)", unsafe.Offsetof(create.DeviceAddress), 56)

	var sizes AccelerationStructureBuildSizesInfoKHR
	check("sizeof(AccelerationStructureBuildSizesInfoKHR)", unsafe.Sizeof(sizes), 40)

	var props PhysicalDeviceAccelerationStructurePropertiesKHR
	check("sizeof(PhysicalDeviceAccelerationStructurePropertiesKHR)", unsafe.Sizeof(props), 64)
	check("offsetof(ASProps.MinAccelerationStructureScratchOffsetAlignment)",
		unsafe.Offsetof(props.MinAccelerationStructureScratchOffsetAlignment), 56)

	var write WriteDescriptorSetAccelerationStructureKHR
	check("sizeof(WriteDescriptorSetAccelerationStructureKHR)", unsafe.Sizeof(write), 32)
	check("offsetof(WriteDescriptorSetAS.PAccelerationStructures)", unsafe.Offsetof(write.PAccelerationStructures), 24)

	var flagsInfo MemoryAllocateFlagsInfo
	check("sizeof(MemoryAllocateFlagsInfo)", unsafe.Sizeof(flagsInfo), 24)
}
