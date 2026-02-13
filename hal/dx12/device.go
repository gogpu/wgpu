// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/hlsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/d3dcompile"
	"golang.org/x/sys/windows"
)

// Device implements hal.Device for DirectX 12.
// It manages the D3D12 device, command queue, descriptor heaps, and synchronization.
type Device struct {
	raw      *d3d12.ID3D12Device
	instance *Instance

	// Command queue for graphics/compute operations.
	directQueue *d3d12.ID3D12CommandQueue

	// Descriptor heaps (shared for all resources).
	viewHeap    *DescriptorHeap // CBV/SRV/UAV
	samplerHeap *DescriptorHeap // Samplers
	rtvHeap     *DescriptorHeap // Render Target Views
	dsvHeap     *DescriptorHeap // Depth Stencil Views

	// GPU synchronization.
	fence      *d3d12.ID3D12Fence
	fenceValue uint64
	fenceEvent windows.Handle
	fenceMu    sync.Mutex

	// Feature level and capabilities.
	featureLevel d3d12.D3D_FEATURE_LEVEL

	// Shared empty root signature for pipelines without bind groups.
	// DX12 requires a valid root signature for every PSO, even if the shader
	// has no resource bindings. This is lazily created on first use and shared
	// across all pipelines that don't provide an explicit PipelineLayout.
	emptyRootSignature *d3d12.ID3D12RootSignature
}

// DescriptorHeap wraps a D3D12 descriptor heap with allocation tracking.
type DescriptorHeap struct {
	raw           *d3d12.ID3D12DescriptorHeap
	heapType      d3d12.D3D12_DESCRIPTOR_HEAP_TYPE
	cpuStart      d3d12.D3D12_CPU_DESCRIPTOR_HANDLE
	gpuStart      d3d12.D3D12_GPU_DESCRIPTOR_HANDLE
	incrementSize uint32
	capacity      uint32
	nextFree      uint32
	mu            sync.Mutex
}

// Allocate allocates descriptors from the heap.
// Returns the CPU handle for the first allocated descriptor.
func (h *DescriptorHeap) Allocate(count uint32) (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.nextFree+count > h.capacity {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, fmt.Errorf("dx12: descriptor heap exhausted")
	}

	handle := h.cpuStart.Offset(int(h.nextFree), h.incrementSize)
	h.nextFree += count
	return handle, nil
}

// AllocateGPU allocates descriptors and returns both CPU and GPU handles.
func (h *DescriptorHeap) AllocateGPU(count uint32) (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, d3d12.D3D12_GPU_DESCRIPTOR_HANDLE, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.nextFree+count > h.capacity {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, d3d12.D3D12_GPU_DESCRIPTOR_HANDLE{},
			fmt.Errorf("dx12: descriptor heap exhausted")
	}

	cpuHandle := h.cpuStart.Offset(int(h.nextFree), h.incrementSize)
	gpuHandle := h.gpuStart.Offset(int(h.nextFree), h.incrementSize)
	h.nextFree += count
	return cpuHandle, gpuHandle, nil
}

// newDevice creates a new DX12 device from a DXGI adapter.
// adapterPtr is the IUnknown pointer to the DXGI adapter.
func newDevice(instance *Instance, adapterPtr unsafe.Pointer, featureLevel d3d12.D3D_FEATURE_LEVEL) (*Device, error) {
	// Create D3D12 device
	rawDevice, err := instance.d3d12Lib.CreateDevice(adapterPtr, featureLevel)
	if err != nil {
		return nil, fmt.Errorf("dx12: D3D12CreateDevice failed: %w", err)
	}

	dev := &Device{
		raw:          rawDevice,
		instance:     instance,
		featureLevel: featureLevel,
	}

	// Create the direct (graphics) command queue
	if err := dev.createCommandQueue(); err != nil {
		rawDevice.Release()
		return nil, err
	}

	// Create descriptor heaps
	if err := dev.createDescriptorHeaps(); err != nil {
		dev.cleanup()
		return nil, err
	}

	// Create the fence for GPU synchronization
	if err := dev.createFence(); err != nil {
		dev.cleanup()
		return nil, err
	}

	// Set a finalizer to ensure cleanup
	runtime.SetFinalizer(dev, (*Device).Destroy)

	return dev, nil
}

// createCommandQueue creates the direct (graphics) command queue.
func (d *Device) createCommandQueue() error {
	desc := d3d12.D3D12_COMMAND_QUEUE_DESC{
		Type:     d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT,
		Priority: 0, // D3D12_COMMAND_QUEUE_PRIORITY_NORMAL
		Flags:    d3d12.D3D12_COMMAND_QUEUE_FLAG_NONE,
		NodeMask: 0,
	}

	queue, err := d.raw.CreateCommandQueue(&desc)
	if err != nil {
		return fmt.Errorf("dx12: CreateCommandQueue failed: %w", err)
	}

	d.directQueue = queue
	return nil
}

// createDescriptorHeaps creates the descriptor heaps for various resource gputypes.
func (d *Device) createDescriptorHeaps() error {
	var err error

	// CBV/SRV/UAV heap (shader visible)
	d.viewHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_CBV_SRV_UAV,
		1024*1024, // 1M descriptors for bindless
		true,      // shader visible
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create CBV/SRV/UAV heap: %w", err)
	}

	// Sampler heap (shader visible)
	d.samplerHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_SAMPLER,
		2048,
		true,
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create sampler heap: %w", err)
	}

	// RTV heap (not shader visible)
	d.rtvHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_RTV,
		256,
		false,
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create RTV heap: %w", err)
	}

	// DSV heap (not shader visible)
	d.dsvHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_DSV,
		64,
		false,
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create DSV heap: %w", err)
	}

	return nil
}

// createHeap creates a single descriptor heap.
func (d *Device) createHeap(heapType d3d12.D3D12_DESCRIPTOR_HEAP_TYPE, numDescriptors uint32, shaderVisible bool) (*DescriptorHeap, error) {
	var flags d3d12.D3D12_DESCRIPTOR_HEAP_FLAGS
	if shaderVisible {
		flags = d3d12.D3D12_DESCRIPTOR_HEAP_FLAG_SHADER_VISIBLE
	}

	desc := d3d12.D3D12_DESCRIPTOR_HEAP_DESC{
		Type:           heapType,
		NumDescriptors: numDescriptors,
		Flags:          flags,
		NodeMask:       0,
	}

	rawHeap, err := d.raw.CreateDescriptorHeap(&desc)
	if err != nil {
		return nil, err
	}

	heap := &DescriptorHeap{
		raw:           rawHeap,
		heapType:      heapType,
		cpuStart:      rawHeap.GetCPUDescriptorHandleForHeapStart(),
		incrementSize: d.raw.GetDescriptorHandleIncrementSize(heapType),
		capacity:      numDescriptors,
		nextFree:      0,
	}

	if shaderVisible {
		heap.gpuStart = rawHeap.GetGPUDescriptorHandleForHeapStart()
	}

	return heap, nil
}

