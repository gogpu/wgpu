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

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
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

// createDescriptorHeaps creates the descriptor heaps for various resource types.
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

// cleanup releases all device resources without clearing the finalizer.
func (d *Device) cleanup() {
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
// hal.Device interface implementation
// -----------------------------------------------------------------------------

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(_ *hal.BufferDescriptor) (hal.Buffer, error) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
	return nil, fmt.Errorf("dx12: CreateBuffer not yet implemented")
}

// DestroyBuffer destroys a GPU buffer.
func (d *Device) DestroyBuffer(_ hal.Buffer) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(_ *hal.TextureDescriptor) (hal.Texture, error) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
	return nil, fmt.Errorf("dx12: CreateTexture not yet implemented")
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(_ hal.Texture) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(_ hal.Texture, _ *hal.TextureViewDescriptor) (hal.TextureView, error) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
	return nil, fmt.Errorf("dx12: CreateTextureView not yet implemented")
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(_ hal.TextureView) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
	return nil, fmt.Errorf("dx12: CreateSampler not yet implemented")
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(_ hal.Sampler) {
	// TODO: Implement in TASK-DX12-006 (Resource Creation)
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(_ *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
	return nil, fmt.Errorf("dx12: CreateBindGroupLayout not yet implemented")
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(_ hal.BindGroupLayout) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(_ *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
	return nil, fmt.Errorf("dx12: CreateBindGroup not yet implemented")
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(_ hal.BindGroup) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(_ *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
	return nil, fmt.Errorf("dx12: CreatePipelineLayout not yet implemented")
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(_ hal.PipelineLayout) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(_ *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
	return nil, fmt.Errorf("dx12: CreateShaderModule not yet implemented")
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(_ hal.ShaderModule) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(_ *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
	return nil, fmt.Errorf("dx12: CreateRenderPipeline not yet implemented")
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(_ hal.RenderPipeline) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(_ *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
	return nil, fmt.Errorf("dx12: CreateComputePipeline not yet implemented")
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(_ hal.ComputePipeline) {
	// TODO: Implement in TASK-DX12-007 (Pipeline State)
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	// TODO: Implement in TASK-DX12-008 (Command Encoding)
	return nil, fmt.Errorf("dx12: CreateCommandEncoder not yet implemented")
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
