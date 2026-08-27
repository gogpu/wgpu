// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

// DXR ray tracing implementation for the DX12 backend.
//
// Follows Rust wgpu-hal dx12/device.rs and dx12/command.rs:
// - create_acceleration_structure → CreateCommittedResource with D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS
// - get_acceleration_structure_build_sizes → ID3D12Device5::GetRaytracingAccelerationStructurePrebuildInfo
// - build_acceleration_structures → ID3D12GraphicsCommandList4::BuildRaytracingAccelerationStructure
// - place_acceleration_structure_barrier → UAV barrier (global, no resource pointer)
// - copy_acceleration_structure → ID3D12GraphicsCommandList4::CopyRaytracingAccelerationStructure
// - read_acceleration_structure_compact_size → EmitRaytracingAccelerationStructurePostbuildInfo

package dx12

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// ---------------------------------------------------------------------------
// AccelerationStructure resource
// ---------------------------------------------------------------------------

// AccelerationStructure implements hal.AccelerationStructure for DirectX 12.
// The underlying resource is a committed buffer with D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS
// in D3D12_RESOURCE_STATE_RAYTRACING_ACCELERATION_STRUCTURE, matching Rust wgpu-hal.
type AccelerationStructure struct {
	resource *d3d12.ID3D12Resource
}

// NativeHandle returns the underlying ID3D12Resource pointer.
func (a *AccelerationStructure) NativeHandle() uintptr {
	return uintptr(unsafe.Pointer(a.resource))
}

// Destroy releases the committed resource.
func (a *AccelerationStructure) Destroy() {
	if a.resource != nil {
		a.resource.Release()
		a.resource = nil
	}
}

// Compile-time interface assertion.
var _ hal.AccelerationStructure = (*AccelerationStructure)(nil)

// ---------------------------------------------------------------------------
// Device — AccelerationStructure creation and query methods
// ---------------------------------------------------------------------------

// CreateAccelerationStructure creates a committed resource for an acceleration
// structure. Matches Rust wgpu-hal create_acceleration_structure: buffer resource
// with D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS in
// D3D12_RESOURCE_STATE_RAYTRACING_ACCELERATION_STRUCTURE.
func (d *Device) CreateAccelerationStructure(desc *hal.AccelerationStructureDescriptor) (hal.AccelerationStructure, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: acceleration structure descriptor is nil")
	}

	heapProps := d3d12.D3D12_HEAP_PROPERTIES{
		Type:                 d3d12.D3D12_HEAP_TYPE_DEFAULT,
		CPUPageProperty:      d3d12.D3D12_CPU_PAGE_PROPERTY_UNKNOWN,
		MemoryPoolPreference: d3d12.D3D12_MEMORY_POOL_UNKNOWN,
		CreationNodeMask:     0,
		VisibleNodeMask:      0,
	}

	resourceDesc := d3d12.D3D12_RESOURCE_DESC{
		Dimension:        d3d12.D3D12_RESOURCE_DIMENSION_BUFFER,
		Alignment:        0,
		Width:            desc.Size,
		Height:           1,
		DepthOrArraySize: 1,
		MipLevels:        1,
		Format:           d3d12.DXGI_FORMAT_UNKNOWN,
		SampleDesc:       d3d12.DXGI_SAMPLE_DESC{Count: 1, Quality: 0},
		Layout:           d3d12.D3D12_TEXTURE_LAYOUT_ROW_MAJOR,
		// Rust wgpu-hal: D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS
		// TODO: when enhanced barriers are available, use D3D12_RESOURCE_FLAG_RAYTRACING_ACCELERATION_STRUCTURE
		Flags: d3d12.D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS,
	}

	resource, err := d.raw.CreateCommittedResource(
		&heapProps,
		d3d12.D3D12_HEAP_FLAG_NONE,
		&resourceDesc,
		d3d12.D3D12_RESOURCE_STATE_RAYTRACING_ACCELERATION_STRUCTURE,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateCommittedResource for acceleration structure failed: %w", err)
	}

	return &AccelerationStructure{resource: resource}, nil
}

