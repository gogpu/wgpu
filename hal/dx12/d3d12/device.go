// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

import (
	"syscall"
	"unsafe"
)

// -----------------------------------------------------------------------------
// IUnknown methods (shared by all COM interfaces)
// -----------------------------------------------------------------------------

// Release decrements the reference count of the object.
func (d *ID3D12Device) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// AddRef increments the reference count of the object.
func (d *ID3D12Device) AddRef() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.AddRef,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// -----------------------------------------------------------------------------
// ID3D12Device methods
// -----------------------------------------------------------------------------

// GetNodeCount returns the number of physical adapters (nodes) associated with this device.
func (d *ID3D12Device) GetNodeCount() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.GetNodeCount,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// CheckFeatureSupport queries feature support.
// feature specifies which feature to query.
// featureData is a pointer to the feature-specific data structure.
// featureDataSize is the size of the data structure in bytes.
func (d *ID3D12Device) CheckFeatureSupport(feature D3D12_FEATURE, featureData unsafe.Pointer, featureDataSize uint32) error {
	ret, _, _ := syscall.Syscall6(
		d.vtbl.CheckFeatureSupport,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(feature),
		uintptr(featureData),
		uintptr(featureDataSize),
		0, 0,
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// CreateCommandQueue creates a command queue.
func (d *ID3D12Device) CreateCommandQueue(desc *D3D12_COMMAND_QUEUE_DESC) (*ID3D12CommandQueue, error) {
	var queue *ID3D12CommandQueue

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateCommandQueue,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(&IID_ID3D12CommandQueue)),
		uintptr(unsafe.Pointer(&queue)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return queue, nil
}

// CreateCommandAllocator creates a command allocator.
func (d *ID3D12Device) CreateCommandAllocator(listType D3D12_COMMAND_LIST_TYPE) (*ID3D12CommandAllocator, error) {
	var allocator *ID3D12CommandAllocator

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateCommandAllocator,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(listType),
		uintptr(unsafe.Pointer(&IID_ID3D12CommandAllocator)),
		uintptr(unsafe.Pointer(&allocator)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return allocator, nil
}

// CreateGraphicsPipelineState creates a graphics pipeline state.
func (d *ID3D12Device) CreateGraphicsPipelineState(desc *D3D12_GRAPHICS_PIPELINE_STATE_DESC) (*ID3D12PipelineState, error) {
	var pso *ID3D12PipelineState

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateGraphicsPipelineState,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(&IID_ID3D12PipelineState)),
		uintptr(unsafe.Pointer(&pso)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return pso, nil
}

// CreateComputePipelineState creates a compute pipeline state.
func (d *ID3D12Device) CreateComputePipelineState(desc *D3D12_COMPUTE_PIPELINE_STATE_DESC) (*ID3D12PipelineState, error) {
	var pso *ID3D12PipelineState

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateComputePipelineState,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(&IID_ID3D12PipelineState)),
		uintptr(unsafe.Pointer(&pso)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return pso, nil
}

// CreateCommandList creates a command list.
func (d *ID3D12Device) CreateCommandList(
	nodeMask uint32,
	listType D3D12_COMMAND_LIST_TYPE,
	allocator *ID3D12CommandAllocator,
	initialState *ID3D12PipelineState,
) (*ID3D12GraphicsCommandList, error) {
	var cmdList *ID3D12GraphicsCommandList

	ret, _, _ := syscall.Syscall9(
		d.vtbl.CreateCommandList,
		7,
		uintptr(unsafe.Pointer(d)),
		uintptr(nodeMask),
		uintptr(listType),
		uintptr(unsafe.Pointer(allocator)),
		uintptr(unsafe.Pointer(initialState)),
		uintptr(unsafe.Pointer(&IID_ID3D12GraphicsCommandList)),
		uintptr(unsafe.Pointer(&cmdList)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return cmdList, nil
}

// CreateDescriptorHeap creates a descriptor heap.
func (d *ID3D12Device) CreateDescriptorHeap(desc *D3D12_DESCRIPTOR_HEAP_DESC) (*ID3D12DescriptorHeap, error) {
	var heap *ID3D12DescriptorHeap

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateDescriptorHeap,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(&IID_ID3D12DescriptorHeap)),
		uintptr(unsafe.Pointer(&heap)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return heap, nil
}

// GetDescriptorHandleIncrementSize returns the size of the increment for the specified descriptor heap type.
func (d *ID3D12Device) GetDescriptorHandleIncrementSize(heapType D3D12_DESCRIPTOR_HEAP_TYPE) uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.GetDescriptorHandleIncrementSize,
		2,
		uintptr(unsafe.Pointer(d)),
		uintptr(heapType),
		0,
	)
	return uint32(ret)
}

// CreateRootSignature creates a root signature from serialized data.
func (d *ID3D12Device) CreateRootSignature(
	nodeMask uint32,
	blobWithRootSignature unsafe.Pointer,
	blobLengthInBytes uintptr,
) (*ID3D12RootSignature, error) {
	var rootSig *ID3D12RootSignature

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateRootSignature,
		6,
		uintptr(unsafe.Pointer(d)),
		uintptr(nodeMask),
		uintptr(blobWithRootSignature),
		blobLengthInBytes,
		uintptr(unsafe.Pointer(&IID_ID3D12RootSignature)),
		uintptr(unsafe.Pointer(&rootSig)),
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return rootSig, nil
}

// CreateConstantBufferView creates a constant buffer view.
func (d *ID3D12Device) CreateConstantBufferView(desc *D3D12_CONSTANT_BUFFER_VIEW_DESC, destDescriptor D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall(
		d.vtbl.CreateConstantBufferView,
		3,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		destDescriptor.Ptr,
	)
}

// CreateShaderResourceView creates a shader resource view.
func (d *ID3D12Device) CreateShaderResourceView(resource *ID3D12Resource, desc *D3D12_SHADER_RESOURCE_VIEW_DESC, destDescriptor D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall6(
		d.vtbl.CreateShaderResourceView,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(resource)),
		uintptr(unsafe.Pointer(desc)),
		destDescriptor.Ptr,
		0, 0,
	)
}

// CreateUnorderedAccessView creates an unordered access view.
func (d *ID3D12Device) CreateUnorderedAccessView(resource *ID3D12Resource, counterResource *ID3D12Resource, desc *D3D12_UNORDERED_ACCESS_VIEW_DESC, destDescriptor D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall6(
		d.vtbl.CreateUnorderedAccessView,
		5,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(resource)),
		uintptr(unsafe.Pointer(counterResource)),
		uintptr(unsafe.Pointer(desc)),
		destDescriptor.Ptr,
		0,
	)
}

// CreateRenderTargetView creates a render target view.
func (d *ID3D12Device) CreateRenderTargetView(resource *ID3D12Resource, desc *D3D12_RENDER_TARGET_VIEW_DESC, destDescriptor D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall6(
		d.vtbl.CreateRenderTargetView,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(resource)),
		uintptr(unsafe.Pointer(desc)),
		destDescriptor.Ptr,
		0, 0,
	)
}

