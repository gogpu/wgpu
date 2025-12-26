// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

// ID3D12Object is the base interface for D3D12 objects.
type ID3D12Object struct {
	vtbl *id3d12ObjectVtbl
}

type id3d12ObjectVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr
}

// ID3D12DeviceChild is the base interface for D3D12 device child objects.
type ID3D12DeviceChild struct {
	vtbl *id3d12DeviceChildVtbl
}

type id3d12DeviceChildVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr
}

// ID3D12Pageable is the base interface for pageable D3D12 objects.
type ID3D12Pageable struct {
	vtbl *id3d12PageableVtbl
}

type id3d12PageableVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods
}

// ID3D12Device represents a virtual adapter.
type ID3D12Device struct {
	vtbl *id3d12DeviceVtbl
}

type id3d12DeviceVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12Device
	GetNodeCount                                    uintptr
	CreateCommandQueue                              uintptr
	CreateCommandAllocator                          uintptr
	CreateGraphicsPipelineState                     uintptr
	CreateComputePipelineState                      uintptr
	CreateCommandList                               uintptr
	CheckFeatureSupport                             uintptr
	CreateDescriptorHeap                            uintptr
	GetDescriptorHandleIncrementSize                uintptr
	CreateRootSignature                             uintptr
	CreateConstantBufferView                        uintptr
	CreateShaderResourceView                        uintptr
	CreateUnorderedAccessView                       uintptr
	CreateRenderTargetView                          uintptr
	CreateDepthStencilView                          uintptr
	CreateSampler                                   uintptr
	CopyDescriptors                                 uintptr
	CopyDescriptorsSimple                           uintptr
	GetResourceAllocationInfo                       uintptr
	GetCustomHeapProperties                         uintptr
	CreateCommittedResource                         uintptr
	CreateHeap                                      uintptr
	CreatePlacedResource                            uintptr
	CreateReservedResource                          uintptr
	CreateSharedHandle                              uintptr
	OpenSharedHandle                                uintptr
	OpenSharedHandleByName                          uintptr
	MakeResident                                    uintptr
	Evict                                           uintptr
	CreateFence                                     uintptr
	GetDeviceRemovedReason                          uintptr
	GetCopyableFootprints                           uintptr
	CreateQueryHeap                                 uintptr
	SetStablePowerState                             uintptr
	CreateCommandSignature                          uintptr
	GetResourceTiling                               uintptr
	GetAdapterLuid                                  uintptr
}

// ID3D12CommandQueue represents a command queue.
type ID3D12CommandQueue struct {
	vtbl *id3d12CommandQueueVtbl
}

type id3d12CommandQueueVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12CommandQueue
	UpdateTileMappings        uintptr
	CopyTileMappings          uintptr
	ExecuteCommandLists       uintptr
	SetMarker                 uintptr
	BeginEvent                uintptr
	EndEvent                  uintptr
	Signal                    uintptr
	Wait                      uintptr
	GetTimestampFrequency     uintptr
	GetClockCalibration       uintptr
	GetDesc                   uintptr
}

// ID3D12CommandAllocator represents a command allocator.
type ID3D12CommandAllocator struct {
	vtbl *id3d12CommandAllocatorVtbl
}

type id3d12CommandAllocatorVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12CommandAllocator
	Reset uintptr
}

// ID3D12CommandList is the base interface for command lists.
type ID3D12CommandList struct {
	vtbl *id3d12CommandListVtbl
}

type id3d12CommandListVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12CommandList
	GetType uintptr
}

// ID3D12GraphicsCommandList represents a graphics command list.
type ID3D12GraphicsCommandList struct {
	vtbl *id3d12GraphicsCommandListVtbl
}

type id3d12GraphicsCommandListVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12CommandList
	GetType uintptr

	// ID3D12GraphicsCommandList
	Close                            uintptr
	Reset                            uintptr
	ClearState                       uintptr
	DrawInstanced                    uintptr
	DrawIndexedInstanced             uintptr
	Dispatch                         uintptr
	CopyBufferRegion                 uintptr
	CopyTextureRegion                uintptr
	CopyResource                     uintptr
	CopyTiles                        uintptr
	ResolveSubresource               uintptr
	IASetPrimitiveTopology           uintptr
	RSSetViewports                   uintptr
	RSSetScissorRects                uintptr
	OMSetBlendFactor                 uintptr
	OMSetStencilRef                  uintptr
	SetPipelineState                 uintptr
	ResourceBarrier                  uintptr
	ExecuteBundle                    uintptr
	SetDescriptorHeaps               uintptr
	SetComputeRootSignature          uintptr
	SetGraphicsRootSignature         uintptr
	SetComputeRootDescriptorTable    uintptr
	SetGraphicsRootDescriptorTable   uintptr
	SetComputeRoot32BitConstant      uintptr
	SetGraphicsRoot32BitConstant     uintptr
	SetComputeRoot32BitConstants     uintptr
	SetGraphicsRoot32BitConstants    uintptr
	SetComputeRootConstantBufferView uintptr
	SetGraphicsRootConstantBufferView uintptr
	SetComputeRootShaderResourceView uintptr
	SetGraphicsRootShaderResourceView uintptr
	SetComputeRootUnorderedAccessView uintptr
	SetGraphicsRootUnorderedAccessView uintptr
	IASetIndexBuffer                 uintptr
	IASetVertexBuffers               uintptr
	SOSetTargets                     uintptr
	OMSetRenderTargets               uintptr
	ClearDepthStencilView            uintptr
	ClearRenderTargetView            uintptr
	ClearUnorderedAccessViewUint     uintptr
	ClearUnorderedAccessViewFloat    uintptr
	DiscardResource                  uintptr
	BeginQuery                       uintptr
	EndQuery                         uintptr
	ResolveQueryData                 uintptr
	SetPredication                   uintptr
	SetMarker                        uintptr
	BeginEvent                       uintptr
	EndEvent                         uintptr
	ExecuteIndirect                  uintptr
}