// DestroyAccelerationStructure releases the committed resource backing the AS.
func (d *Device) DestroyAccelerationStructure(as hal.AccelerationStructure) {
	if as == nil {
		return
	}
	dxAS, ok := as.(*AccelerationStructure)
	if !ok || dxAS == nil {
		return
	}
	dxAS.Destroy()
}

// GetAccelerationStructureBuildSizes queries the driver for AS build size
// requirements. Casts the device to ID3D12Device5 and calls
// GetRaytracingAccelerationStructurePrebuildInfo.
func (d *Device) GetAccelerationStructureBuildSizes(desc *hal.GetAccelerationStructureBuildSizesDescriptor) hal.AccelerationStructureBuildSizes {
	if desc == nil || desc.Entries == nil {
		return hal.AccelerationStructureBuildSizes{}
	}

	dev5 := d.raw.QueryDevice5()
	if dev5 == nil {
		// DXR not supported on this device; return zero sizes.
		return hal.AccelerationStructureBuildSizes{}
	}
	defer dev5.Release()

	bi := buildInputsFromEntries(desc.Entries, desc.Flags, nil)

	var info d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_PREBUILD_INFO
	dev5.GetRaytracingAccelerationStructurePrebuildInfo(&bi.Inputs, &info)

	// The geometry backing array is referenced via raw pointer inside bi.Inputs;
	// keep the whole struct alive until the driver finishes reading it.
	runtime.KeepAlive(bi)

	return hal.AccelerationStructureBuildSizes{
		AccelerationStructureSize: info.ResultDataMaxSizeInBytes,
		UpdateScratchSize:         info.UpdateScratchDataSizeInBytes,
		BuildScratchSize:          info.ScratchDataSizeInBytes,
	}
}

// GetAccelerationStructureDeviceAddress returns the GPU virtual address of the
// AS resource. Matches Rust wgpu-hal: resource.GetGPUVirtualAddress().
func (d *Device) GetAccelerationStructureDeviceAddress(as hal.AccelerationStructure) uint64 {
	if as == nil {
		return 0
	}
	dxAS, ok := as.(*AccelerationStructure)
	if !ok || dxAS == nil || dxAS.resource == nil {
		return 0
	}
	return dxAS.resource.GetGPUVirtualAddress()
}

// TlasInstanceToBytes packs a TlasInstance into the 64-byte
// D3D12_RAYTRACING_INSTANCE_DESC layout. Matches Rust wgpu-hal tlas_instance_to_bytes.
//
// Layout:
//
//	Bytes  0-47: Transform [3][4]float32 (row-major)
//	Bytes 48-50: InstanceID (24 bits)
//	Byte  51:    InstanceMask (8 bits)
//	Bytes 52-54: InstanceContributionToHitGroupIndex (24 bits)
//	Byte  55:    Flags (8 bits, always 0 for now)
//	Bytes 56-63: AccelerationStructure (GPU virtual address, uint64)
func (d *Device) TlasInstanceToBytes(instance hal.TlasInstance) []byte {
	const maxU24 = (1 << 24) - 1

	buf := make([]byte, 64)

	// Transform: 12 x float32 = 48 bytes (row-major 3x4 affine)
	for i, v := range instance.Transform {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}

	// InstanceID (bits 0-23) | Mask (bits 24-31)
	bitfield1 := (instance.CustomData & maxU24) | (uint32(instance.Mask) << 24)
	binary.LittleEndian.PutUint32(buf[48:], bitfield1)

	// InstanceContributionToHitGroupIndex (bits 0-23) | Flags (bits 24-31, currently 0)
	bitfield2 := instance.ShaderBindingTableRecordOffset & maxU24
	binary.LittleEndian.PutUint32(buf[52:], bitfield2)

	// AccelerationStructure (GPU virtual address)
	binary.LittleEndian.PutUint64(buf[56:], instance.BlasAddress)

	return buf
}

