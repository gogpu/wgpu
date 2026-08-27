//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/memory"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// AccelerationStructure implements hal.AccelerationStructure for Vulkan.
// Mirrors Rust wgpu-hal AccelerationStructure (vulkan/mod.rs) which holds:
//   - raw: VkAccelerationStructureKHR handle
//   - buffer: backing VkBuffer (AS storage)
//   - allocation: GPU memory for the backing buffer
//   - compacted_size_query: optional VkQueryPool for compaction size readback
type AccelerationStructure struct {
	raw        vk.AccelerationStructureKHR
	buffer     vk.Buffer
	memory     *memory.MemoryBlock
	queryPool  vk.QueryPool // compaction size query (0 if compaction not allowed)
	deviceAddr uint64       // cached device address
}

// NativeHandle returns the VkAccelerationStructureKHR handle.
func (a *AccelerationStructure) NativeHandle() uintptr { return uintptr(a.raw) }

// Destroy is a no-op — destruction is managed by Device.DestroyAccelerationStructure.
func (a *AccelerationStructure) Destroy() {}

// Compile-time interface check.
var _ hal.AccelerationStructure = (*AccelerationStructure)(nil)

// --- Conversion helpers (Rust wgpu-hal conv.rs parity) ---

// mapAccelerationStructureFormat converts HAL format to VkAccelerationStructureTypeKHR.
// Reference: Rust conv::map_acceleration_structure_format (conv.rs:989-998).
func mapAccelerationStructureFormat(f hal.AccelerationStructureFormat) vk.AccelerationStructureTypeKHR {
	switch f {
	case hal.AccelerationStructureFormatTopLevel:
		return vk.AccelerationStructureTypeTopLevelKhr
	case hal.AccelerationStructureFormatBottomLevel:
		return vk.AccelerationStructureTypeBottomLevelKhr
	default:
		return vk.AccelerationStructureTypeTopLevelKhr
	}
}

// mapAccelerationStructureBuildMode converts HAL build mode to Vulkan enum.
// Reference: Rust conv::map_acceleration_structure_build_mode (conv.rs:1000-1011).
func mapAccelerationStructureBuildMode(m hal.AccelerationStructureBuildMode) vk.BuildAccelerationStructureModeKHR {
	switch m {
	case hal.AccelerationStructureBuildModeBuild:
		return vk.BuildAccelerationStructureModeBuildKhr
	case hal.AccelerationStructureBuildModeUpdate:
		return vk.BuildAccelerationStructureModeUpdateKhr
	default:
		return vk.BuildAccelerationStructureModeBuildKhr
	}
}

// mapAccelerationStructureFlags converts HAL flags to Vulkan build flags.
// Reference: Rust conv::map_acceleration_structure_flags (conv.rs:1013-1043).
func mapAccelerationStructureFlags(flags gputypes.AccelerationStructureFlags) vk.BuildAccelerationStructureFlagsKHR {
	var vkFlags vk.BuildAccelerationStructureFlagsKHR

	if flags.Contains(gputypes.ASFlagAllowUpdate) {
		vkFlags |= vk.BuildAccelerationStructureFlagsKHR(vk.BuildAccelerationStructureAllowUpdateBitKhr)
	}
	if flags.Contains(gputypes.ASFlagAllowCompaction) {
		vkFlags |= vk.BuildAccelerationStructureFlagsKHR(vk.BuildAccelerationStructureAllowCompactionBitKhr)
	}
	if flags.Contains(gputypes.ASFlagPreferFastTrace) {
		vkFlags |= vk.BuildAccelerationStructureFlagsKHR(vk.BuildAccelerationStructurePreferFastTraceBitKhr)
	}
	if flags.Contains(gputypes.ASFlagPreferFastBuild) {
		vkFlags |= vk.BuildAccelerationStructureFlagsKHR(vk.BuildAccelerationStructurePreferFastBuildBitKhr)
	}
	if flags.Contains(gputypes.ASFlagLowMemory) {
		vkFlags |= vk.BuildAccelerationStructureFlagsKHR(vk.BuildAccelerationStructureLowMemoryBitKhr)
	}
	if flags.Contains(gputypes.ASFlagAllowRayHitVertexReturn) {
		vkFlags |= vk.BuildAccelerationStructureFlagsKHR(vk.BuildAccelerationStructureAllowDataAccessBitKhr)
	}

	return vkFlags
}