// CreateDepthStencilView creates a depth stencil view.
func (d *ID3D12Device) CreateDepthStencilView(resource *ID3D12Resource, desc *D3D12_DEPTH_STENCIL_VIEW_DESC, destDescriptor D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall6(
		d.vtbl.CreateDepthStencilView,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(resource)),
		uintptr(unsafe.Pointer(desc)),
		destDescriptor.Ptr,
		0, 0,
	)
}

// CreateSampler creates a sampler.
func (d *ID3D12Device) CreateSampler(desc *D3D12_SAMPLER_DESC, destDescriptor D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall(
		d.vtbl.CreateSampler,
		3,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		destDescriptor.Ptr,
	)
}

// CreateCommittedResource creates a committed resource (heap + resource).
func (d *ID3D12Device) CreateCommittedResource(
	heapProperties *D3D12_HEAP_PROPERTIES,
	heapFlags D3D12_HEAP_FLAGS,
	desc *D3D12_RESOURCE_DESC,
	initialResourceState D3D12_RESOURCE_STATES,
	optimizedClearValue *D3D12_CLEAR_VALUE,
) (*ID3D12Resource, error) {
	var resource *ID3D12Resource

	ret, _, _ := syscall.Syscall9(
		d.vtbl.CreateCommittedResource,
		8,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(heapProperties)),
		uintptr(heapFlags),
		uintptr(unsafe.Pointer(desc)),
		uintptr(initialResourceState),
		uintptr(unsafe.Pointer(optimizedClearValue)),
		uintptr(unsafe.Pointer(&IID_ID3D12Resource)),
		uintptr(unsafe.Pointer(&resource)),
		0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return resource, nil
}