// ---------------------------------------------------------------------------
// CommandEncoder — AccelerationStructure build/barrier/copy methods
// ---------------------------------------------------------------------------

// BuildAccelerationStructures builds one or more acceleration structures.
// Casts the command list to ID3D12GraphicsCommandList4 and calls
// BuildRaytracingAccelerationStructure per entry, matching Rust wgpu-hal.
func (e *CommandEncoder) BuildAccelerationStructures(descriptors []hal.BuildAccelerationStructureDescriptor) {
	if len(descriptors) == 0 || !e.isRecording || e.cmdList == nil {
		return
	}

	list4 := e.cmdList.QueryCommandList4()
	if list4 == nil {
		return
	}
	defer list4.Release()

	for i := range descriptors {
		desc := &descriptors[i]
		if desc.Entries == nil || desc.DestinationAccelerationStructure == nil || desc.ScratchBuffer == nil {
			continue
		}

		dstAS, ok := desc.DestinationAccelerationStructure.(*AccelerationStructure)
		if !ok || dstAS == nil || dstAS.resource == nil {
			continue
		}

		scratchBuf, ok := desc.ScratchBuffer.(*Buffer)
		if !ok || scratchBuf == nil || scratchBuf.raw == nil {
			continue
		}

		var buildMode *hal.AccelerationStructureBuildMode
		if desc.Mode == hal.AccelerationStructureBuildModeUpdate {
			m := hal.AccelerationStructureBuildModeUpdate
			buildMode = &m
		}

		bi := buildInputsFromEntries(desc.Entries, desc.Flags, buildMode)

		dstAddr := dstAS.resource.GetGPUVirtualAddress()
		scratchAddr := scratchBuf.gpuVA + desc.ScratchBufferOffset

		var srcAddr uint64
		if desc.SourceAccelerationStructure != nil {
			if srcAS, ok := desc.SourceAccelerationStructure.(*AccelerationStructure); ok && srcAS != nil && srcAS.resource != nil {
				srcAddr = srcAS.resource.GetGPUVirtualAddress()
			}
		}

		buildDesc := d3d12.D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_DESC{
			DestAccelerationStructureData:    dstAddr,
			Inputs:                           bi.Inputs,
			SourceAccelerationStructureData:  srcAddr,
			ScratchAccelerationStructureData: scratchAddr,
		}

		list4.BuildRaytracingAccelerationStructure(&buildDesc, 0, nil)

		// The geometry backing array is referenced via raw pointer inside
		// bi.Inputs; keep the struct alive until the driver reads it.
		runtime.KeepAlive(bi)
	}
}

// PlaceAccelerationStructureBarrier inserts a global UAV barrier for acceleration
// structures. Matches Rust wgpu-hal: UAV barrier with nil resource (global).
func (e *CommandEncoder) PlaceAccelerationStructureBarrier(_ hal.AccelerationStructureBarrier) {
	if !e.isRecording || e.cmdList == nil {
		return
	}

	// Global UAV barrier (nil resource) — ensures all previous AS writes
	// complete before subsequent reads. Rust wgpu-hal uses the same pattern:
	// D3D12_RESOURCE_BARRIER_TYPE_UAV with pResource = NULL.
	barrier := d3d12.NewUAVBarrier(nil)
	e.cmdList.ResourceBarrier(1, &barrier)
}