// mapAccelerationStructureGeometryFlags converts HAL geometry flags to Vulkan.
// Reference: Rust conv::map_acceleration_structure_geometry_flags (conv.rs:1045-1059).
func mapAccelerationStructureGeometryFlags(flags gputypes.AccelerationStructureGeometryFlags) vk.GeometryFlagsKHR {
	var vkFlags vk.GeometryFlagsKHR

	if flags.Contains(gputypes.ASGeometryFlagOpaque) {
		vkFlags |= vk.GeometryFlagsKHR(vk.GeometryOpaqueBitKhr)
	}
	if flags.Contains(gputypes.ASGeometryFlagNoDuplicateAnyHitInvocation) {
		vkFlags |= vk.GeometryFlagsKHR(vk.GeometryNoDuplicateAnyHitInvocationBitKhr)
	}

	return vkFlags
}

// mapAccelerationStructureCopyMode converts HAL copy mode to Vulkan.
func mapAccelerationStructureCopyMode(mode gputypes.AccelerationStructureCopyMode) vk.CopyAccelerationStructureModeKHR {
	switch mode {
	case gputypes.AccelerationStructureCopyModeClone:
		return vk.CopyAccelerationStructureModeCloneKhr
	case gputypes.AccelerationStructureCopyModeCompact:
		return vk.CopyAccelerationStructureModeCompactKhr
	default:
		return vk.CopyAccelerationStructureModeCloneKhr
	}
}

// mapAccelerationStructureUsageToBarrier converts HAL AS usage to Vulkan pipeline stage
// and access flags for a memory barrier.
// Reference: Rust conv::map_acceleration_structure_usage_to_barrier (conv.rs:1061-1103).
// Simplified: we do not distinguish ray query vs RT pipeline features — we always emit
// the shader stage flags if SHADER_INPUT is set.
func mapAccelerationStructureUsageToBarrier(usage hal.AccelerationStructureUses) (vk.PipelineStageFlags, vk.AccessFlags) {
	var stages vk.PipelineStageFlags
	var access vk.AccessFlags

	if usage&hal.AccelerationStructureUsesBuildInput != 0 {
		stages |= vk.PipelineStageFlags(vk.PipelineStageAccelerationStructureBuildBitKhr)
		access |= vk.AccessFlags(vk.AccessAccelerationStructureReadBitKhr)
	}
	if usage&hal.AccelerationStructureUsesQueryInput != 0 {
		stages |= vk.PipelineStageFlags(vk.PipelineStageAccelerationStructureBuildBitKhr)
		access |= vk.AccessFlags(vk.AccessAccelerationStructureReadBitKhr)
	}
	if usage&hal.AccelerationStructureUsesBuildOutput != 0 {
		stages |= vk.PipelineStageFlags(vk.PipelineStageAccelerationStructureBuildBitKhr)
		access |= vk.AccessFlags(vk.AccessAccelerationStructureWriteBitKhr)
	}
	if usage&hal.AccelerationStructureUsesShaderInput != 0 {
		// Emit both compute/vertex/fragment stages and RT shader stage
		// so the barrier is correct regardless of which shader types read the AS.
		stages |= vk.PipelineStageFlags(vk.PipelineStageComputeShaderBit |
			vk.PipelineStageVertexShaderBit |
			vk.PipelineStageFragmentShaderBit)
		stages |= vk.PipelineStageFlags(vk.PipelineStageRayTracingShaderBitKhr)
		access |= vk.AccessFlags(vk.AccessAccelerationStructureReadBitKhr)
	}
	if usage&hal.AccelerationStructureUsesCopySrc != 0 {
		stages |= vk.PipelineStageFlags(vk.PipelineStageAccelerationStructureBuildBitKhr)
		access |= vk.AccessFlags(vk.AccessAccelerationStructureReadBitKhr)
	}
	if usage&hal.AccelerationStructureUsesCopyDst != 0 {
		stages |= vk.PipelineStageFlags(vk.PipelineStageAccelerationStructureBuildBitKhr)
		access |= vk.AccessFlags(vk.AccessAccelerationStructureWriteBitKhr)
	}

	return stages, access
}

// mapIndexFormatToVk converts HAL index format to Vulkan index type.
func mapIndexFormatToVk(f gputypes.IndexFormat) vk.IndexType {
	switch f {
	case gputypes.IndexFormatUint16:
		return vk.IndexTypeUint16
	case gputypes.IndexFormatUint32:
		return vk.IndexTypeUint32
	default:
		return vk.IndexTypeUint16
	}
}

// getBufferDeviceAddress returns the VkDeviceAddress for a HAL buffer.
// Panics if buffer is nil — caller must guarantee valid buffers.
func (d *Device) getBufferDeviceAddress(buffer hal.Buffer) uint64 {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		panic("vulkan: ray tracing requires valid buffer for device address")
	}

	info := vk.BufferDeviceAddressInfo{
		SType:  vk.StructureTypeBufferDeviceAddressInfo,
		Buffer: buf.handle,
	}
	return uint64(d.cmds.GetBufferDeviceAddress(d.handle, &info))
}