// CreateHeap creates a heap.
func (d *ID3D12Device) CreateHeap(desc *D3D12_HEAP_DESC) (*ID3D12Heap, error) {
	var heap *ID3D12Heap

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateHeap,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(&IID_ID3D12Heap)),
		uintptr(unsafe.Pointer(&heap)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return heap, nil
}

// CreatePlacedResource creates a resource placed in an existing heap.
func (d *ID3D12Device) CreatePlacedResource(
	heap *ID3D12Heap,
	heapOffset uint64,
	desc *D3D12_RESOURCE_DESC,
	initialState D3D12_RESOURCE_STATES,
	optimizedClearValue *D3D12_CLEAR_VALUE,
) (*ID3D12Resource, error) {
	var resource *ID3D12Resource

	ret, _, _ := syscall.Syscall9(
		d.vtbl.CreatePlacedResource,
		8,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(heap)),
		uintptr(heapOffset),
		uintptr(unsafe.Pointer(desc)),
		uintptr(initialState),
		uintptr(unsafe.Pointer(optimizedClearValue)),
		uintptr(unsafe.Pointer(&IID_ID3D12Resource)),
		uintptr(unsafe.Pointer(&resource)),
		0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return resource, nil
}

// CreateFence creates a fence.
func (d *ID3D12Device) CreateFence(initialValue uint64, flags D3D12_FENCE_FLAGS) (*ID3D12Fence, error) {
	var fence *ID3D12Fence

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateFence,
		5,
		uintptr(unsafe.Pointer(d)),
		uintptr(initialValue),
		uintptr(flags),
		uintptr(unsafe.Pointer(&IID_ID3D12Fence)),
		uintptr(unsafe.Pointer(&fence)),
		0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return fence, nil
}

// GetDeviceRemovedReason returns the reason the device was removed.
func (d *ID3D12Device) GetDeviceRemovedReason() error {
	ret, _, _ := syscall.Syscall(
		d.vtbl.GetDeviceRemovedReason,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// CreateQueryHeap creates a query heap.
func (d *ID3D12Device) CreateQueryHeap(desc *D3D12_QUERY_HEAP_DESC) (*ID3D12QueryHeap, error) {
	var heap *ID3D12QueryHeap

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateQueryHeap,
		4,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(&IID_ID3D12QueryHeap)),
		uintptr(unsafe.Pointer(&heap)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return heap, nil
}

// CreateCommandSignature creates a command signature.
func (d *ID3D12Device) CreateCommandSignature(
	desc *D3D12_COMMAND_SIGNATURE_DESC,
	rootSignature *ID3D12RootSignature,
) (*ID3D12CommandSignature, error) {
	var sig *ID3D12CommandSignature

	ret, _, _ := syscall.Syscall6(
		d.vtbl.CreateCommandSignature,
		5,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(rootSignature)),
		uintptr(unsafe.Pointer(&IID_ID3D12CommandSignature)),
		uintptr(unsafe.Pointer(&sig)),
		0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return sig, nil
}

// GetResourceAllocationInfo returns resource allocation info.
func (d *ID3D12Device) GetResourceAllocationInfo(visibleMask uint32, numResourceDescs uint32, resourceDescs *D3D12_RESOURCE_DESC) D3D12_RESOURCE_ALLOCATION_INFO {
	var info D3D12_RESOURCE_ALLOCATION_INFO

	// Note: GetResourceAllocationInfo returns the struct by value, which on Windows x64
	// means the caller must provide a pointer to receive the result as the first hidden parameter.
	_, _, _ = syscall.Syscall6(
		d.vtbl.GetResourceAllocationInfo,
		5,
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(d)),
		uintptr(visibleMask),
		uintptr(numResourceDescs),
		uintptr(unsafe.Pointer(resourceDescs)),
		0,
	)

	return info
}

// -----------------------------------------------------------------------------
// ID3D12CommandQueue methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (q *ID3D12CommandQueue) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		q.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(q)),
		0, 0,
	)
	return uint32(ret)
}

// ExecuteCommandLists submits command lists for execution.
func (q *ID3D12CommandQueue) ExecuteCommandLists(numCommandLists uint32, commandLists **ID3D12GraphicsCommandList) {
	_, _, _ = syscall.Syscall(
		q.vtbl.ExecuteCommandLists,
		3,
		uintptr(unsafe.Pointer(q)),
		uintptr(numCommandLists),
		uintptr(unsafe.Pointer(commandLists)),
	)
}