// createFence creates the GPU synchronization fence.
func (d *Device) createFence() error {
	fence, err := d.raw.CreateFence(0, d3d12.D3D12_FENCE_FLAG_NONE)
	if err != nil {
		return fmt.Errorf("dx12: CreateFence failed: %w", err)
	}

	// Create Windows event for fence signaling
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		fence.Release()
		return fmt.Errorf("dx12: CreateEvent failed: %w", err)
	}

	d.fence = fence
	d.fenceEvent = event
	d.fenceValue = 0
	return nil
}

// waitForGPU blocks until all GPU work completes.
func (d *Device) waitForGPU() error {
	d.fenceMu.Lock()
	defer d.fenceMu.Unlock()

	d.fenceValue++
	targetValue := d.fenceValue

	// Signal the fence from the GPU
	if err := d.directQueue.Signal(d.fence, targetValue); err != nil {
		return fmt.Errorf("dx12: queue Signal failed: %w", err)
	}

	// Wait for the GPU to reach the fence value
	if d.fence.GetCompletedValue() < targetValue {
		if err := d.fence.SetEventOnCompletion(targetValue, uintptr(d.fenceEvent)); err != nil {
			return fmt.Errorf("dx12: SetEventOnCompletion failed: %w", err)
		}
		_, err := windows.WaitForSingleObject(d.fenceEvent, windows.INFINITE)
		if err != nil {
			return fmt.Errorf("dx12: WaitForSingleObject failed: %w", err)
		}
	}

	return nil
}

// getOrCreateEmptyRootSignature returns a shared empty root signature for
// pipelines that have no resource bindings (no PipelineLayout).
// DX12 requires a valid root signature for every PSO — even with zero parameters.
// Created lazily on first use, reused for all such pipelines on this device.
func (d *Device) getOrCreateEmptyRootSignature() (*d3d12.ID3D12RootSignature, error) {
	if d.emptyRootSignature != nil {
		return d.emptyRootSignature, nil
	}

	// Create root signature with zero parameters (only the IA input layout flag).
	desc := d3d12.D3D12_ROOT_SIGNATURE_DESC{
		Flags: d3d12.D3D12_ROOT_SIGNATURE_FLAG_ALLOW_INPUT_ASSEMBLER_INPUT_LAYOUT,
	}

	blob, errorBlob, err := d.instance.d3d12Lib.SerializeRootSignature(&desc, d3d12.D3D_ROOT_SIGNATURE_VERSION_1_0)
	if err != nil {
		if errorBlob != nil {
			errorBlob.Release()
		}
		return nil, fmt.Errorf("dx12: failed to serialize empty root signature: %w", err)
	}
	defer blob.Release()

	rootSig, err := d.raw.CreateRootSignature(0, blob.GetBufferPointer(), blob.GetBufferSize())
	if err != nil {
		return nil, fmt.Errorf("dx12: failed to create empty root signature: %w", err)
	}

	d.emptyRootSignature = rootSig
	return rootSig, nil
}

// cleanup releases all device resources without clearing the finalizer.
func (d *Device) cleanup() {
	if d.emptyRootSignature != nil {
		d.emptyRootSignature.Release()
		d.emptyRootSignature = nil
	}

	if d.fenceEvent != 0 {
		_ = windows.CloseHandle(d.fenceEvent)
		d.fenceEvent = 0
	}

	if d.fence != nil {
		d.fence.Release()
		d.fence = nil
	}

	if d.viewHeap != nil && d.viewHeap.raw != nil {
		d.viewHeap.raw.Release()
		d.viewHeap = nil
	}
	if d.samplerHeap != nil && d.samplerHeap.raw != nil {
		d.samplerHeap.raw.Release()
		d.samplerHeap = nil
	}
	if d.rtvHeap != nil && d.rtvHeap.raw != nil {
		d.rtvHeap.raw.Release()
		d.rtvHeap = nil
	}
	if d.dsvHeap != nil && d.dsvHeap.raw != nil {
		d.dsvHeap.raw.Release()
		d.dsvHeap = nil
	}

	if d.directQueue != nil {
		d.directQueue.Release()
		d.directQueue = nil
	}

	if d.raw != nil {
		d.raw.Release()
		d.raw = nil
	}
}

// -----------------------------------------------------------------------------
// Descriptor Allocation Helpers
// -----------------------------------------------------------------------------

// allocateRTVDescriptor allocates a render target view descriptor.
func (d *Device) allocateRTVDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	if d.rtvHeap == nil {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: RTV heap not initialized")
	}

	d.rtvHeap.mu.Lock()
	defer d.rtvHeap.mu.Unlock()

	if d.rtvHeap.nextFree >= d.rtvHeap.capacity {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: RTV heap exhausted")
	}

	index := d.rtvHeap.nextFree
	handle := d.rtvHeap.cpuStart.Offset(int(index), d.rtvHeap.incrementSize)
	d.rtvHeap.nextFree++
	return handle, index, nil
}

// allocateDSVDescriptor allocates a depth stencil view descriptor.
func (d *Device) allocateDSVDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	if d.dsvHeap == nil {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: DSV heap not initialized")
	}

	d.dsvHeap.mu.Lock()
	defer d.dsvHeap.mu.Unlock()

	if d.dsvHeap.nextFree >= d.dsvHeap.capacity {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: DSV heap exhausted")
	}

	index := d.dsvHeap.nextFree
	handle := d.dsvHeap.cpuStart.Offset(int(index), d.dsvHeap.incrementSize)
	d.dsvHeap.nextFree++
	return handle, index, nil
}