// CopyAccelerationStructure copies or compacts an acceleration structure.
// Casts the command list to ID3D12GraphicsCommandList4 and calls
// CopyRaytracingAccelerationStructure, matching Rust wgpu-hal.
func (e *CommandEncoder) CopyAccelerationStructure(src, dst hal.AccelerationStructure, copyMode gputypes.AccelerationStructureCopyMode) {
	if !e.isRecording || e.cmdList == nil {
		return
	}
	if src == nil || dst == nil {
		return
	}

	srcAS, ok := src.(*AccelerationStructure)
	if !ok || srcAS == nil || srcAS.resource == nil {
		return
	}
	dstAS, ok := dst.(*AccelerationStructure)
	if !ok || dstAS == nil || dstAS.resource == nil {
		return
	}

	list4 := e.cmdList.QueryCommandList4()
	if list4 == nil {
		return
	}
	defer list4.Release()

	dxMode := mapAccelerationStructureCopyMode(copyMode)
	list4.CopyRaytracingAccelerationStructure(
		dstAS.resource.GetGPUVirtualAddress(),
		srcAS.resource.GetGPUVirtualAddress(),
		dxMode,
	)
}

// ReadAccelerationStructureCompactSize emits a post-build info query for
// compacted size into the specified buffer. Matches Rust wgpu-hal
// read_acceleration_structure_compact_size using
// EmitRaytracingAccelerationStructurePostbuildInfo with COMPACTED_SIZE.
func (e *CommandEncoder) ReadAccelerationStructureCompactSize(as hal.AccelerationStructure, buffer hal.Buffer, offset uint64) {
	if !e.isRecording || e.cmdList == nil {
		return
	}
	if as == nil || buffer == nil {
		return
	}

	dxAS, ok := as.(*AccelerationStructure)
	if !ok || dxAS == nil || dxAS.resource == nil {
		return
	}
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || buf.raw == nil {
		return
	}

	list4 := e.cmdList.QueryCommandList4()
	if list4 == nil {
		return
	}
	defer list4.Release()

	postbuildDesc := d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_DESC{
		DestBuffer: buf.gpuVA + offset,
		InfoType:   d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_COMPACTED_SIZE,
	}

	srcAddr := dxAS.resource.GetGPUVirtualAddress()
	list4.EmitRaytracingAccelerationStructurePostbuildInfo(&postbuildDesc, 1, &srcAddr)
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

// asBuildInputs bundles the D3D12 build inputs with the backing geometry
// descriptor slice. The slice must remain alive while the driver reads
// the raw pointer stored in Inputs, so callers must pass *asBuildInputs
// to runtime.KeepAlive after the COM call.
type asBuildInputs struct {
	Inputs          d3d12.D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS
	geometryBacking []d3d12.D3D12_RAYTRACING_GEOMETRY_DESC // GC anchor: Inputs.Union holds raw pointer to this slice
}

// buildInputsFromEntries constructs the D3D12 build inputs from HAL entries.
// The caller must call runtime.KeepAlive on the returned *asBuildInputs after
// any COM call that consumes &result.Inputs, to prevent the GC from collecting
// the geometry descriptor backing array while the driver reads its pointer.
func buildInputsFromEntries(
	entries *hal.AccelerationStructureEntries,
	flags gputypes.AccelerationStructureFlags,
	buildMode *hal.AccelerationStructureBuildMode,
) *asBuildInputs {
	result := &asBuildInputs{}
	result.Inputs.Flags = mapAccelerationStructureBuildFlags(flags, buildMode)
	result.Inputs.DescsLayout = d3d12.D3D12_ELEMENTS_LAYOUT_ARRAY

	switch {
	case entries.Instances != nil:
		result.Inputs.Type = d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE_TOP_LEVEL
		result.Inputs.NumDescs = entries.Instances.Count

		var instanceAddr uint64
		if entries.Instances.Buffer != nil {
			if buf, ok := entries.Instances.Buffer.(*Buffer); ok && buf != nil {
				instanceAddr = buf.gpuVA + uint64(entries.Instances.Offset)
			}
		}
		result.Inputs.SetInstanceDescs(instanceAddr)

	case len(entries.Triangles) > 0:
		result.Inputs.Type = d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE_BOTTOM_LEVEL
		result.geometryBacking = buildTriangleGeometryDescs(entries.Triangles)
		result.Inputs.NumDescs = uint32(len(result.geometryBacking))
		if len(result.geometryBacking) > 0 {
			result.Inputs.SetGeometryDescs(unsafe.Pointer(&result.geometryBacking[0]))
		}

	case len(entries.AABBs) > 0:
		result.Inputs.Type = d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE_BOTTOM_LEVEL
		result.geometryBacking = buildAABBGeometryDescs(entries.AABBs)
		result.Inputs.NumDescs = uint32(len(result.geometryBacking))
		if len(result.geometryBacking) > 0 {
			result.Inputs.SetGeometryDescs(unsafe.Pointer(&result.geometryBacking[0]))
		}
	}

	return result
}

// buildTriangleGeometryDescs converts HAL triangle geometry descriptions to D3D12.
func buildTriangleGeometryDescs(triangles []hal.AccelerationStructureTriangles) []d3d12.D3D12_RAYTRACING_GEOMETRY_DESC {
	descs := make([]d3d12.D3D12_RAYTRACING_GEOMETRY_DESC, 0, len(triangles))
	for i := range triangles {
		tri := &triangles[i]

		var indexFormat d3d12.DXGI_FORMAT
		var indexCount uint32
		var indexAddr uint64
		if tri.Indices != nil {
			indexFormat = mapIndexFormatDXGI(tri.Indices.Format)
			indexCount = tri.Indices.Count
			if tri.Indices.Buffer != nil {
				if buf, ok := tri.Indices.Buffer.(*Buffer); ok && buf != nil {
					indexAddr = buf.gpuVA + uint64(tri.Indices.Offset)
				}
			}
		} else {
			indexFormat = d3d12.DXGI_FORMAT_UNKNOWN
		}

		var vertexAddr uint64
		if tri.VertexBuffer != nil {
			if buf, ok := tri.VertexBuffer.(*Buffer); ok && buf != nil {
				vertexAddr = buf.gpuVA + uint64(tri.FirstVertex)*tri.VertexStride
			}
		}

		var transformAddr uint64
		if tri.Transform != nil && tri.Transform.Buffer != nil {
			if buf, ok := tri.Transform.Buffer.(*Buffer); ok && buf != nil {
				transformAddr = buf.gpuVA + uint64(tri.Transform.Offset)
			}
		}

		triDesc := d3d12.D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC{
			Transform3x4: transformAddr,
			IndexFormat:  indexFormat,
			VertexFormat: vertexFormatToD3D12(tri.VertexFormat),
			IndexCount:   indexCount,
			VertexCount:  tri.VertexCount,
			IndexBuffer:  indexAddr,
			VertexBuffer: d3d12.D3D12_GPU_VIRTUAL_ADDRESS_AND_STRIDE{
				StartAddress:  vertexAddr,
				StrideInBytes: tri.VertexStride,
			},
		}

		var geomDesc d3d12.D3D12_RAYTRACING_GEOMETRY_DESC
		geomDesc.Type = d3d12.D3D12_RAYTRACING_GEOMETRY_TYPE_TRIANGLES
		geomDesc.Flags = mapAccelerationStructureGeometryFlags(tri.Flags)
		geomDesc.SetTriangles(triDesc)
		descs = append(descs, geomDesc)
	}
	return descs
}

// buildAABBGeometryDescs converts HAL AABB geometry descriptions to D3D12.
func buildAABBGeometryDescs(aabbs []hal.AccelerationStructureAABBs) []d3d12.D3D12_RAYTRACING_GEOMETRY_DESC {
	descs := make([]d3d12.D3D12_RAYTRACING_GEOMETRY_DESC, 0, len(aabbs))
	for i := range aabbs {
		aabb := &aabbs[i]

		var dataAddr uint64
		if aabb.Buffer != nil {
			if buf, ok := aabb.Buffer.(*Buffer); ok && buf != nil {
				dataAddr = buf.gpuVA + uint64(aabb.Offset)*aabb.Stride
			}
		}

		aabbDesc := d3d12.D3D12_RAYTRACING_GEOMETRY_AABBS_DESC{
			AABBCount: uint64(aabb.Count),
			AABBs: d3d12.D3D12_GPU_VIRTUAL_ADDRESS_AND_STRIDE{
				StartAddress:  dataAddr,
				StrideInBytes: aabb.Stride,
			},
		}

		var geomDesc d3d12.D3D12_RAYTRACING_GEOMETRY_DESC
		geomDesc.Type = d3d12.D3D12_RAYTRACING_GEOMETRY_TYPE_PROCEDURAL_PRIMITIVE_AABBS
		geomDesc.Flags = mapAccelerationStructureGeometryFlags(aabb.Flags)
		geomDesc.SetAABBs(aabbDesc)
		descs = append(descs, geomDesc)
	}
	return descs
}

// mapAccelerationStructureBuildFlags converts HAL AS build flags to D3D12.
// Matches Rust wgpu-hal conv::map_acceleration_structure_build_flags.
func mapAccelerationStructureBuildFlags(
	flags gputypes.AccelerationStructureFlags,
	buildMode *hal.AccelerationStructureBuildMode,
) d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS {
	var d3dFlags d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS

	if flags.Contains(gputypes.ASFlagAllowCompaction) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_ALLOW_COMPACTION
	}
	if flags.Contains(gputypes.ASFlagAllowUpdate) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_ALLOW_UPDATE
	}
	if flags.Contains(gputypes.ASFlagLowMemory) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_MINIMIZE_MEMORY
	}
	if flags.Contains(gputypes.ASFlagPreferFastBuild) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_PREFER_FAST_BUILD
	}
	if flags.Contains(gputypes.ASFlagPreferFastTrace) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_PREFER_FAST_TRACE
	}

	if buildMode != nil && *buildMode == hal.AccelerationStructureBuildModeUpdate {
		d3dFlags |= d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_PERFORM_UPDATE
	}

	return d3dFlags
}