// --- Device methods ---

// CreateAccelerationStructure creates an acceleration structure (BLAS or TLAS).
// Allocates a VkBuffer as backing store, then creates the VkAccelerationStructureKHR on top.
// Reference: Rust wgpu-hal device.rs:2794-2907.
func (d *Device) CreateAccelerationStructure(desc *hal.AccelerationStructureDescriptor) (hal.AccelerationStructure, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: acceleration structure descriptor is nil")
	}

	// Step 1: Create backing VkBuffer with AS_STORAGE + SHADER_DEVICE_ADDRESS usage.
	bufferCreateInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        vk.DeviceSize(desc.Size),
		Usage:       vk.BufferUsageFlags(vk.BufferUsageAccelerationStructureStorageBitKhr) | vk.BufferUsageFlags(vk.BufferUsageShaderDeviceAddressBit),
		SharingMode: vk.SharingModeExclusive,
	}

	var rawBuffer vk.Buffer
	result := d.cmds.CreateBuffer(d.handle, &bufferCreateInfo, nil, &rawBuffer)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateBuffer for AS backing store failed: %d", result)
	}

	// Step 2: Get memory requirements and allocate GPU-only memory.
	var memReqs vk.MemoryRequirements
	d.cmds.GetBufferMemoryRequirements(d.handle, rawBuffer, &memReqs)

	memBlock, err := d.allocator.Alloc(memory.AllocationRequest{
		Size:           uint64(memReqs.Size),
		Alignment:      uint64(memReqs.Alignment),
		Usage:          memory.UsageFastDeviceAccess,
		MemoryTypeBits: memReqs.MemoryTypeBits,
	})
	if err != nil {
		d.cmds.DestroyBuffer(d.handle, rawBuffer, nil)
		return nil, fmt.Errorf("vulkan: failed to allocate AS backing memory: %w", err)
	}

	// Step 3: Bind memory.
	result = d.cmds.BindBufferMemory(d.handle, rawBuffer, memBlock.Memory, vk.DeviceSize(memBlock.Offset))
	if result != vk.Success {
		_ = d.allocator.Free(memBlock)
		d.cmds.DestroyBuffer(d.handle, rawBuffer, nil)
		return nil, fmt.Errorf("vulkan: vkBindBufferMemory for AS failed: %d", result)
	}

	if desc.Label != "" {
		d.setObjectName(vk.ObjectTypeBuffer, uint64(rawBuffer), desc.Label+" (AS backing)")
	}

	// Step 4: Create VkAccelerationStructureKHR.
	asCreateInfo := vk.AccelerationStructureCreateInfoKHR{
		SType:  vk.StructureTypeAccelerationStructureCreateInfoKhr,
		Buffer: rawBuffer,
		Offset: 0,
		Size:   vk.DeviceSize(desc.Size),
		Type:   mapAccelerationStructureFormat(desc.Format),
	}

	var rawAS vk.AccelerationStructureKHR
	result = d.cmds.CreateAccelerationStructureKHR(d.handle, &asCreateInfo, nil, &rawAS)
	if result != vk.Success {
		_ = d.allocator.Free(memBlock)
		d.cmds.DestroyBuffer(d.handle, rawBuffer, nil)
		return nil, fmt.Errorf("vulkan: vkCreateAccelerationStructureKHR failed: %d", result)
	}

	if desc.Label != "" {
		d.setObjectName(vk.ObjectTypeAccelerationStructureKHR, uint64(rawAS), desc.Label)
	}

	// Step 5: Optionally create a query pool for compaction size readback.
	var queryPool vk.QueryPool
	if desc.AllowCompaction {
		queryPoolInfo := vk.QueryPoolCreateInfo{
			SType:      vk.StructureTypeQueryPoolCreateInfo,
			QueryType:  vk.QueryTypeAccelerationStructureCompactedSizeKhr,
			QueryCount: 1,
		}

		result = d.cmds.CreateQueryPool(d.handle, &queryPoolInfo, nil, &queryPool)
		if result != vk.Success {
			d.cmds.DestroyAccelerationStructureKHR(d.handle, rawAS, nil)
			_ = d.allocator.Free(memBlock)
			d.cmds.DestroyBuffer(d.handle, rawBuffer, nil)
			return nil, fmt.Errorf("vulkan: vkCreateQueryPool for AS compaction failed: %d", result)
		}
	}

	return &AccelerationStructure{
		raw:       rawAS,
		buffer:    rawBuffer,
		memory:    memBlock,
		queryPool: queryPool,
	}, nil
}