// allocateSRVDescriptor allocates a shader resource view descriptor.
func (d *Device) allocateSRVDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	if d.viewHeap == nil {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: SRV heap not initialized")
	}

	d.viewHeap.mu.Lock()
	defer d.viewHeap.mu.Unlock()

	if d.viewHeap.nextFree >= d.viewHeap.capacity {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: CBV/SRV/UAV heap exhausted")
	}

	index := d.viewHeap.nextFree
	handle := d.viewHeap.cpuStart.Offset(int(index), d.viewHeap.incrementSize)
	d.viewHeap.nextFree++
	return handle, index, nil
}

// allocateSamplerDescriptor allocates a sampler descriptor.
func (d *Device) allocateSamplerDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	if d.samplerHeap == nil {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: sampler heap not initialized")
	}

	d.samplerHeap.mu.Lock()
	defer d.samplerHeap.mu.Unlock()

	if d.samplerHeap.nextFree >= d.samplerHeap.capacity {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: sampler heap exhausted")
	}

	index := d.samplerHeap.nextFree
	handle := d.samplerHeap.cpuStart.Offset(int(index), d.samplerHeap.incrementSize)
	d.samplerHeap.nextFree++
	return handle, index, nil
}

// -----------------------------------------------------------------------------
// hal.Device interface implementation
// -----------------------------------------------------------------------------

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: buffer descriptor is nil")
	}

	if desc.Size == 0 {
		return nil, fmt.Errorf("dx12: buffer size must be > 0")
	}

	// Determine heap type based on usage
	var heapType d3d12.D3D12_HEAP_TYPE
	var initialState d3d12.D3D12_RESOURCE_STATES

	switch {
	case desc.Usage&gputypes.BufferUsageMapRead != 0:
		// Readback buffer
		heapType = d3d12.D3D12_HEAP_TYPE_READBACK
		initialState = d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	case desc.Usage&gputypes.BufferUsageMapWrite != 0 || desc.MappedAtCreation:
		// Upload buffer
		heapType = d3d12.D3D12_HEAP_TYPE_UPLOAD
		initialState = d3d12.D3D12_RESOURCE_STATE_GENERIC_READ
	default:
		// Default (GPU-only) buffer
		heapType = d3d12.D3D12_HEAP_TYPE_DEFAULT
		initialState = d3d12.D3D12_RESOURCE_STATE_COMMON
	}

	// Align size for constant buffers (256-byte alignment required)
	bufferSize := desc.Size
	if desc.Usage&gputypes.BufferUsageUniform != 0 {
		bufferSize = alignTo256(bufferSize)
	}

	// Build resource flags
	var resourceFlags d3d12.D3D12_RESOURCE_FLAGS
	if desc.Usage&gputypes.BufferUsageStorage != 0 {
		resourceFlags |= d3d12.D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS
	}

	// Create heap properties
	heapProps := d3d12.D3D12_HEAP_PROPERTIES{
		Type:                 heapType,
		CPUPageProperty:      d3d12.D3D12_CPU_PAGE_PROPERTY_UNKNOWN,
		MemoryPoolPreference: d3d12.D3D12_MEMORY_POOL_UNKNOWN,
		CreationNodeMask:     0,
		VisibleNodeMask:      0,
	}

	// Create resource description for buffer
	resourceDesc := d3d12.D3D12_RESOURCE_DESC{
		Dimension:        d3d12.D3D12_RESOURCE_DIMENSION_BUFFER,
		Alignment:        0,
		Width:            bufferSize,
		Height:           1,
		DepthOrArraySize: 1,
		MipLevels:        1,
		Format:           d3d12.DXGI_FORMAT_UNKNOWN,
		SampleDesc:       d3d12.DXGI_SAMPLE_DESC{Count: 1, Quality: 0},
		Layout:           d3d12.D3D12_TEXTURE_LAYOUT_ROW_MAJOR,
		Flags:            resourceFlags,
	}

	// Create the committed resource
	resource, err := d.raw.CreateCommittedResource(
		&heapProps,
		d3d12.D3D12_HEAP_FLAG_NONE,
		&resourceDesc,
		initialState,
		nil, // No optimized clear value for buffers
	)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateCommittedResource failed: %w", err)
	}

	buffer := &Buffer{
		raw:      resource,
		size:     desc.Size, // Return original size, not aligned size
		usage:    desc.Usage,
		heapType: heapType,
		gpuVA:    resource.GetGPUVirtualAddress(),
		device:   d,
	}

	// Map at creation if requested
	if desc.MappedAtCreation {
		ptr, mapErr := buffer.Map(0, desc.Size)
		if mapErr != nil {
			resource.Release()
			return nil, fmt.Errorf("dx12: failed to map buffer at creation: %w", mapErr)
		}
		buffer.mappedPointer = ptr
	}

	return buffer, nil
}

