// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

// DXR (DirectX Raytracing) COM bindings for ID3D12Device5 and
// ID3D12GraphicsCommandList4 ray tracing methods.
//
// Follows Rust wgpu-hal dx12/device.rs (create_acceleration_structure,
// get_acceleration_structure_build_sizes) and dx12/command.rs
// (build_acceleration_structures, place_acceleration_structure_barrier,
// copy_acceleration_structure, read_acceleration_structure_compact_size).

package d3d12

import (
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// DXR constants
// ---------------------------------------------------------------------------

// D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE distinguishes TLAS from BLAS.
type D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE uint32

const (
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE_TOP_LEVEL    D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE = 0
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE_BOTTOM_LEVEL D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE = 1
)

// D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS controls AS build behavior.
type D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS uint32

const (
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_NONE              D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_ALLOW_UPDATE      D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0x1
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_ALLOW_COMPACTION  D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0x2
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_PREFER_FAST_TRACE D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0x4
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_PREFER_FAST_BUILD D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0x8
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_MINIMIZE_MEMORY   D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0x10
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAG_PERFORM_UPDATE    D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS = 0x20
)

// D3D12_RAYTRACING_GEOMETRY_TYPE specifies the geometry type for BLAS build.
type D3D12_RAYTRACING_GEOMETRY_TYPE uint32

const (
	D3D12_RAYTRACING_GEOMETRY_TYPE_TRIANGLES                  D3D12_RAYTRACING_GEOMETRY_TYPE = 0
	D3D12_RAYTRACING_GEOMETRY_TYPE_PROCEDURAL_PRIMITIVE_AABBS D3D12_RAYTRACING_GEOMETRY_TYPE = 1
)

// D3D12_RAYTRACING_GEOMETRY_FLAGS controls per-geometry behavior.
type D3D12_RAYTRACING_GEOMETRY_FLAGS uint32

const (
	D3D12_RAYTRACING_GEOMETRY_FLAG_NONE                           D3D12_RAYTRACING_GEOMETRY_FLAGS = 0
	D3D12_RAYTRACING_GEOMETRY_FLAG_OPAQUE                         D3D12_RAYTRACING_GEOMETRY_FLAGS = 0x1
	D3D12_RAYTRACING_GEOMETRY_FLAG_NO_DUPLICATE_ANYHIT_INVOCATION D3D12_RAYTRACING_GEOMETRY_FLAGS = 0x2
)

// D3D12_ELEMENTS_LAYOUT specifies the layout of geometry descriptors.
type D3D12_ELEMENTS_LAYOUT uint32

const (
	D3D12_ELEMENTS_LAYOUT_ARRAY             D3D12_ELEMENTS_LAYOUT = 0
	D3D12_ELEMENTS_LAYOUT_ARRAY_OF_POINTERS D3D12_ELEMENTS_LAYOUT = 1
)

// D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE controls AS copy behavior.
type D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE uint32

const (
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_CLONE                          D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE = 0
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_COMPACT                        D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE = 1
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_VISUALIZATION_DECODE_FOR_TOOLS D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE = 2
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_SERIALIZE                      D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE = 3
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE_DESERIALIZE                    D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE = 4
)

// D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE identifies post-build queries.
type D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE uint32

const (
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_COMPACTED_SIZE      D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE = 0
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TOOLS_VISUALIZATION D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE = 1
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_SERIALIZATION       D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE = 2
	D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_CURRENT_SIZE        D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE = 3
)

// ---------------------------------------------------------------------------
// DXR types (matching D3D12 SDK headers)
// ---------------------------------------------------------------------------

// D3D12_GPU_VIRTUAL_ADDRESS_AND_STRIDE pairs a GPU VA with stride.
type D3D12_GPU_VIRTUAL_ADDRESS_AND_STRIDE struct {
	StartAddress  uint64
	StrideInBytes uint64
}

// D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC describes triangle geometry.
type D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC struct {
	Transform3x4 uint64 // D3D12_GPU_VIRTUAL_ADDRESS
	IndexFormat  DXGI_FORMAT
	VertexFormat DXGI_FORMAT
	IndexCount   uint32
	VertexCount  uint32
	IndexBuffer  uint64 // D3D12_GPU_VIRTUAL_ADDRESS
	VertexBuffer D3D12_GPU_VIRTUAL_ADDRESS_AND_STRIDE
}

// D3D12_RAYTRACING_GEOMETRY_AABBS_DESC describes AABB geometry.
type D3D12_RAYTRACING_GEOMETRY_AABBS_DESC struct {
	AABBCount uint64
	AABBs     D3D12_GPU_VIRTUAL_ADDRESS_AND_STRIDE
}

// D3D12_RAYTRACING_GEOMETRY_DESC describes one geometry entry.
// The Union field holds either a triangles or AABBs descriptor, interpreted
// according to Type. Its size matches the larger of the two variants
// (D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC = 64 bytes).
type D3D12_RAYTRACING_GEOMETRY_DESC struct {
	Type  D3D12_RAYTRACING_GEOMETRY_TYPE
	Flags D3D12_RAYTRACING_GEOMETRY_FLAGS
	// Union: either D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC (64 bytes)
	// or D3D12_RAYTRACING_GEOMETRY_AABBS_DESC (24 bytes).
	Union [64]byte
}

// SetTriangles writes a triangles descriptor into the union.
func (g *D3D12_RAYTRACING_GEOMETRY_DESC) SetTriangles(desc D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC) {
	*(*D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC)(unsafe.Pointer(&g.Union[0])) = desc
}

// SetAABBs writes an AABBs descriptor into the union.
func (g *D3D12_RAYTRACING_GEOMETRY_DESC) SetAABBs(desc D3D12_RAYTRACING_GEOMETRY_AABBS_DESC) {
	*(*D3D12_RAYTRACING_GEOMETRY_AABBS_DESC)(unsafe.Pointer(&g.Union[0])) = desc
}

// D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS describes build inputs.
// The Union field is either InstanceDescs (uint64 GPU VA for TLAS) or
// pGeometryDescs (uintptr pointer to geometry desc array for BLAS).
type D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS struct {
	Type        D3D12_RAYTRACING_ACCELERATION_STRUCTURE_TYPE
	Flags       D3D12_RAYTRACING_ACCELERATION_STRUCTURE_BUILD_FLAGS
	NumDescs    uint32
	DescsLayout D3D12_ELEMENTS_LAYOUT
	// Union: InstanceDescs (uint64) or pGeometryDescs (uintptr).
	// Both fit in 8 bytes on 64-bit.
	Union [8]byte
}

// SetInstanceDescs sets the union to InstanceDescs (GPU virtual address).
func (i *D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS) SetInstanceDescs(gpuVA uint64) {
	*(*uint64)(unsafe.Pointer(&i.Union[0])) = gpuVA
}

// SetGeometryDescs sets the union to pGeometryDescs (pointer to array).
func (i *D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS) SetGeometryDescs(ptr unsafe.Pointer) {
	*(*uintptr)(unsafe.Pointer(&i.Union[0])) = uintptr(ptr)
}

// D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_DESC describes a full AS build.
type D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_DESC struct {
	DestAccelerationStructureData    uint64 // D3D12_GPU_VIRTUAL_ADDRESS
	Inputs                           D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS
	SourceAccelerationStructureData  uint64 // D3D12_GPU_VIRTUAL_ADDRESS (0 for initial build)
	ScratchAccelerationStructureData uint64 // D3D12_GPU_VIRTUAL_ADDRESS
}

// D3D12_RAYTRACING_ACCELERATION_STRUCTURE_PREBUILD_INFO holds size info from prebuild query.
type D3D12_RAYTRACING_ACCELERATION_STRUCTURE_PREBUILD_INFO struct {
	ResultDataMaxSizeInBytes     uint64
	ScratchDataSizeInBytes       uint64
	UpdateScratchDataSizeInBytes uint64
}

// D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_DESC describes a post-build info query.
type D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_DESC struct {
	DestBuffer uint64 // D3D12_GPU_VIRTUAL_ADDRESS
	InfoType   D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_TYPE
	_          [4]byte // padding to align
}

// D3D12_RAYTRACING_INSTANCE_DESC is the 64-byte packed instance descriptor for TLAS.
// Layout: Transform [3][4]float32 (48 bytes) + bitfield1 (4 bytes) + bitfield2 (4 bytes)
// + AccelerationStructure (8 bytes) = 64 bytes total.
type D3D12_RAYTRACING_INSTANCE_DESC struct {
	Transform               [12]float32 // Row-major 3x4 affine transform
	InstanceIDAndMask       uint32      // InstanceID (bits 0-23), Mask (bits 24-31)
	InstanceContribAndFlags uint32      // InstanceContributionToHitGroupIndex (bits 0-23), Flags (bits 24-31)
	AccelerationStructure   uint64      // GPU virtual address of BLAS
}

// ---------------------------------------------------------------------------
// ID3D12Device5 — DXR device interface
// ---------------------------------------------------------------------------

// ID3D12Device5 extends ID3D12Device with DXR support.
// Obtained via QueryInterface on ID3D12Device.
type ID3D12Device5 struct {
	vtbl *id3d12Device5Vtbl
}

// id3d12Device5Vtbl contains the full vtable from IUnknown through ID3D12Device5.
// Only DXR-relevant methods are named; the rest are placeholders for correct offsets.
type id3d12Device5Vtbl struct {
	// IUnknown (3)
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object (4)
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12Device (26)
	GetNodeCount                     uintptr
	CreateCommandQueue               uintptr
	CreateCommandAllocator           uintptr
	CreateGraphicsPipelineState      uintptr
	CreateComputePipelineState       uintptr
	CreateCommandList                uintptr
	CheckFeatureSupport              uintptr
	CreateDescriptorHeap             uintptr
	GetDescriptorHandleIncrementSize uintptr
	CreateRootSignature              uintptr
	CreateConstantBufferView         uintptr
	CreateShaderResourceView         uintptr
	CreateUnorderedAccessView        uintptr
	CreateRenderTargetView           uintptr
	CreateDepthStencilView           uintptr
	CreateSampler                    uintptr
	CopyDescriptors                  uintptr
	CopyDescriptorsSimple            uintptr
	GetResourceAllocationInfo        uintptr
	GetCustomHeapProperties          uintptr
	CreateCommittedResource          uintptr
	CreateHeap                       uintptr
	CreatePlacedResource             uintptr
	CreateReservedResource           uintptr
	CreateSharedHandle               uintptr
	OpenSharedHandle                 uintptr
	OpenSharedHandleByName           uintptr
	MakeResident                     uintptr
	Evict                            uintptr
	CreateFence                      uintptr
	GetDeviceRemovedReason           uintptr
	GetCopyableFootprints            uintptr
	CreateQueryHeap                  uintptr
	SetStablePowerState              uintptr
	CreateCommandSignature           uintptr
	GetResourceTiling                uintptr
	GetAdapterLuid                   uintptr

	// ID3D12Device1 (3)
	CreatePipelineLibrary             uintptr
	SetEventOnMultipleFenceCompletion uintptr
	SetResidencyPriority              uintptr

	// ID3D12Device2 (1)
	CreatePipelineState2 uintptr // CreatePipelineState (stream desc)

	// ID3D12Device3 (3)
	OpenExistingHeapFromAddress     uintptr
	OpenExistingHeapFromFileMapping uintptr
	EnqueueMakeResident             uintptr

	// ID3D12Device4 (6)
	CreateCommandList1             uintptr
	CreateProtectedResourceSession uintptr
	CreateCommittedResource1       uintptr
	CreateHeap1                    uintptr
	CreateReservedResource1        uintptr
	GetResourceAllocationInfo1     uintptr

	// ID3D12Device5 (8)
	CreateLifetimeTracker                          uintptr
	RemoveDevice                                   uintptr
	EnumerateMetaCommands                          uintptr
	EnumerateMetaCommandParameters                 uintptr
	CreateMetaCommand                              uintptr
	CreateStateObject                              uintptr
	GetRaytracingAccelerationStructurePrebuildInfo uintptr
	CheckDriverMatchingIdentifier                  uintptr
}

// QueryDevice5 obtains an ID3D12Device5 interface from an ID3D12Device.
// Returns nil if DXR is not supported.
func (d *ID3D12Device) QueryDevice5() *ID3D12Device5 {
	var dev5 *ID3D12Device5
	ret, _, _ := syscall.Syscall(
		d.vtbl.QueryInterface,
		3,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(&IID_ID3D12Device5)),
		uintptr(unsafe.Pointer(&dev5)),
	)
	if ret != 0 {
		return nil
	}
	return dev5
}