// DestroyAccelerationStructure destroys an acceleration structure and its backing resources.
// Reference: Rust wgpu-hal device.rs:2909-2938.
func (d *Device) DestroyAccelerationStructure(as hal.AccelerationStructure) {
	vkAS, ok := as.(*AccelerationStructure)
	if !ok || vkAS == nil {
		return
	}

	if vkAS.queryPool != 0 {
		d.cmds.DestroyQueryPool(d.handle, vkAS.queryPool, nil)
	}

	d.cmds.DestroyAccelerationStructureKHR(d.handle, vkAS.raw, nil)
	d.cmds.DestroyBuffer(d.handle, vkAS.buffer, nil)

	if vkAS.memory != nil {
		if err := d.allocator.Free(vkAS.memory); err != nil {
			hal.Logger().Warn("vulkan: failed to free AS backing memory",
				"error", err)
		}
	}
}

// GetAccelerationStructureBuildSizes returns the sizes needed for building an AS.
// Reference: Rust wgpu-hal device.rs:2623-2771.
func (d *Device) GetAccelerationStructureBuildSizes(desc *hal.GetAccelerationStructureBuildSizesDescriptor) hal.AccelerationStructureBuildSizes {
	if desc == nil || desc.Entries == nil {
		return hal.AccelerationStructureBuildSizes{}
	}

	geometries, primitiveCounts, asType := d.buildGeometryInfosForSizeQuery(desc)

	geometryInfo := vk.AccelerationStructureBuildGeometryInfoKHR{
		SType:         vk.StructureTypeAccelerationStructureBuildGeometryInfoKhr,
		Type:          asType,
		Flags:         mapAccelerationStructureFlags(desc.Flags),
		GeometryCount: uint32(len(geometries)),
	}
	if len(geometries) > 0 {
		geometryInfo.PGeometries = &geometries[0]
	}

	sizeInfo := vk.AccelerationStructureBuildSizesInfoKHR{
		SType: vk.StructureTypeAccelerationStructureBuildSizesInfoKhr,
	}

	if len(primitiveCounts) > 0 {
		d.cmds.GetAccelerationStructureBuildSizesKHR(
			d.handle,
			vk.AccelerationStructureBuildTypeDeviceKhr,
			&geometryInfo,
			&primitiveCounts[0],
			&sizeInfo,
		)
	}

	return hal.AccelerationStructureBuildSizes{
		AccelerationStructureSize: uint64(sizeInfo.AccelerationStructureSize),
		UpdateScratchSize:         uint64(sizeInfo.UpdateScratchSize),
		BuildScratchSize:          uint64(sizeInfo.BuildScratchSize),
	}
}