// ID3D12GraphicsCommandList1 extends ID3D12GraphicsCommandList.
type ID3D12GraphicsCommandList1 struct {
	vtbl *id3d12GraphicsCommandList1Vtbl
}

type id3d12GraphicsCommandList1Vtbl struct {
	id3d12GraphicsCommandListVtbl

	// ID3D12GraphicsCommandList1
	AtomicCopyBufferUINT       uintptr
	AtomicCopyBufferUINT64     uintptr
	OMSetDepthBounds           uintptr
	SetSamplePositions         uintptr
	ResolveSubresourceRegion   uintptr
	SetViewInstanceMask        uintptr
}

// ID3D12GraphicsCommandList2 extends ID3D12GraphicsCommandList1.
type ID3D12GraphicsCommandList2 struct {
	vtbl *id3d12GraphicsCommandList2Vtbl
}

type id3d12GraphicsCommandList2Vtbl struct {
	id3d12GraphicsCommandList1Vtbl

	// ID3D12GraphicsCommandList2
	WriteBufferImmediate uintptr
}

// ID3D12GraphicsCommandList3 extends ID3D12GraphicsCommandList2.
type ID3D12GraphicsCommandList3 struct {
	vtbl *id3d12GraphicsCommandList3Vtbl
}

type id3d12GraphicsCommandList3Vtbl struct {
	id3d12GraphicsCommandList2Vtbl

	// ID3D12GraphicsCommandList3
	SetProtectedResourceSession uintptr
}

// ID3D12GraphicsCommandList4 extends ID3D12GraphicsCommandList3.
type ID3D12GraphicsCommandList4 struct {
	vtbl *id3d12GraphicsCommandList4Vtbl
}

type id3d12GraphicsCommandList4Vtbl struct {
	id3d12GraphicsCommandList3Vtbl

	// ID3D12GraphicsCommandList4
	BeginRenderPass                          uintptr
	EndRenderPass                            uintptr
	InitializeMetaCommand                    uintptr
	ExecuteMetaCommand                       uintptr
	BuildRaytracingAccelerationStructure     uintptr
	EmitRaytracingAccelerationStructurePostbuildInfo uintptr
	CopyRaytracingAccelerationStructure      uintptr
	SetPipelineState1                        uintptr
	DispatchRays                             uintptr
}

// ID3D12Fence represents a fence for synchronization.
type ID3D12Fence struct {
	vtbl *id3d12FenceVtbl
}

type id3d12FenceVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12Fence
	GetCompletedValue uintptr
	SetEventOnCompletion uintptr
	Signal            uintptr
}

// ID3D12Fence1 extends ID3D12Fence.
type ID3D12Fence1 struct {
	vtbl *id3d12Fence1Vtbl
}

type id3d12Fence1Vtbl struct {
	id3d12FenceVtbl

	// ID3D12Fence1
	GetCreationFlags uintptr
}

// ID3D12Resource represents a resource.
type ID3D12Resource struct {
	vtbl *id3d12ResourceVtbl
}

type id3d12ResourceVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12Resource
	Map               uintptr
	Unmap             uintptr
	GetDesc           uintptr
	GetGPUVirtualAddress uintptr
	WriteToSubresource uintptr
	ReadFromSubresource uintptr
	GetHeapProperties uintptr
}

// ID3D12Resource1 extends ID3D12Resource.
type ID3D12Resource1 struct {
	vtbl *id3d12Resource1Vtbl
}

type id3d12Resource1Vtbl struct {
	id3d12ResourceVtbl

	// ID3D12Resource1
	GetProtectedResourceSession uintptr
}

// ID3D12DescriptorHeap represents a descriptor heap.
type ID3D12DescriptorHeap struct {
	vtbl *id3d12DescriptorHeapVtbl
}

type id3d12DescriptorHeapVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12DescriptorHeap
	GetDesc                  uintptr
	GetCPUDescriptorHandleForHeapStart uintptr
	GetGPUDescriptorHandleForHeapStart uintptr
}