// Release decrements the reference count.
func (d *ID3D12Device5) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// GetRaytracingAccelerationStructurePrebuildInfo queries size requirements
// for building an acceleration structure.
func (d *ID3D12Device5) GetRaytracingAccelerationStructurePrebuildInfo(
	desc *D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS,
	info *D3D12_RAYTRACING_ACCELERATION_STRUCTURE_PREBUILD_INFO,
) {
	_, _, _ = syscall.Syscall(
		d.vtbl.GetRaytracingAccelerationStructurePrebuildInfo,
		3,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(info)),
	)
}

// ---------------------------------------------------------------------------
// ID3D12GraphicsCommandList4 DXR methods
// ---------------------------------------------------------------------------

// BuildRaytracingAccelerationStructure records an AS build command.
func (c *ID3D12GraphicsCommandList4) BuildRaytracingAccelerationStructure(
	desc *D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_DESC,
	numPostbuildInfoDescs uint32,
	postbuildInfoDescs *D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_DESC,
) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.BuildRaytracingAccelerationStructure,
		4,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(numPostbuildInfoDescs),
		uintptr(unsafe.Pointer(postbuildInfoDescs)),
		0, 0,
	)
}

// EmitRaytracingAccelerationStructurePostbuildInfo records a post-build info query.
func (c *ID3D12GraphicsCommandList4) EmitRaytracingAccelerationStructurePostbuildInfo(
	desc *D3D12_RAYTRACING_ACCELERATION_STRUCTURE_POSTBUILD_INFO_DESC,
	numSourceAccelerationStructures uint32,
	sourceAccelerationStructureData *uint64,
) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.EmitRaytracingAccelerationStructurePostbuildInfo,
		4,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(numSourceAccelerationStructures),
		uintptr(unsafe.Pointer(sourceAccelerationStructureData)),
		0, 0,
	)
}