// mapAccelerationStructureGeometryFlags converts HAL per-geometry flags to D3D12.
// Matches Rust wgpu-hal conv::map_acceleration_structure_geometry_flags.
func mapAccelerationStructureGeometryFlags(flags gputypes.AccelerationStructureGeometryFlags) d3d12.D3D12_RAYTRACING_GEOMETRY_FLAGS {
	var d3dFlags d3d12.D3D12_RAYTRACING_GEOMETRY_FLAGS
	if flags.Contains(gputypes.ASGeometryFlagOpaque) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_GEOMETRY_FLAG_OPAQUE
	}
	if flags.Contains(gputypes.ASGeometryFlagNoDuplicateAnyHitInvocation) {
		d3dFlags |= d3d12.D3D12_RAYTRACING_GEOMETRY_FLAG_NO_DUPLICATE_ANYHIT_INVOCATION
	}
	return d3dFlags
}

// mapAccelerationStructureCopyMode converts HAL copy mode to D3D12.
// Matches Rust wgpu-hal conv::map_acceleration_structure_copy_mode.
func mapAccelerationStructureCopyMode(mode gputypes.AccelerationStructureCopyMode) d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE {
	switch mode {
	case gputypes.AccelerationStructureCopyModeCompact:
		return d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_COMPACT
	default:
		return d3d12.D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_CLONE
	}
}

// mapIndexFormatDXGI converts a gputypes.IndexFormat to DXGI_FORMAT.
func mapIndexFormatDXGI(format gputypes.IndexFormat) d3d12.DXGI_FORMAT {
	if format == gputypes.IndexFormatUint32 {
		return d3d12.DXGI_FORMAT_R32_UINT
	}
	return d3d12.DXGI_FORMAT_R16_UINT
}