// buildGeometryInfosForSizeQuery builds the VkAccelerationStructureGeometryKHR array and
// primitive counts for a build size query. Addresses are zeroed because the spec
// says they are ignored by vkGetAccelerationStructureBuildSizesKHR (except
// transformData which is checked for NULL to determine if transforms are used).
func (d *Device) buildGeometryInfosForSizeQuery(
	desc *hal.GetAccelerationStructureBuildSizesDescriptor,
) ([]vk.AccelerationStructureGeometryKHR, []uint32, vk.AccelerationStructureTypeKHR) {
	entries := desc.Entries

	if entries.Instances != nil {
		instanceData := vk.AccelerationStructureGeometryInstancesDataKHR{
			SType: vk.StructureTypeAccelerationStructureGeometryInstancesDataKhr,
		}

		var geomData vk.AccelerationStructureGeometryDataKHR
		// Copy the instances data into the union's 64-byte storage.
		*(*vk.AccelerationStructureGeometryInstancesDataKHR)(unsafe.Pointer(&geomData)) = instanceData

		geometry := vk.AccelerationStructureGeometryKHR{
			SType:        vk.StructureTypeAccelerationStructureGeometryKhr,
			GeometryType: vk.GeometryTypeInstancesKhr,
			Geometry:     geomData,
		}

		return []vk.AccelerationStructureGeometryKHR{geometry},
			[]uint32{entries.Instances.Count},
			vk.AccelerationStructureTypeTopLevelKhr
	}

	if len(entries.Triangles) > 0 {
		geometries := make([]vk.AccelerationStructureGeometryKHR, 0, len(entries.Triangles))
		primCounts := make([]uint32, 0, len(entries.Triangles))

		for _, tri := range entries.Triangles {
			triangleData := vk.AccelerationStructureGeometryTrianglesDataKHR{
				SType:        vk.StructureTypeAccelerationStructureGeometryTrianglesDataKhr,
				VertexFormat: vertexFormatToVk(tri.VertexFormat),
				MaxVertex:    tri.VertexCount,
				VertexStride: vk.DeviceSize(tri.VertexStride),
				IndexType:    vk.IndexTypeNoneKhr,
			}

			// If USE_TRANSFORM flag is set AND a transform buffer is provided,
			// set a non-zero address so the spec knows transforms are used for sizing.
			if desc.Flags.Contains(gputypes.ASFlagUseTransform) && tri.Transform != nil {
				triangleData.TransformData = vk.DeviceOrHostAddressConstKHR(
					d.getBufferDeviceAddress(tri.Transform.Buffer))
			}

			primCount := tri.VertexCount / 3
			if tri.Indices != nil {
				triangleData.IndexType = mapIndexFormatToVk(tri.Indices.Format)
				primCount = tri.Indices.Count / 3
			}

			var geomData vk.AccelerationStructureGeometryDataKHR
			*(*vk.AccelerationStructureGeometryTrianglesDataKHR)(unsafe.Pointer(&geomData)) = triangleData

			geometry := vk.AccelerationStructureGeometryKHR{
				SType:        vk.StructureTypeAccelerationStructureGeometryKhr,
				GeometryType: vk.GeometryTypeTrianglesKhr,
				Geometry:     geomData,
				Flags:        mapAccelerationStructureGeometryFlags(tri.Flags),
			}

			geometries = append(geometries, geometry)
			primCounts = append(primCounts, primCount)
		}

		return geometries, primCounts, vk.AccelerationStructureTypeBottomLevelKhr
	}

	if len(entries.AABBs) > 0 {
		geometries := make([]vk.AccelerationStructureGeometryKHR, 0, len(entries.AABBs))
		primCounts := make([]uint32, 0, len(entries.AABBs))

		for _, aabb := range entries.AABBs {
			aabbData := vk.AccelerationStructureGeometryAabbsDataKHR{
				SType:  vk.StructureTypeAccelerationStructureGeometryAabbsDataKhr,
				Stride: vk.DeviceSize(aabb.Stride),
			}

			var geomData vk.AccelerationStructureGeometryDataKHR
			*(*vk.AccelerationStructureGeometryAabbsDataKHR)(unsafe.Pointer(&geomData)) = aabbData

			geometry := vk.AccelerationStructureGeometryKHR{
				SType:        vk.StructureTypeAccelerationStructureGeometryKhr,
				GeometryType: vk.GeometryTypeAabbsKhr,
				Geometry:     geomData,
				Flags:        mapAccelerationStructureGeometryFlags(aabb.Flags),
			}

			geometries = append(geometries, geometry)
			primCounts = append(primCounts, aabb.Count)
		}

		return geometries, primCounts, vk.AccelerationStructureTypeBottomLevelKhr
	}

	return nil, nil, vk.AccelerationStructureTypeTopLevelKhr
}

// GetAccelerationStructureDeviceAddress returns the GPU device address of an AS.
// Reference: Rust wgpu-hal device.rs:2773-2792.
func (d *Device) GetAccelerationStructureDeviceAddress(as hal.AccelerationStructure) uint64 {
	vkAS, ok := as.(*AccelerationStructure)
	if !ok || vkAS == nil {
		return 0
	}

	// Return cached address if available.
	if vkAS.deviceAddr != 0 {
		return vkAS.deviceAddr
	}

	info := vk.AccelerationStructureDeviceAddressInfoKHR{
		SType:                 vk.StructureTypeAccelerationStructureDeviceAddressInfoKhr,
		AccelerationStructure: vkAS.raw,
	}

	addr := uint64(d.cmds.GetAccelerationStructureDeviceAddressKHR(d.handle, &info))
	vkAS.deviceAddr = addr
	return addr
}

// TlasInstanceToBytes converts a TlasInstance to the 64-byte packed format
// VkAccelerationStructureInstanceKHR.
//
// Layout (from Vulkan spec 1.3.290, Table 33):
//
//	Bytes  0-47: VkTransformMatrixKHR — 3x4 row-major float32 (12 floats)
//	Bytes 48-51: instanceCustomIndex (24 bits) | mask (8 bits)
//	Bytes 52-55: instanceShaderBindingTableRecordOffset (24 bits) | flags (8 bits)
//	Bytes 56-63: accelerationStructureReference (uint64, device address)
//
// Note: TlasInstance.Transform is already [12]float32 in row-major order matching
// VkTransformMatrixKHR, so no transposition is needed.
// Reference: Rust wgpu-hal device.rs:2981-2993.
func (d *Device) TlasInstanceToBytes(instance hal.TlasInstance) []byte {
	const maxU24 = (1 << 24) - 1

	buf := make([]byte, 64)

	// Bytes 0-47: 12 float32 values (3x4 transform matrix).
	for i, v := range instance.Transform {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}

	// Bytes 48-51: instanceCustomIndex (lower 24 bits) | mask (upper 8 bits).
	customAndMask := (instance.CustomData & maxU24) | (uint32(instance.Mask) << 24)
	binary.LittleEndian.PutUint32(buf[48:], customAndMask)

	// Bytes 52-55: instanceShaderBindingTableRecordOffset (lower 24 bits) | flags (upper 8 bits).
	// Flags are zero in the base WebGPU RT API — no VkGeometryInstanceFlagsKHR exposed.
	sbtAndFlags := instance.ShaderBindingTableRecordOffset & maxU24
	binary.LittleEndian.PutUint32(buf[52:], sbtAndFlags)

	// Bytes 56-63: accelerationStructureReference (BLAS device address).
	binary.LittleEndian.PutUint64(buf[56:], instance.BlasAddress)

	return buf
}