// DestroyBuffer destroys a GPU buffer.
func (d *Device) DestroyBuffer(buffer hal.Buffer) {
	if b, ok := buffer.(*Buffer); ok && b != nil {
		b.Destroy()
	}
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: texture descriptor is nil")
	}

	if desc.Size.Width == 0 || desc.Size.Height == 0 {
		return nil, fmt.Errorf("dx12: texture size must be > 0")
	}

	// Convert format
	dxgiFormat := textureFormatToD3D12(desc.Format)
	if dxgiFormat == d3d12.DXGI_FORMAT_UNKNOWN {
		return nil, fmt.Errorf("dx12: unsupported texture format: %d", desc.Format)
	}

	// Build resource flags based on usage
	var resourceFlags d3d12.D3D12_RESOURCE_FLAGS
	if desc.Usage&gputypes.TextureUsageRenderAttachment != 0 {
		if isDepthFormat(desc.Format) {
			resourceFlags |= d3d12.D3D12_RESOURCE_FLAG_ALLOW_DEPTH_STENCIL
		} else {
			resourceFlags |= d3d12.D3D12_RESOURCE_FLAG_ALLOW_RENDER_TARGET
		}
	}
	if desc.Usage&gputypes.TextureUsageStorageBinding != 0 {
		resourceFlags |= d3d12.D3D12_RESOURCE_FLAG_ALLOW_UNORDERED_ACCESS
	}

	// Determine depth/array size
	depthOrArraySize := desc.Size.DepthOrArrayLayers
	if depthOrArraySize == 0 {
		depthOrArraySize = 1
	}

	// Mip levels
	mipLevels := desc.MipLevelCount
	if mipLevels == 0 {
		mipLevels = 1
	}

	// Sample count
	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}

	// For depth formats, we may need to use typeless format for SRV compatibility
	createFormat := dxgiFormat
	if isDepthFormat(desc.Format) && (desc.Usage&gputypes.TextureUsageTextureBinding != 0) {
		// Use typeless format to allow both DSV and SRV
		createFormat = depthFormatToTypeless(desc.Format)
		if createFormat == d3d12.DXGI_FORMAT_UNKNOWN {
			createFormat = dxgiFormat
		}
	}

	// Use optimal texture layout for all dimensions - let driver choose
	layout := d3d12.D3D12_TEXTURE_LAYOUT_UNKNOWN

	// Create resource description
	resourceDesc := d3d12.D3D12_RESOURCE_DESC{
		Dimension:        textureDimensionToD3D12(desc.Dimension),
		Alignment:        0,
		Width:            uint64(desc.Size.Width),
		Height:           desc.Size.Height,
		DepthOrArraySize: uint16(depthOrArraySize),
		MipLevels:        uint16(mipLevels),
		Format:           createFormat,
		SampleDesc:       d3d12.DXGI_SAMPLE_DESC{Count: sampleCount, Quality: 0},
		Layout:           layout,
		Flags:            resourceFlags,
	}

	// Heap properties (default heap for GPU textures)
	heapProps := d3d12.D3D12_HEAP_PROPERTIES{
		Type:                 d3d12.D3D12_HEAP_TYPE_DEFAULT,
		CPUPageProperty:      d3d12.D3D12_CPU_PAGE_PROPERTY_UNKNOWN,
		MemoryPoolPreference: d3d12.D3D12_MEMORY_POOL_UNKNOWN,
		CreationNodeMask:     0,
		VisibleNodeMask:      0,
	}

	// Determine initial state based on primary usage
	var initialState d3d12.D3D12_RESOURCE_STATES
	switch {
	case desc.Usage&gputypes.TextureUsageRenderAttachment != 0 && isDepthFormat(desc.Format):
		initialState = d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE
	case desc.Usage&gputypes.TextureUsageRenderAttachment != 0:
		initialState = d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET
	case desc.Usage&gputypes.TextureUsageCopyDst != 0:
		initialState = d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	default:
		initialState = d3d12.D3D12_RESOURCE_STATE_COMMON
	}

	// Optimized clear value for render targets/depth stencil
	var clearValue *d3d12.D3D12_CLEAR_VALUE
	if desc.Usage&gputypes.TextureUsageRenderAttachment != 0 {
		cv := d3d12.D3D12_CLEAR_VALUE{
			Format: dxgiFormat,
		}
		if isDepthFormat(desc.Format) {
			cv.SetDepthStencil(1.0, 0)
		} else {
			cv.SetColor([4]float32{0, 0, 0, 0})
		}
		clearValue = &cv
	}

	// Create the committed resource
	resource, err := d.raw.CreateCommittedResource(
		&heapProps,
		d3d12.D3D12_HEAP_FLAG_NONE,
		&resourceDesc,
		initialState,
		clearValue,
	)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateCommittedResource for texture failed: %w", err)
	}

	return &Texture{
		raw:       resource,
		format:    desc.Format,
		dimension: desc.Dimension,
		size: hal.Extent3D{
			Width:              desc.Size.Width,
			Height:             desc.Size.Height,
			DepthOrArrayLayers: depthOrArraySize,
		},
		mipLevels: mipLevels,
		samples:   sampleCount,
		usage:     desc.Usage,
		device:    d,
	}, nil
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(texture hal.Texture) {
	if t, ok := texture.(*Texture); ok && t != nil {
		t.Destroy()
	}
}