// Signal sets a fence to a specified value.
func (q *ID3D12CommandQueue) Signal(fence *ID3D12Fence, value uint64) error {
	ret, _, _ := syscall.Syscall(
		q.vtbl.Signal,
		3,
		uintptr(unsafe.Pointer(q)),
		uintptr(unsafe.Pointer(fence)),
		uintptr(value),
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// Wait waits on a fence.
func (q *ID3D12CommandQueue) Wait(fence *ID3D12Fence, value uint64) error {
	ret, _, _ := syscall.Syscall(
		q.vtbl.Wait,
		3,
		uintptr(unsafe.Pointer(q)),
		uintptr(unsafe.Pointer(fence)),
		uintptr(value),
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// GetTimestampFrequency gets the GPU timestamp counter frequency.
func (q *ID3D12CommandQueue) GetTimestampFrequency() (uint64, error) {
	var frequency uint64

	ret, _, _ := syscall.Syscall(
		q.vtbl.GetTimestampFrequency,
		2,
		uintptr(unsafe.Pointer(q)),
		uintptr(unsafe.Pointer(&frequency)),
		0,
	)

	if ret != 0 {
		return 0, HRESULTError(ret)
	}
	return frequency, nil
}

// GetDesc returns the command queue description.
func (q *ID3D12CommandQueue) GetDesc() D3D12_COMMAND_QUEUE_DESC {
	var desc D3D12_COMMAND_QUEUE_DESC

	_, _, _ = syscall.Syscall(
		q.vtbl.GetDesc,
		2,
		uintptr(unsafe.Pointer(&desc)),
		uintptr(unsafe.Pointer(q)),
		0,
	)

	return desc
}

// -----------------------------------------------------------------------------
// ID3D12CommandAllocator methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (a *ID3D12CommandAllocator) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		a.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(a)),
		0, 0,
	)
	return uint32(ret)
}

// Reset resets the command allocator.
func (a *ID3D12CommandAllocator) Reset() error {
	ret, _, _ := syscall.Syscall(
		a.vtbl.Reset,
		1,
		uintptr(unsafe.Pointer(a)),
		0, 0,
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// -----------------------------------------------------------------------------
// ID3D12GraphicsCommandList methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (c *ID3D12GraphicsCommandList) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		c.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(c)),
		0, 0,
	)
	return uint32(ret)
}

// Close closes the command list.
func (c *ID3D12GraphicsCommandList) Close() error {
	ret, _, _ := syscall.Syscall(
		c.vtbl.Close,
		1,
		uintptr(unsafe.Pointer(c)),
		0, 0,
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// Reset resets the command list.
func (c *ID3D12GraphicsCommandList) Reset(allocator *ID3D12CommandAllocator, initialState *ID3D12PipelineState) error {
	ret, _, _ := syscall.Syscall(
		c.vtbl.Reset,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(allocator)),
		uintptr(unsafe.Pointer(initialState)),
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// ClearState clears the command list state.
func (c *ID3D12GraphicsCommandList) ClearState(pipelineState *ID3D12PipelineState) {
	_, _, _ = syscall.Syscall(
		c.vtbl.ClearState,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(pipelineState)),
		0,
	)
}

// DrawInstanced draws non-indexed, instanced primitives.
func (c *ID3D12GraphicsCommandList) DrawInstanced(vertexCountPerInstance, instanceCount, startVertexLocation, startInstanceLocation uint32) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.DrawInstanced,
		5,
		uintptr(unsafe.Pointer(c)),
		uintptr(vertexCountPerInstance),
		uintptr(instanceCount),
		uintptr(startVertexLocation),
		uintptr(startInstanceLocation),
		0,
	)
}

// DrawIndexedInstanced draws indexed, instanced primitives.
func (c *ID3D12GraphicsCommandList) DrawIndexedInstanced(indexCountPerInstance, instanceCount, startIndexLocation uint32, baseVertexLocation int32, startInstanceLocation uint32) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.DrawIndexedInstanced,
		6,
		uintptr(unsafe.Pointer(c)),
		uintptr(indexCountPerInstance),
		uintptr(instanceCount),
		uintptr(startIndexLocation),
		uintptr(baseVertexLocation),
		uintptr(startInstanceLocation),
	)
}