// --- CommandEncoder methods ---

// BuildAccelerationStructures builds one or more acceleration structures.
// Batched to match vkCmdBuildAccelerationStructuresKHR.
// Reference: Rust wgpu-hal command.rs:545-764.
func (e *CommandEncoder) BuildAccelerationStructures(descriptors []hal.BuildAccelerationStructureDescriptor) {
	if e.active == 0 || len(descriptors) == 0 {
		return
	}

	descriptorCount := len(descriptors)

	// Pre-allocate storage for the Vulkan structs.
	// Each descriptor produces one geometry info + one ranges slice.
	geometryInfos := make([]vk.AccelerationStructureBuildGeometryInfoKHR, 0, descriptorCount)
	geometriesStorage := make([][]vk.AccelerationStructureGeometryKHR, 0, descriptorCount)
	rangesStorage := make([][]vk.AccelerationStructureBuildRangeInfoKHR, 0, descriptorCount)

	for _, desc := range descriptors {
		if desc.Entries == nil {
			continue
		}

		geometries, ranges := e.buildGeometryInfosForBuild(&desc)

		geometriesStorage = append(geometriesStorage, geometries)
		rangesStorage = append(rangesStorage, ranges)

		dstAS, _ := desc.DestinationAccelerationStructure.(*AccelerationStructure)
		if dstAS == nil {
			continue
		}

		scratchAddr := e.device.getBufferDeviceAddress(desc.ScratchBuffer)

		asType := vk.AccelerationStructureTypeBottomLevelKhr
		if desc.Entries.Instances != nil {
			asType = vk.AccelerationStructureTypeTopLevelKhr
		}

		geometryInfo := vk.AccelerationStructureBuildGeometryInfoKHR{
			SType:                    vk.StructureTypeAccelerationStructureBuildGeometryInfoKhr,
			Type:                     asType,
			Flags:                    mapAccelerationStructureFlags(desc.Flags),
			Mode:                     mapAccelerationStructureBuildMode(desc.Mode),
			DstAccelerationStructure: dstAS.raw,
			ScratchData:              vk.DeviceOrHostAddressKHR(scratchAddr + desc.ScratchBufferOffset),
		}

		// For update mode, set the source AS.
		if desc.Mode == hal.AccelerationStructureBuildModeUpdate {
			srcAS, _ := desc.SourceAccelerationStructure.(*AccelerationStructure)
			if srcAS != nil {
				geometryInfo.SrcAccelerationStructure = srcAS.raw
			} else {
				// Self-update: use destination as source (Rust wgpu-hal pattern).
				geometryInfo.SrcAccelerationStructure = dstAS.raw
			}
		}

		geometryInfos = append(geometryInfos, geometryInfo)
	}

	if len(geometryInfos) == 0 {
		return
	}

	// Patch geometry pointers into the geometry infos (must be done after all
	// slices are fully built, because append may reallocate).
	for i := range geometryInfos {
		geometryInfos[i].GeometryCount = uint32(len(geometriesStorage[i]))
		if len(geometriesStorage[i]) > 0 {
			geometryInfos[i].PGeometries = &geometriesStorage[i][0]
		}
	}

	// Build the pointer-to-pointer array for ranges.
	rangePtrs := make([]*vk.AccelerationStructureBuildRangeInfoKHR, len(geometryInfos))
	for i := range rangesStorage {
		if len(rangesStorage[i]) > 0 {
			rangePtrs[i] = &rangesStorage[i][0]
		}
	}

	e.device.cmds.CmdBuildAccelerationStructuresKHR(
		e.active,
		uint32(len(geometryInfos)),
		&geometryInfos[0],
		&rangePtrs[0],
	)
}