// CreateTextureView creates a view into a texture.
//
//nolint:maintidx // inherent D3D12 complexity: one WebGPU view → RTV + DSV + SRV descriptors
func (d *Device) CreateTextureView(texture hal.Texture, desc *hal.TextureViewDescriptor) (hal.TextureView, error) {
	if texture == nil {
		return nil, fmt.Errorf("dx12: texture is nil")
	}

	// Handle SurfaceTexture (swapchain back buffer) — return a lightweight view
	// with the pre-existing RTV handle, similar to Vulkan's SwapchainTexture path.
	if st, ok := texture.(*SurfaceTexture); ok {
		return &TextureView{
			texture: &Texture{
				raw:        st.resource,
				format:     st.format,
				dimension:  gputypes.TextureDimension2D,
				size:       hal.Extent3D{Width: st.width, Height: st.height, DepthOrArrayLayers: 1},
				mipLevels:  1,
				isExternal: true,
			},
			format:     st.format,
			dimension:  gputypes.TextureViewDimension2D,
			baseMip:    0,
			mipCount:   1,
			baseLayer:  0,
			layerCount: 1,
			device:     d,
			rtvHandle:  st.rtvHandle,
			hasRTV:     true,
		}, nil
	}

	tex, ok := texture.(*Texture)
	if !ok {
		return nil, fmt.Errorf("dx12: texture is not a DX12 texture")
	}

	// Determine view format
	viewFormat := tex.format
	if desc != nil && desc.Format != gputypes.TextureFormatUndefined {
		viewFormat = desc.Format
	}

	// Determine view dimension
	viewDim := gputypes.TextureViewDimension2D // Default
	if desc != nil && desc.Dimension != gputypes.TextureViewDimensionUndefined {
		viewDim = desc.Dimension
	} else {
		// Infer from texture dimension
		switch tex.dimension {
		case gputypes.TextureDimension1D:
			viewDim = gputypes.TextureViewDimension1D
		case gputypes.TextureDimension2D:
			viewDim = gputypes.TextureViewDimension2D
		case gputypes.TextureDimension3D:
			viewDim = gputypes.TextureViewDimension3D
		}
	}

	// Determine mip range
	baseMip := uint32(0)
	mipCount := tex.mipLevels
	if desc != nil {
		baseMip = desc.BaseMipLevel
		if desc.MipLevelCount > 0 {
			mipCount = desc.MipLevelCount
		} else {
			mipCount = tex.mipLevels - baseMip
		}
	}

	// Determine array layer range
	baseLayer := uint32(0)
	layerCount := tex.size.DepthOrArrayLayers
	if desc != nil {
		baseLayer = desc.BaseArrayLayer
		if desc.ArrayLayerCount > 0 {
			layerCount = desc.ArrayLayerCount
		} else {
			layerCount = tex.size.DepthOrArrayLayers - baseLayer
		}
	}

	view := &TextureView{
		texture:    tex,
		format:     viewFormat,
		dimension:  viewDim,
		baseMip:    baseMip,
		mipCount:   mipCount,
		baseLayer:  baseLayer,
		layerCount: layerCount,
		device:     d,
	}

	dxgiFormat := textureFormatToD3D12(viewFormat)

	// Create RTV if texture supports render attachment and is not depth
	if tex.usage&gputypes.TextureUsageRenderAttachment != 0 && !isDepthFormat(viewFormat) {
		// Allocate RTV descriptor
		rtvHandle, rtvIndex, err := d.allocateRTVDescriptor()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to allocate RTV descriptor: %w", err)
		}

		// Create RTV desc
		rtvDesc := d3d12.D3D12_RENDER_TARGET_VIEW_DESC{
			Format:        dxgiFormat,
			ViewDimension: textureViewDimensionToRTV(viewDim),
		}

		// Set up dimension-specific fields
		switch viewDim {
		case gputypes.TextureViewDimension1D:
			rtvDesc.SetTexture1D(baseMip)
		case gputypes.TextureViewDimension2D:
			rtvDesc.SetTexture2D(baseMip, 0)
		case gputypes.TextureViewDimension2DArray:
			rtvDesc.SetTexture2DArray(baseMip, baseLayer, layerCount, 0)
		case gputypes.TextureViewDimension3D:
			rtvDesc.SetTexture3D(baseMip, baseLayer, layerCount)
		}

		d.raw.CreateRenderTargetView(tex.raw, &rtvDesc, rtvHandle)
		view.rtvHandle = rtvHandle
		view.rtvHeapIndex = rtvIndex
		view.hasRTV = true
	}

	// Create DSV if texture supports render attachment and is depth
	if tex.usage&gputypes.TextureUsageRenderAttachment != 0 && isDepthFormat(viewFormat) {
		// Allocate DSV descriptor
		dsvHandle, dsvIndex, err := d.allocateDSVDescriptor()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to allocate DSV descriptor: %w", err)
		}

		// For depth views, use the actual depth format, not typeless
		depthFormat := textureFormatToD3D12(viewFormat)

		// Create DSV desc
		dsvDesc := d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC{
			Format:        depthFormat,
			ViewDimension: textureViewDimensionToDSV(viewDim),
			Flags:         0,
		}

		// Set up dimension-specific fields
		switch viewDim {
		case gputypes.TextureViewDimension1D:
			dsvDesc.SetTexture1D(baseMip)
		case gputypes.TextureViewDimension2D:
			dsvDesc.SetTexture2D(baseMip)
		case gputypes.TextureViewDimension2DArray:
			dsvDesc.SetTexture2DArray(baseMip, baseLayer, layerCount)
		}

		d.raw.CreateDepthStencilView(tex.raw, &dsvDesc, dsvHandle)
		view.dsvHandle = dsvHandle
		view.dsvHeapIndex = dsvIndex
		view.hasDSV = true
	}

	// Create SRV if texture supports texture binding
	if tex.usage&gputypes.TextureUsageTextureBinding != 0 {
		// Allocate SRV descriptor
		srvHandle, srvIndex, err := d.allocateSRVDescriptor()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to allocate SRV descriptor: %w", err)
		}

		// For depth textures, use SRV-compatible format
		srvFormat := dxgiFormat
		if isDepthFormat(viewFormat) {
			srvFormat = depthFormatToSRV(viewFormat)
		}

		// Create SRV desc
		srvDesc := d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC{
			Format:                  srvFormat,
			ViewDimension:           textureViewDimensionToSRV(viewDim),
			Shader4ComponentMapping: d3d12.D3D12_DEFAULT_SHADER_4_COMPONENT_MAPPING,
		}

		// Set up dimension-specific fields
		switch viewDim {
		case gputypes.TextureViewDimension1D:
			srvDesc.SetTexture1D(baseMip, mipCount, 0)
		case gputypes.TextureViewDimension2D:
			srvDesc.SetTexture2D(baseMip, mipCount, 0, 0)
		case gputypes.TextureViewDimension2DArray:
			srvDesc.SetTexture2DArray(baseMip, mipCount, baseLayer, layerCount, 0, 0)
		case gputypes.TextureViewDimensionCube:
			srvDesc.SetTextureCube(baseMip, mipCount, 0)
		case gputypes.TextureViewDimensionCubeArray:
			srvDesc.SetTextureCubeArray(baseMip, mipCount, baseLayer/6, layerCount/6, 0)
		case gputypes.TextureViewDimension3D:
			srvDesc.SetTexture3D(baseMip, mipCount, 0)
		}

		d.raw.CreateShaderResourceView(tex.raw, &srvDesc, srvHandle)
		view.srvHandle = srvHandle
		view.srvHeapIndex = srvIndex
		view.hasSRV = true
	}

	return view, nil
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	if v, ok := view.(*TextureView); ok && v != nil {
		v.Destroy()
	}
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *hal.SamplerDescriptor) (hal.Sampler, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: sampler descriptor is nil")
	}

	// Allocate sampler descriptor
	handle, heapIndex, err := d.allocateSamplerDescriptor()
	if err != nil {
		return nil, fmt.Errorf("dx12: failed to allocate sampler descriptor: %w", err)
	}

	// Build D3D12 sampler desc
	samplerDesc := d3d12.D3D12_SAMPLER_DESC{
		Filter:         filterModeToD3D12(desc.MinFilter, desc.MagFilter, desc.MipmapFilter, desc.Compare),
		AddressU:       addressModeToD3D12(desc.AddressModeU),
		AddressV:       addressModeToD3D12(desc.AddressModeV),
		AddressW:       addressModeToD3D12(desc.AddressModeW),
		MipLODBias:     0,
		MaxAnisotropy:  uint32(desc.Anisotropy),
		ComparisonFunc: compareFunctionToD3D12(desc.Compare),
		BorderColor:    [4]float32{0, 0, 0, 0},
		MinLOD:         desc.LodMinClamp,
		MaxLOD:         desc.LodMaxClamp,
	}

	// Clamp anisotropy
	if samplerDesc.MaxAnisotropy == 0 {
		samplerDesc.MaxAnisotropy = 1
	}
	if samplerDesc.MaxAnisotropy > 16 {
		samplerDesc.MaxAnisotropy = 16
	}

	d.raw.CreateSampler(&samplerDesc, handle)

	return &Sampler{
		handle:    handle,
		heapIndex: heapIndex,
		device:    d,
	}, nil
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	if s, ok := sampler.(*Sampler); ok && s != nil {
		s.Destroy()
	}
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: bind group layout descriptor is nil")
	}

	entries := make([]BindGroupLayoutEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		entries[i] = BindGroupLayoutEntry{
			Binding:    entry.Binding,
			Visibility: entry.Visibility,
			Count:      1,
		}

		// Determine binding type
		switch {
		case entry.Buffer != nil:
			switch entry.Buffer.Type {
			case gputypes.BufferBindingTypeUniform:
				entries[i].Type = BindingTypeUniformBuffer
			case gputypes.BufferBindingTypeStorage:
				entries[i].Type = BindingTypeStorageBuffer
			case gputypes.BufferBindingTypeReadOnlyStorage:
				entries[i].Type = BindingTypeReadOnlyStorageBuffer
			}
		case entry.Sampler != nil:
			if entry.Sampler.Type == gputypes.SamplerBindingTypeComparison {
				entries[i].Type = BindingTypeComparisonSampler
			} else {
				entries[i].Type = BindingTypeSampler
			}
		case entry.Texture != nil:
			entries[i].Type = BindingTypeSampledTexture
		case entry.StorageTexture != nil:
			entries[i].Type = BindingTypeStorageTexture
		}
	}

	return &BindGroupLayout{
		entries: entries,
		device:  d,
	}, nil
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(layout hal.BindGroupLayout) {
	if l, ok := layout.(*BindGroupLayout); ok && l != nil {
		l.Destroy()
	}
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: bind group descriptor is nil")
	}

	layout, ok := desc.Layout.(*BindGroupLayout)
	if !ok {
		return nil, fmt.Errorf("dx12: invalid bind group layout type")
	}

	bg := &BindGroup{
		layout: layout,
		device: d,
	}

	// Classify entries into CBV/SRV/UAV vs Sampler
	var viewEntries []gputypes.BindGroupEntry  // CBV, SRV, UAV
	var samplerEntries []gputypes.BindGroupEntry

	for _, entry := range desc.Entries {
		switch entry.Resource.(type) {
		case gputypes.SamplerBinding:
			samplerEntries = append(samplerEntries, entry)
		default: // BufferBinding, TextureViewBinding
			viewEntries = append(viewEntries, entry)
		}
	}

	// Allocate and populate CBV/SRV/UAV descriptors
	if len(viewEntries) > 0 && d.viewHeap != nil {
		cpuStart, gpuStart, err := d.viewHeap.AllocateGPU(uint32(len(viewEntries)))
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to allocate view descriptors: %w", err)
		}
		bg.gpuDescHandle = gpuStart

		for i, entry := range viewEntries {
			destCPU := cpuStart.Offset(i, d.viewHeap.incrementSize)
			if err := d.writeViewDescriptor(destCPU, entry); err != nil {
				return nil, fmt.Errorf("dx12: failed to write descriptor for binding %d: %w", entry.Binding, err)
			}
		}
	}

	// Allocate and populate sampler descriptors
	if len(samplerEntries) > 0 && d.samplerHeap != nil {
		cpuStart, gpuStart, err := d.samplerHeap.AllocateGPU(uint32(len(samplerEntries)))
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to allocate sampler descriptors: %w", err)
		}
		bg.samplerGPUHandle = gpuStart

		for i, entry := range samplerEntries {
			destCPU := cpuStart.Offset(i, d.samplerHeap.incrementSize)
			sb := entry.Resource.(gputypes.SamplerBinding)
			sampler := (*Sampler)(unsafe.Pointer(sb.Sampler))
			d.raw.CopyDescriptorsSimple(1, destCPU, sampler.handle, d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_SAMPLER)
		}
	}

	return bg, nil
}