// Dispatch dispatches a compute shader.
func (c *ID3D12GraphicsCommandList) Dispatch(threadGroupCountX, threadGroupCountY, threadGroupCountZ uint32) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.Dispatch,
		4,
		uintptr(unsafe.Pointer(c)),
		uintptr(threadGroupCountX),
		uintptr(threadGroupCountY),
		uintptr(threadGroupCountZ),
		0, 0,
	)
}

// CopyBufferRegion copies a region from one buffer to another.
func (c *ID3D12GraphicsCommandList) CopyBufferRegion(dstBuffer *ID3D12Resource, dstOffset uint64, srcBuffer *ID3D12Resource, srcOffset, numBytes uint64) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.CopyBufferRegion,
		6,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(dstBuffer)),
		uintptr(dstOffset),
		uintptr(unsafe.Pointer(srcBuffer)),
		uintptr(srcOffset),
		uintptr(numBytes),
	)
}

// CopyResource copies a resource.
func (c *ID3D12GraphicsCommandList) CopyResource(dstResource, srcResource *ID3D12Resource) {
	_, _, _ = syscall.Syscall(
		c.vtbl.CopyResource,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(dstResource)),
		uintptr(unsafe.Pointer(srcResource)),
	)
}

// IASetPrimitiveTopology sets the primitive topology.
func (c *ID3D12GraphicsCommandList) IASetPrimitiveTopology(topology D3D_PRIMITIVE_TOPOLOGY) {
	_, _, _ = syscall.Syscall(
		c.vtbl.IASetPrimitiveTopology,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(topology),
		0,
	)
}

// RSSetViewports sets the viewports.
func (c *ID3D12GraphicsCommandList) RSSetViewports(numViewports uint32, viewports *D3D12_VIEWPORT) {
	_, _, _ = syscall.Syscall(
		c.vtbl.RSSetViewports,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(numViewports),
		uintptr(unsafe.Pointer(viewports)),
	)
}

// RSSetScissorRects sets the scissor rectangles.
func (c *ID3D12GraphicsCommandList) RSSetScissorRects(numRects uint32, rects *D3D12_RECT) {
	_, _, _ = syscall.Syscall(
		c.vtbl.RSSetScissorRects,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(numRects),
		uintptr(unsafe.Pointer(rects)),
	)
}

// OMSetBlendFactor sets the blend factor.
func (c *ID3D12GraphicsCommandList) OMSetBlendFactor(blendFactor *[4]float32) {
	_, _, _ = syscall.Syscall(
		c.vtbl.OMSetBlendFactor,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(blendFactor)),
		0,
	)
}

// OMSetStencilRef sets the stencil reference value.
func (c *ID3D12GraphicsCommandList) OMSetStencilRef(stencilRef uint32) {
	_, _, _ = syscall.Syscall(
		c.vtbl.OMSetStencilRef,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(stencilRef),
		0,
	)
}

// SetPipelineState sets the pipeline state.
func (c *ID3D12GraphicsCommandList) SetPipelineState(pipelineState *ID3D12PipelineState) {
	_, _, _ = syscall.Syscall(
		c.vtbl.SetPipelineState,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(pipelineState)),
		0,
	)
}

// ResourceBarrier notifies the driver of resource state transitions.
func (c *ID3D12GraphicsCommandList) ResourceBarrier(numBarriers uint32, barriers *D3D12_RESOURCE_BARRIER) {
	_, _, _ = syscall.Syscall(
		c.vtbl.ResourceBarrier,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(numBarriers),
		uintptr(unsafe.Pointer(barriers)),
	)
}

// SetDescriptorHeaps sets descriptor heaps.
func (c *ID3D12GraphicsCommandList) SetDescriptorHeaps(numDescriptorHeaps uint32, descriptorHeaps **ID3D12DescriptorHeap) {
	_, _, _ = syscall.Syscall(
		c.vtbl.SetDescriptorHeaps,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(numDescriptorHeaps),
		uintptr(unsafe.Pointer(descriptorHeaps)),
	)
}

// SetComputeRootSignature sets the compute root signature.
func (c *ID3D12GraphicsCommandList) SetComputeRootSignature(rootSignature *ID3D12RootSignature) {
	_, _, _ = syscall.Syscall(
		c.vtbl.SetComputeRootSignature,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(rootSignature)),
		0,
	)
}