// buildGeometryInfosForBuild converts a single BuildAccelerationStructureDescriptor's
// entries into Vulkan geometry and range info arrays for a build operation.
func (e *CommandEncoder) buildGeometryInfosForBuild(
	desc *hal.BuildAccelerationStructureDescriptor,
) ([]vk.AccelerationStructureGeometryKHR, []vk.AccelerationStructureBuildRangeInfoKHR) {
	entries := desc.Entries

	if entries.Instances != nil {
		instanceAddr := e.device.getBufferDeviceAddress(entries.Instances.Buffer)

		instanceData := vk.AccelerationStructureGeometryInstancesDataKHR{
			SType: vk.StructureTypeAccelerationStructureGeometryInstancesDataKhr,
			Data:  vk.DeviceOrHostAddressConstKHR(instanceAddr),
		}

		var geomData vk.AccelerationStructureGeometryDataKHR
		*(*vk.AccelerationStructureGeometryInstancesDataKHR)(unsafe.Pointer(&geomData)) = instanceData

		geometry := vk.AccelerationStructureGeometryKHR{
			SType:        vk.StructureTypeAccelerationStructureGeometryKhr,
			GeometryType: vk.GeometryTypeInstancesKhr,
			Geometry:     geomData,
		}

		rangeInfo := vk.AccelerationStructureBuildRangeInfoKHR{
			PrimitiveCount:  entries.Instances.Count,
			PrimitiveOffset: entries.Instances.Offset,
		}

		return []vk.AccelerationStructureGeometryKHR{geometry},
			[]vk.AccelerationStructureBuildRangeInfoKHR{rangeInfo}
	}

	if len(entries.Triangles) > 0 {
		geometries := make([]vk.AccelerationStructureGeometryKHR, 0, len(entries.Triangles))
		ranges := make([]vk.AccelerationStructureBuildRangeInfoKHR, 0, len(entries.Triangles))

		for _, tri := range entries.Triangles {
			vertexAddr := e.device.getBufferDeviceAddress(tri.VertexBuffer) +
				uint64(tri.FirstVertex)*tri.VertexStride

			triangleData := vk.AccelerationStructureGeometryTrianglesDataKHR{
				SType:        vk.StructureTypeAccelerationStructureGeometryTrianglesDataKhr,
				VertexFormat: vertexFormatToVk(tri.VertexFormat),
				VertexData:   vk.DeviceOrHostAddressConstKHR(vertexAddr),
				VertexStride: vk.DeviceSize(tri.VertexStride),
				MaxVertex:    tri.VertexCount,
				IndexType:    vk.IndexTypeNoneKhr,
			}

			rangeInfo := vk.AccelerationStructureBuildRangeInfoKHR{}

			if tri.Indices != nil {
				indexAddr := e.device.getBufferDeviceAddress(tri.Indices.Buffer)
				triangleData.IndexType = mapIndexFormatToVk(tri.Indices.Format)
				triangleData.IndexData = vk.DeviceOrHostAddressConstKHR(indexAddr)
				rangeInfo.PrimitiveCount = tri.Indices.Count / 3
				rangeInfo.PrimitiveOffset = tri.Indices.Offset
			} else {
				rangeInfo.PrimitiveCount = tri.VertexCount / 3
			}

			if tri.Transform != nil {
				transformAddr := e.device.getBufferDeviceAddress(tri.Transform.Buffer)
				triangleData.TransformData = vk.DeviceOrHostAddressConstKHR(transformAddr)
				rangeInfo.TransformOffset = tri.Transform.Offset
			}

			var geomData vk.AccelerationStructureGeometryDataKHR
			*(*vk.AccelerationStructureGeometryTrianglesDataKHR)(unsafe.Pointer(&geomData)) = triangleData

			geometry := vk.AccelerationStructureGeometryKHR{
				SType:        vk.StructureTypeAccelerationStructureGeometryKhr,
				GeometryType: vk.GeometryTypeTrianglesKhr,
				Geometry:     geomData,
				Flags:        mapAccelerationStructureGeometryFlags(tri.Flags),
			}

			geometries = append(geometries, geometry)
			ranges = append(ranges, rangeInfo)
		}

		return geometries, ranges
	}

	if len(entries.AABBs) > 0 {
		geometries := make([]vk.AccelerationStructureGeometryKHR, 0, len(entries.AABBs))
		ranges := make([]vk.AccelerationStructureBuildRangeInfoKHR, 0, len(entries.AABBs))

		for _, aabb := range entries.AABBs {
			aabbAddr := e.device.getBufferDeviceAddress(aabb.Buffer)

			aabbData := vk.AccelerationStructureGeometryAabbsDataKHR{
				SType:  vk.StructureTypeAccelerationStructureGeometryAabbsDataKhr,
				Data:   vk.DeviceOrHostAddressConstKHR(aabbAddr),
				Stride: vk.DeviceSize(aabb.Stride),
			}

			var geomData vk.AccelerationStructureGeometryDataKHR
			*(*vk.AccelerationStructureGeometryAabbsDataKHR)(unsafe.Pointer(&geomData)) = aabbData

			geometry := vk.AccelerationStructureGeometryKHR{
				SType:        vk.StructureTypeAccelerationStructureGeometryKhr,
				GeometryType: vk.GeometryTypeAabbsKhr,
				Geometry:     geomData,
				Flags:        mapAccelerationStructureGeometryFlags(aabb.Flags),
			}

			rangeInfo := vk.AccelerationStructureBuildRangeInfoKHR{
				PrimitiveCount:  aabb.Count,
				PrimitiveOffset: aabb.Offset,
			}

			geometries = append(geometries, geometry)
			ranges = append(ranges, rangeInfo)
		}

		return geometries, ranges
	}

	return nil, nil
}