// writeViewDescriptor writes a single CBV/SRV/UAV descriptor to the specified CPU handle.
func (d *Device) writeViewDescriptor(dest d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, entry gputypes.BindGroupEntry) error {
	switch res := entry.Resource.(type) {
	case gputypes.BufferBinding:
		buf := (*Buffer)(unsafe.Pointer(res.Buffer))
		size := res.Size
		if size == 0 {
			size = buf.size - res.Offset
		}
		// Align CBV size to 256 bytes (D3D12 requirement)
		alignedSize := (size + 255) &^ 255
		d.raw.CreateConstantBufferView(&d3d12.D3D12_CONSTANT_BUFFER_VIEW_DESC{
			BufferLocation: buf.gpuVA + res.Offset,
			SizeInBytes:    uint32(alignedSize),
		}, dest)

	case gputypes.TextureViewBinding:
		view := (*TextureView)(unsafe.Pointer(res.TextureView))
		if !view.hasSRV {
			return fmt.Errorf("texture view has no SRV")
		}
		d.raw.CopyDescriptorsSimple(1, dest, view.srvHandle, d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_CBV_SRV_UAV)

	default:
		return fmt.Errorf("unsupported binding resource type: %T", entry.Resource)
	}
	return nil
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(group hal.BindGroup) {
	if g, ok := group.(*BindGroup); ok && g != nil {
		g.Destroy()
	}
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: pipeline layout descriptor is nil")
	}

	// Create root signature from bind group layouts
	rootSig, groupMappings, err := d.createRootSignatureFromLayouts(desc.BindGroupLayouts)
	if err != nil {
		return nil, err
	}

	// Store references to bind group layouts
	bgLayouts := make([]*BindGroupLayout, len(desc.BindGroupLayouts))
	for i, l := range desc.BindGroupLayouts {
		bgLayout, ok := l.(*BindGroupLayout)
		if !ok {
			rootSig.Release()
			return nil, fmt.Errorf("dx12: invalid bind group layout type at index %d", i)
		}
		bgLayouts[i] = bgLayout
	}

	return &PipelineLayout{
		rootSignature:    rootSig,
		bindGroupLayouts: bgLayouts,
		groupMappings:    groupMappings,
		device:           d,
	}, nil
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {
	if l, ok := layout.(*PipelineLayout); ok && l != nil {
		l.Destroy()
	}
}

// CreateShaderModule creates a shader module.
// Supports WGSL source (compiled via naga HLSL backend + D3DCompile) and pre-compiled SPIR-V.
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: shader module descriptor is nil")
	}

	module := &ShaderModule{
		entryPoints: make(map[string][]byte),
		device:      d,
	}

	switch {
	case desc.Source.WGSL != "":
		if err := d.compileWGSLModule(desc.Source.WGSL, module); err != nil {
			return nil, fmt.Errorf("dx12: WGSL compilation failed: %w", err)
		}
	case len(desc.Source.SPIRV) > 0:
		// Legacy: pre-compiled SPIR-V stored as single entry "main"
		bytecode := make([]byte, len(desc.Source.SPIRV)*4)
		for i, word := range desc.Source.SPIRV {
			bytecode[i*4+0] = byte(word)
			bytecode[i*4+1] = byte(word >> 8)
			bytecode[i*4+2] = byte(word >> 16)
			bytecode[i*4+3] = byte(word >> 24)
		}
		module.entryPoints["main"] = bytecode
	default:
		return nil, fmt.Errorf("dx12: no shader source provided (need WGSL or SPIRV)")
	}

	return module, nil
}