// SetGraphicsRootSignature sets the graphics root signature.
func (c *ID3D12GraphicsCommandList) SetGraphicsRootSignature(rootSignature *ID3D12RootSignature) {
	_, _, _ = syscall.Syscall(
		c.vtbl.SetGraphicsRootSignature,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(rootSignature)),
		0,
	)
}

// SetComputeRootDescriptorTable sets a compute descriptor table.
func (c *ID3D12GraphicsCommandList) SetComputeRootDescriptorTable(rootParameterIndex uint32, baseDescriptor D3D12_GPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall(
		c.vtbl.SetComputeRootDescriptorTable,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(rootParameterIndex),
		uintptr(baseDescriptor.Ptr),
	)
}

// SetGraphicsRootDescriptorTable sets a graphics descriptor table.
func (c *ID3D12GraphicsCommandList) SetGraphicsRootDescriptorTable(rootParameterIndex uint32, baseDescriptor D3D12_GPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall(
		c.vtbl.SetGraphicsRootDescriptorTable,
		3,
		uintptr(unsafe.Pointer(c)),
		uintptr(rootParameterIndex),
		uintptr(baseDescriptor.Ptr),
	)
}

// IASetIndexBuffer sets the index buffer.
func (c *ID3D12GraphicsCommandList) IASetIndexBuffer(view *D3D12_INDEX_BUFFER_VIEW) {
	_, _, _ = syscall.Syscall(
		c.vtbl.IASetIndexBuffer,
		2,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(view)),
		0,
	)
}

// IASetVertexBuffers sets vertex buffers.
func (c *ID3D12GraphicsCommandList) IASetVertexBuffers(startSlot, numViews uint32, views *D3D12_VERTEX_BUFFER_VIEW) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.IASetVertexBuffers,
		4,
		uintptr(unsafe.Pointer(c)),
		uintptr(startSlot),
		uintptr(numViews),
		uintptr(unsafe.Pointer(views)),
		0, 0,
	)
}

// OMSetRenderTargets sets render targets and depth stencil.
func (c *ID3D12GraphicsCommandList) OMSetRenderTargets(numRenderTargetDescriptors uint32, renderTargetDescriptors *D3D12_CPU_DESCRIPTOR_HANDLE, rtsSingleHandleToDescriptorRange int32, depthStencilDescriptor *D3D12_CPU_DESCRIPTOR_HANDLE) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.OMSetRenderTargets,
		5,
		uintptr(unsafe.Pointer(c)),
		uintptr(numRenderTargetDescriptors),
		uintptr(unsafe.Pointer(renderTargetDescriptors)),
		uintptr(rtsSingleHandleToDescriptorRange),
		uintptr(unsafe.Pointer(depthStencilDescriptor)),
		0,
	)
}

// ClearDepthStencilView clears a depth stencil view.
func (c *ID3D12GraphicsCommandList) ClearDepthStencilView(depthStencilView D3D12_CPU_DESCRIPTOR_HANDLE, clearFlags D3D12_CLEAR_FLAGS, depth float32, stencil uint8, numRects uint32, rects *D3D12_RECT) {
	_, _, _ = syscall.Syscall9(
		c.vtbl.ClearDepthStencilView,
		7,
		uintptr(unsafe.Pointer(c)),
		depthStencilView.Ptr,
		uintptr(clearFlags),
		uintptr(*(*uint32)(unsafe.Pointer(&depth))),
		uintptr(stencil),
		uintptr(numRects),
		uintptr(unsafe.Pointer(rects)),
		0, 0,
	)
}

// ClearRenderTargetView clears a render target view.
func (c *ID3D12GraphicsCommandList) ClearRenderTargetView(renderTargetView D3D12_CPU_DESCRIPTOR_HANDLE, colorRGBA *[4]float32, numRects uint32, rects *D3D12_RECT) {
	_, _, _ = syscall.Syscall6(
		c.vtbl.ClearRenderTargetView,
		5,
		uintptr(unsafe.Pointer(c)),
		renderTargetView.Ptr,
		uintptr(unsafe.Pointer(colorRGBA)),
		uintptr(numRects),
		uintptr(unsafe.Pointer(rects)),
		0,
	)
}

// -----------------------------------------------------------------------------
// ID3D12Fence methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (f *ID3D12Fence) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		f.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(f)),
		0, 0,
	)
	return uint32(ret)
}