// CopyRaytracingAccelerationStructure records an AS copy/compact command.
func (c *ID3D12GraphicsCommandList4) CopyRaytracingAccelerationStructure(
	destAccelerationStructureData uint64,
	sourceAccelerationStructureData uint64,
	mode D3D12_RAYTRACING_ACCELERATION_STRUCTURE_COPY_MODE,
) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.CopyRaytracingAccelerationStructure,
		4,
		uintptr(unsafe.Pointer(c)),
		uintptr(destAccelerationStructureData),
		uintptr(sourceAccelerationStructureData),
		uintptr(mode),
		0, 0,
	)
}

// Release decrements the reference count of the command list interface.
// Must be called after QueryCommandList4 to avoid a COM leak.
func (c *ID3D12GraphicsCommandList4) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		c.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(c)),
		0, 0,
	)
	return uint32(ret)
}

// QueryCommandList4 obtains an ID3D12GraphicsCommandList4 interface from
// an ID3D12GraphicsCommandList via QueryInterface.
// Returns nil if the command list does not support DXR.
func (c *ID3D12GraphicsCommandList) QueryCommandList4() *ID3D12GraphicsCommandList4 {
	var list4 *ID3D12GraphicsCommandList4
	ret, _, _ := syscall.Syscall(
		c.vtbl.QueryInterface,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(&IID_ID3D12GraphicsCommandList4)),
		uintptr(unsafe.Pointer(&list4)),
	)
	if ret != 0 {
		return nil
	}
	return list4
}