// compileWGSLModule compiles WGSL source to per-entry-point DXBC bytecode.
// Pipeline: WGSL → naga parse → IR → HLSL → D3DCompile → DXBC
func (d *Device) compileWGSLModule(wgslSource string, module *ShaderModule) error {
	// Step 1: Parse WGSL to AST
	ast, err := naga.Parse(wgslSource)
	if err != nil {
		return fmt.Errorf("WGSL parse: %w", err)
	}

	// Step 2: Lower to IR
	irModule, err := naga.LowerWithSource(ast, wgslSource)
	if err != nil {
		return fmt.Errorf("WGSL lower: %w", err)
	}

	// Step 3: Generate HLSL (all entry points)
	hlslSource, info, err := hlsl.Compile(irModule, hlsl.DefaultOptions())
	if err != nil {
		return fmt.Errorf("HLSL generation: %w", err)
	}

	// Step 4: Load d3dcompiler_47.dll
	compiler, err := d3dcompile.Load()
	if err != nil {
		return fmt.Errorf("load d3dcompiler: %w", err)
	}

	// Step 5: Compile each entry point separately
	for _, ep := range irModule.EntryPoints {
		target := shaderStageToTarget(ep.Stage)

		// Use the HLSL entry point name (naga may rename it)
		hlslName := ep.Name
		if info != nil && info.EntryPointNames != nil {
			if mapped, ok := info.EntryPointNames[ep.Name]; ok {
				hlslName = mapped
			}
		}

		bytecode, err := compiler.Compile(hlslSource, hlslName, target)
		if err != nil {
			return fmt.Errorf("D3DCompile entry point %q (hlsl: %q, target: %s): %w",
				ep.Name, hlslName, target, err)
		}

		module.entryPoints[ep.Name] = bytecode
	}

	return nil
}

// shaderStageToTarget maps naga IR shader stage to D3DCompile target profile.
func shaderStageToTarget(stage ir.ShaderStage) string {
	switch stage {
	case ir.StageVertex:
		return d3dcompile.TargetVS51
	case ir.StageFragment:
		return d3dcompile.TargetPS51
	case ir.StageCompute:
		return d3dcompile.TargetCS51
	default:
		return d3dcompile.TargetVS51
	}
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	if m, ok := module.(*ShaderModule); ok && m != nil {
		m.Destroy()
	}
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: render pipeline descriptor is nil")
	}

	// Build input layout from vertex buffers
	inputElements, semanticNames := buildInputLayout(desc.Vertex.Buffers)

	// Keep semantic names alive until pipeline creation
	_ = semanticNames

	// Build PSO description
	psoDesc, err := d.buildGraphicsPipelineStateDesc(desc, inputElements, semanticNames)
	if err != nil {
		return nil, err
	}

	// Create the pipeline state object
	pso, err := d.raw.CreateGraphicsPipelineState(psoDesc)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateGraphicsPipelineState failed: %w", err)
	}

	// Get root signature reference and group mappings for command list binding.
	// Must match the root signature used in the PSO.
	var rootSig *d3d12.ID3D12RootSignature
	var groupMappings []rootParamMapping
	if desc.Layout != nil {
		pipelineLayout, ok := desc.Layout.(*PipelineLayout)
		if ok {
			rootSig = pipelineLayout.rootSignature
			groupMappings = pipelineLayout.groupMappings
		}
	} else {
		// No layout → use the same empty root signature that was used in the PSO.
		rootSig, _ = d.getOrCreateEmptyRootSignature()
	}

	// Calculate vertex strides for IASetVertexBuffers
	vertexStrides := make([]uint32, len(desc.Vertex.Buffers))
	for i, buf := range desc.Vertex.Buffers {
		vertexStrides[i] = uint32(buf.ArrayStride)
	}

	return &RenderPipeline{
		pso:            pso,
		rootSignature:  rootSig,
		groupMappings:  groupMappings,
		topology:       primitiveTopologyToD3D12(desc.Primitive.Topology),
		vertexStrides:  vertexStrides,
	}, nil
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	if p, ok := pipeline.(*RenderPipeline); ok && p != nil {
		p.Destroy()
	}
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: compute pipeline descriptor is nil")
	}

	// Get shader module
	shaderModule, ok := desc.Compute.Module.(*ShaderModule)
	if !ok {
		return nil, fmt.Errorf("dx12: invalid compute shader module type")
	}

	// Get root signature and group mappings from layout.
	// DX12 requires a valid root signature for every PSO.
	var rootSig *d3d12.ID3D12RootSignature
	var groupMappings []rootParamMapping
	if desc.Layout != nil {
		pipelineLayout, ok := desc.Layout.(*PipelineLayout)
		if !ok {
			return nil, fmt.Errorf("dx12: invalid pipeline layout type")
		}
		rootSig = pipelineLayout.rootSignature
		groupMappings = pipelineLayout.groupMappings
	} else {
		emptyRS, err := d.getOrCreateEmptyRootSignature()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to get empty root signature for compute: %w", err)
		}
		rootSig = emptyRS
	}

	// Build compute pipeline state desc
	psoDesc := d3d12.D3D12_COMPUTE_PIPELINE_STATE_DESC{
		RootSignature: rootSig,
		NodeMask:      0,
	}

	bytecode := shaderModule.EntryPointBytecode(desc.Compute.EntryPoint)
	if len(bytecode) > 0 {
		psoDesc.CS = d3d12.D3D12_SHADER_BYTECODE{
			ShaderBytecode: unsafe.Pointer(&bytecode[0]),
			BytecodeLength: uintptr(len(bytecode)),
		}
	} else {
		return nil, fmt.Errorf("dx12: compute shader entry point %q not found in module", desc.Compute.EntryPoint)
	}

	// Create the pipeline state object
	pso, err := d.raw.CreateComputePipelineState(&psoDesc)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateComputePipelineState failed: %w", err)
	}

	return &ComputePipeline{
		pso:            pso,
		rootSignature:  rootSig,
		groupMappings:  groupMappings,
	}, nil
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	if p, ok := pipeline.(*ComputePipeline); ok && p != nil {
		p.Destroy()
	}
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(desc *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	// Create command allocator
	allocator, err := d.raw.CreateCommandAllocator(d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateCommandAllocator failed: %w", err)
	}

	// Create command list (starts in recording state)
	cmdList, err := d.raw.CreateCommandList(0, d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT, allocator, nil)
	if err != nil {
		allocator.Release()
		return nil, fmt.Errorf("dx12: CreateCommandList failed: %w", err)
	}

	// Command list is created in open state - close it immediately
	// It will be reset when BeginEncoding is called
	if err := cmdList.Close(); err != nil {
		cmdList.Release()
		allocator.Release()
		return nil, fmt.Errorf("dx12: initial command list close failed: %w", err)
	}

	var label string
	if desc != nil {
		label = desc.Label
	}

	return &CommandEncoder{
		device: d,
		allocator: &CommandAllocator{
			raw: allocator,
		},
		cmdList:     cmdList,
		label:       label,
		isRecording: false,
	}, nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	fence, err := d.raw.CreateFence(0, d3d12.D3D12_FENCE_FLAG_NONE)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateFence failed: %w", err)
	}

	// Create Windows event for this fence
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		fence.Release()
		return nil, fmt.Errorf("dx12: CreateEvent for fence failed: %w", err)
	}

	return &Fence{
		raw:   fence,
		event: event,
	}, nil
}