// PlaceAccelerationStructureBarrier inserts an AS memory barrier.
// Uses vkCmdPipelineBarrier with VkMemoryBarrier (not buffer/image-specific)
// because AS barriers are global memory barriers.
// Reference: Rust wgpu-hal command.rs:766-794.
func (e *CommandEncoder) PlaceAccelerationStructureBarrier(barrier hal.AccelerationStructureBarrier) {
	if e.active == 0 {
		return
	}

	srcStage, srcAccess := mapAccelerationStructureUsageToBarrier(barrier.Usage.OldUsage)
	dstStage, dstAccess := mapAccelerationStructureUsageToBarrier(barrier.Usage.NewUsage)

	// Rust wgpu-hal ORs TOP_OF_PIPE/BOTTOM_OF_PIPE to ensure non-empty stage masks
	// (Vulkan spec requires at least one stage bit set in barrier).
	srcStage |= vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
	dstStage |= vk.PipelineStageFlags(vk.PipelineStageBottomOfPipeBit)

	memBarrier := vk.MemoryBarrier{
		SType:         vk.StructureTypeMemoryBarrier,
		SrcAccessMask: srcAccess,
		DstAccessMask: dstAccess,
	}

	vkCmdPipelineBarrier(
		e.device.cmds,
		e.active,
		srcStage,
		dstStage,
		0,              // dependencyFlags
		1, &memBarrier, // memory barriers
		0, nil, // buffer barriers
		0, nil, // image barriers
	)
}

// CopyAccelerationStructure copies or compacts an acceleration structure.
// Reference: Rust wgpu-hal uses vkCmdCopyAccelerationStructureKHR.
func (e *CommandEncoder) CopyAccelerationStructure(src, dst hal.AccelerationStructure, copyMode gputypes.AccelerationStructureCopyMode) {
	if e.active == 0 {
		return
	}

	srcAS, ok := src.(*AccelerationStructure)
	if !ok || srcAS == nil {
		return
	}
	dstAS, ok := dst.(*AccelerationStructure)
	if !ok || dstAS == nil {
		return
	}

	copyInfo := vk.CopyAccelerationStructureInfoKHR{
		SType: vk.StructureTypeCopyAccelerationStructureInfoKhr,
		Src:   srcAS.raw,
		Dst:   dstAS.raw,
		Mode:  mapAccelerationStructureCopyMode(copyMode),
	}

	e.device.cmds.CmdCopyAccelerationStructureKHR(e.active, &copyInfo)
}

// ReadAccelerationStructureCompactSize reads the post-compact size of an AS
// into the given buffer. This resets the query pool, writes the compacted size
// property, then copies the result to the destination buffer.
// Reference: Rust wgpu-hal command.rs:473-512.
func (e *CommandEncoder) ReadAccelerationStructureCompactSize(as hal.AccelerationStructure, buffer hal.Buffer, offset uint64) {
	if e.active == 0 {
		return
	}

	vkAS, ok := as.(*AccelerationStructure)
	if !ok || vkAS == nil || vkAS.queryPool == 0 {
		return
	}
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}

	// Step 1: Reset the query pool.
	e.device.cmds.CmdResetQueryPool(e.active, vkAS.queryPool, 0, 1)

	// Step 2: Write the compacted size property.
	asHandle := vkAS.raw
	e.device.cmds.CmdWriteAccelerationStructuresPropertiesKHR(
		e.active,
		1,
		&asHandle,
		vk.QueryTypeAccelerationStructureCompactedSizeKhr,
		vkAS.queryPool,
		0,
	)

	// Step 3: Copy query result to the buffer.
	// 8 bytes per result (uint64), WAIT flag ensures result is available.
	vkCmdCopyQueryPoolResults(
		e.device.cmds,
		e.active,
		vkAS.queryPool,
		0, // firstQuery
		1, // queryCount
		buf.handle,
		offset,
		8, // stride: sizeof(uint64)
		vk.QueryResultFlags(vk.QueryResult64Bit|vk.QueryResultWaitBit),
	)
}