// GetCompletedValue returns the current fence value.
func (f *ID3D12Fence) GetCompletedValue() uint64 {
	ret, _, _ := syscall.Syscall(
		f.vtbl.GetCompletedValue,
		1,
		uintptr(unsafe.Pointer(f)),
		0, 0,
	)
	return uint64(ret)
}

// SetEventOnCompletion sets an event to be signaled when the fence reaches a value.
func (f *ID3D12Fence) SetEventOnCompletion(value uint64, hEvent uintptr) error {
	ret, _, _ := syscall.Syscall(
		f.vtbl.SetEventOnCompletion,
		3,
		uintptr(unsafe.Pointer(f)),
		uintptr(value),
		hEvent,
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// Signal sets the fence to a value from the CPU side.
func (f *ID3D12Fence) Signal(value uint64) error {
	ret, _, _ := syscall.Syscall(
		f.vtbl.Signal,
		2,
		uintptr(unsafe.Pointer(f)),
		uintptr(value),
		0,
	)

	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// -----------------------------------------------------------------------------
// ID3D12Resource methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (r *ID3D12Resource) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		r.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(r)),
		0, 0,
	)
	return uint32(ret)
}

// Map maps a subresource to CPU memory.
func (r *ID3D12Resource) Map(subresource uint32, readRange *D3D12_RANGE) (unsafe.Pointer, error) {
	var data unsafe.Pointer

	ret, _, _ := syscall.Syscall6(
		r.vtbl.Map,
		4,
		uintptr(unsafe.Pointer(r)),
		uintptr(subresource),
		uintptr(unsafe.Pointer(readRange)),
		uintptr(unsafe.Pointer(&data)),
		0, 0,
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return data, nil
}

// Unmap unmaps a subresource.
func (r *ID3D12Resource) Unmap(subresource uint32, writtenRange *D3D12_RANGE) {
	_, _, _ = syscall.Syscall(
		r.vtbl.Unmap,
		3,
		uintptr(unsafe.Pointer(r)),
		uintptr(subresource),
		uintptr(unsafe.Pointer(writtenRange)),
	)
}

// GetGPUVirtualAddress returns the GPU virtual address of the resource.
func (r *ID3D12Resource) GetGPUVirtualAddress() uint64 {
	ret, _, _ := syscall.Syscall(
		r.vtbl.GetGPUVirtualAddress,
		1,
		uintptr(unsafe.Pointer(r)),
		0, 0,
	)
	return uint64(ret)
}

// GetDesc returns the resource description.
func (r *ID3D12Resource) GetDesc() D3D12_RESOURCE_DESC {
	var desc D3D12_RESOURCE_DESC

	_, _, _ = syscall.Syscall(
		r.vtbl.GetDesc,
		2,
		uintptr(unsafe.Pointer(&desc)),
		uintptr(unsafe.Pointer(r)),
		0,
	)

	return desc
}

// -----------------------------------------------------------------------------
// ID3D12DescriptorHeap methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (h *ID3D12DescriptorHeap) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		h.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(h)),
		0, 0,
	)
	return uint32(ret)
}

// GetCPUDescriptorHandleForHeapStart returns the CPU handle for the start of the heap.
func (h *ID3D12DescriptorHeap) GetCPUDescriptorHandleForHeapStart() D3D12_CPU_DESCRIPTOR_HANDLE {
	var handle D3D12_CPU_DESCRIPTOR_HANDLE

	_, _, _ = syscall.Syscall(
		h.vtbl.GetCPUDescriptorHandleForHeapStart,
		2,
		uintptr(unsafe.Pointer(&handle)),
		uintptr(unsafe.Pointer(h)),
		0,
	)

	return handle
}

// GetGPUDescriptorHandleForHeapStart returns the GPU handle for the start of the heap.
func (h *ID3D12DescriptorHeap) GetGPUDescriptorHandleForHeapStart() D3D12_GPU_DESCRIPTOR_HANDLE {
	var handle D3D12_GPU_DESCRIPTOR_HANDLE

	_, _, _ = syscall.Syscall(
		h.vtbl.GetGPUDescriptorHandleForHeapStart,
		2,
		uintptr(unsafe.Pointer(&handle)),
		uintptr(unsafe.Pointer(h)),
		0,
	)

	return handle
}

// GetDesc returns the descriptor heap description.
func (h *ID3D12DescriptorHeap) GetDesc() D3D12_DESCRIPTOR_HEAP_DESC {
	var desc D3D12_DESCRIPTOR_HEAP_DESC

	_, _, _ = syscall.Syscall(
		h.vtbl.GetDesc,
		2,
		uintptr(unsafe.Pointer(&desc)),
		uintptr(unsafe.Pointer(h)),
		0,
	)

	return desc
}