// ID3D12Heap represents a heap.
type ID3D12Heap struct {
	vtbl *id3d12HeapVtbl
}

type id3d12HeapVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12Heap
	GetDesc uintptr
}

// ID3D12Heap1 extends ID3D12Heap.
type ID3D12Heap1 struct {
	vtbl *id3d12Heap1Vtbl
}

type id3d12Heap1Vtbl struct {
	id3d12HeapVtbl

	// ID3D12Heap1
	GetProtectedResourceSession uintptr
}

// ID3D12PipelineState represents a pipeline state.
type ID3D12PipelineState struct {
	vtbl *id3d12PipelineStateVtbl
}

type id3d12PipelineStateVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12PipelineState
	GetCachedBlob uintptr
}

// ID3D12RootSignature represents a root signature.
type ID3D12RootSignature struct {
	vtbl *id3d12RootSignatureVtbl
}

type id3d12RootSignatureVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12RootSignature has no additional methods
}

// ID3D12QueryHeap represents a query heap.
type ID3D12QueryHeap struct {
	vtbl *id3d12QueryHeapVtbl
}

type id3d12QueryHeapVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12QueryHeap has no additional methods
}

// ID3D12CommandSignature represents a command signature.
type ID3D12CommandSignature struct {
	vtbl *id3d12CommandSignatureVtbl
}

type id3d12CommandSignatureVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Object
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	SetName                 uintptr

	// ID3D12DeviceChild
	GetDevice uintptr

	// ID3D12Pageable has no additional methods

	// ID3D12CommandSignature has no additional methods
}

// ID3D12Debug is the debug interface.
type ID3D12Debug struct {
	vtbl *id3d12DebugVtbl
}

type id3d12DebugVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Debug
	EnableDebugLayer uintptr
}

// ID3D12Debug1 extends ID3D12Debug.
type ID3D12Debug1 struct {
	vtbl *id3d12Debug1Vtbl
}

type id3d12Debug1Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Debug
	EnableDebugLayer uintptr

	// ID3D12Debug1
	SetEnableGPUBasedValidation uintptr
	SetEnableSynchronizedCommandQueueValidation uintptr
}

// ID3D12Debug2 extends ID3D12Debug1.
type ID3D12Debug2 struct {
	vtbl *id3d12Debug2Vtbl
}

type id3d12Debug2Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Debug2 (note: does not include EnableDebugLayer from ID3D12Debug)
	SetGPUBasedValidationFlags uintptr
}

// ID3D12Debug3 extends ID3D12Debug.
type ID3D12Debug3 struct {
	vtbl *id3d12Debug3Vtbl
}

type id3d12Debug3Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12Debug
	EnableDebugLayer uintptr

	// ID3D12Debug1
	SetEnableGPUBasedValidation uintptr
	SetEnableSynchronizedCommandQueueValidation uintptr

	// ID3D12Debug3
	SetGPUBasedValidationFlags uintptr
}

// ID3D12InfoQueue is the info queue interface for debug messages.
type ID3D12InfoQueue struct {
	vtbl *id3d12InfoQueueVtbl
}

type id3d12InfoQueueVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12InfoQueue
	SetMessageCountLimit          uintptr
	ClearStoredMessages           uintptr
	GetMessage                    uintptr
	GetNumMessagesAllowedByStorageFilter uintptr
	GetNumMessagesDeniedByStorageFilter uintptr
	GetNumStoredMessages          uintptr
	GetNumStoredMessagesAllowedByRetrievalFilter uintptr
	GetNumMessagesDiscardedByMessageCountLimit uintptr
	GetMessageCountLimit          uintptr
	AddStorageFilterEntries       uintptr
	GetStorageFilter              uintptr
	ClearStorageFilter            uintptr
	PushEmptyStorageFilter        uintptr
	PushCopyOfStorageFilter       uintptr
	PushStorageFilter             uintptr
	PopStorageFilter              uintptr
	GetStorageFilterStackSize     uintptr
	AddRetrievalFilterEntries     uintptr
	GetRetrievalFilter            uintptr
	ClearRetrievalFilter          uintptr
	PushEmptyRetrievalFilter      uintptr
	PushCopyOfRetrievalFilter     uintptr
	PushRetrievalFilter           uintptr
	PopRetrievalFilter            uintptr
	GetRetrievalFilterStackSize   uintptr
	AddMessage                    uintptr
	AddApplicationMessage         uintptr
	SetBreakOnCategory            uintptr
	SetBreakOnSeverity            uintptr
	SetBreakOnID                  uintptr
	GetBreakOnCategory            uintptr
	GetBreakOnSeverity            uintptr
	GetBreakOnID                  uintptr
	SetMuteDebugOutput            uintptr
	GetMuteDebugOutput            uintptr
}

// ID3DBlob represents a binary blob (typically shader bytecode).
type ID3DBlob struct {
	vtbl *id3dBlobVtbl
}

type id3dBlobVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3DBlob
	GetBufferPointer uintptr
	GetBufferSize    uintptr
}