// DestroyFence destroys a fence.
func (d *Device) DestroyFence(fence hal.Fence) {
	f, ok := fence.(*Fence)
	if !ok || f == nil {
		return
	}

	if f.event != 0 {
		_ = windows.CloseHandle(f.event)
		f.event = 0
	}

	if f.raw != nil {
		f.raw.Release()
		f.raw = nil
	}
}

// Wait waits for a fence to reach the specified value.
// Returns true if the fence reached the value, false if timeout.
func (d *Device) Wait(fence hal.Fence, value uint64, timeout time.Duration) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok || f == nil {
		return false, fmt.Errorf("dx12: invalid fence")
	}

	// Check if already completed
	if f.raw.GetCompletedValue() >= value {
		return true, nil
	}

	// Set up event notification
	if err := f.raw.SetEventOnCompletion(value, uintptr(f.event)); err != nil {
		return false, fmt.Errorf("dx12: SetEventOnCompletion failed: %w", err)
	}

	// Convert timeout to milliseconds
	var timeoutMs uint32
	if timeout < 0 {
		timeoutMs = windows.INFINITE
	} else {
		timeoutMs = uint32(timeout.Milliseconds())
		if timeoutMs == 0 && timeout > 0 {
			timeoutMs = 1 // At least 1ms if non-zero duration
		}
	}

	result, err := windows.WaitForSingleObject(f.event, timeoutMs)
	if err != nil {
		return false, fmt.Errorf("dx12: WaitForSingleObject failed: %w", err)
	}

	switch result {
	case windows.WAIT_OBJECT_0:
		return true, nil
	case uint32(windows.WAIT_TIMEOUT):
		return false, nil
	default:
		return false, fmt.Errorf("dx12: WaitForSingleObject returned unexpected: %d", result)
	}
}

// ResetFence resets a fence to the unsignaled state.
// Note: D3D12 fences are timeline-based and don't have a direct reset.
// The fence value monotonically increases, so "reset" is a no-op.
// Users should track fence values properly for D3D12.
func (d *Device) ResetFence(_ hal.Fence) error {
	// D3D12 fences are timeline semaphores - they cannot be reset.
	// The fence value only increases monotonically.
	// This is a no-op to satisfy the interface.
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
// D3D12 fences are timeline-based, so we check if completed value > 0.
func (d *Device) GetFenceStatus(fence hal.Fence) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok || f == nil {
		return false, nil
	}
	// D3D12 fence: check if GPU has signaled the fence at all
	return f.GetCompletedValue() > 0, nil
}

// FreeCommandBuffer returns a command buffer to its command allocator.
// In DX12, command allocators are reset at frame boundaries rather than
// freeing individual command lists.
func (d *Device) FreeCommandBuffer(cmdBuffer hal.CommandBuffer) {
	// DX12 command lists are automatically managed through command allocator reset
	// Individual list freeing is not needed - allocator reset handles this
}

// CreateRenderBundleEncoder creates a render bundle encoder.
// Note: DX12 supports bundles natively, but not yet implemented.
func (d *Device) CreateRenderBundleEncoder(desc *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("dx12: render bundles not yet implemented")
}

// DestroyRenderBundle destroys a render bundle.
func (d *Device) DestroyRenderBundle(bundle hal.RenderBundle) {}

// Destroy releases the device.
func (d *Device) Destroy() {
	if d == nil {
		return
	}

	// Clear finalizer to prevent double-free
	runtime.SetFinalizer(d, nil)

	// Wait for GPU to finish before cleanup
	_ = d.waitForGPU()

	d.cleanup()
}

// -----------------------------------------------------------------------------
// Fence implementation
// -----------------------------------------------------------------------------

// Fence implements hal.Fence for DirectX 12.
type Fence struct {
	raw   *d3d12.ID3D12Fence
	event windows.Handle
}

// Destroy releases the fence resources.
func (f *Fence) Destroy() {
	if f.event != 0 {
		_ = windows.CloseHandle(f.event)
		f.event = 0
	}
	if f.raw != nil {
		f.raw.Release()
		f.raw = nil
	}
}

// GetCompletedValue returns the current fence value.
func (f *Fence) GetCompletedValue() uint64 {
	if f.raw == nil {
		return 0
	}
	return f.raw.GetCompletedValue()
}

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var (
	_ hal.Device = (*Device)(nil)
	_ hal.Fence  = (*Fence)(nil)
)