// -----------------------------------------------------------------------------
// ID3D12PipelineState methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (p *ID3D12PipelineState) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		p.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(p)),
		0, 0,
	)
	return uint32(ret)
}

// -----------------------------------------------------------------------------
// ID3D12RootSignature methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (s *ID3D12RootSignature) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// -----------------------------------------------------------------------------
// ID3D12Heap methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (h *ID3D12Heap) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		h.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(h)),
		0, 0,
	)
	return uint32(ret)
}

// -----------------------------------------------------------------------------
// ID3D12QueryHeap methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (h *ID3D12QueryHeap) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		h.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(h)),
		0, 0,
	)
	return uint32(ret)
}

// -----------------------------------------------------------------------------
// ID3D12CommandSignature methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (s *ID3D12CommandSignature) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// -----------------------------------------------------------------------------
// ID3D12Debug methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (d *ID3D12Debug) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// EnableDebugLayer enables the debug layer.
func (d *ID3D12Debug) EnableDebugLayer() {
	_, _, _ = syscall.Syscall(
		d.vtbl.EnableDebugLayer,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
}

// -----------------------------------------------------------------------------
// ID3D12Debug1 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (d *ID3D12Debug1) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// EnableDebugLayer enables the debug layer.
func (d *ID3D12Debug1) EnableDebugLayer() {
	_, _, _ = syscall.Syscall(
		d.vtbl.EnableDebugLayer,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
}

// SetEnableGPUBasedValidation enables or disables GPU-based validation.
func (d *ID3D12Debug1) SetEnableGPUBasedValidation(enable bool) {
	var enableInt uintptr
	if enable {
		enableInt = 1
	}
	_, _, _ = syscall.Syscall(
		d.vtbl.SetEnableGPUBasedValidation,
		2,
		uintptr(unsafe.Pointer(d)),
		enableInt,
		0,
	)
}

// SetEnableSynchronizedCommandQueueValidation enables or disables synchronized command queue validation.
func (d *ID3D12Debug1) SetEnableSynchronizedCommandQueueValidation(enable bool) {
	var enableInt uintptr
	if enable {
		enableInt = 1
	}
	_, _, _ = syscall.Syscall(
		d.vtbl.SetEnableSynchronizedCommandQueueValidation,
		2,
		uintptr(unsafe.Pointer(d)),
		enableInt,
		0,
	)
}

// -----------------------------------------------------------------------------
// ID3D12Debug3 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (d *ID3D12Debug3) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// EnableDebugLayer enables the debug layer.
func (d *ID3D12Debug3) EnableDebugLayer() {
	_, _, _ = syscall.Syscall(
		d.vtbl.EnableDebugLayer,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
}

// SetEnableGPUBasedValidation enables or disables GPU-based validation.
func (d *ID3D12Debug3) SetEnableGPUBasedValidation(enable bool) {
	var enableInt uintptr
	if enable {
		enableInt = 1
	}
	_, _, _ = syscall.Syscall(
		d.vtbl.SetEnableGPUBasedValidation,
		2,
		uintptr(unsafe.Pointer(d)),
		enableInt,
		0,
	)
}

// -----------------------------------------------------------------------------
// ID3DBlob methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (b *ID3DBlob) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		b.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(b)),
		0, 0,
	)
	return uint32(ret)
}

// GetBufferPointer returns a pointer to the blob data.
func (b *ID3DBlob) GetBufferPointer() unsafe.Pointer {
	var ptr unsafe.Pointer
	ret, _, _ := syscall.Syscall(
		b.vtbl.GetBufferPointer,
		1,
		uintptr(unsafe.Pointer(b)),
		0, 0,
	)
	// Store the return value as uintptr and convert via intermediate variable
	// to satisfy go vet. The returned pointer is valid for the lifetime of the blob.
	*(*uintptr)(unsafe.Pointer(&ptr)) = ret
	return ptr
}

// GetBufferSize returns the size of the blob data.
func (b *ID3DBlob) GetBufferSize() uintptr {
	ret, _, _ := syscall.Syscall(
		b.vtbl.GetBufferSize,
		1,
		uintptr(unsafe.Pointer(b)),
		0, 0,
	)
	return ret
}
