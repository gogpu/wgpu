// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/dxil"
	"github.com/gogpu/naga/hlsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/d3dcompile"
	"golang.org/x/sys/windows"
)

// maxFramesInFlight is the number of frames that can be in-flight simultaneously.
// Set to 1 pending buffer barrier implementation in PendingWrites.
// With maxFramesInFlight=2, frame N+1's CopyBufferToBuffer overwrites buffers
// that frame N's render is still reading — GPU pipeline overlap causes data race.
// Rust wgpu handles this via per-buffer transition barriers (current→COPY_DST)
// that force GPU to complete reads before writes. See BUG-DX12-006.
// maxFramesInFlight is the number of frames that can be in-flight simultaneously.
// Set to 2 for CPU/GPU parallelism. GPU serialization is handled by
// ID3D12CommandQueue::Wait in Submit (GPU waits for previous frame's fence
// before executing, while CPU continues recording the next frame).
const maxFramesInFlight = 2

// frameState tracks allocators and fence value for one in-flight frame slot.
type frameState struct {
	allocators []*CommandAllocator
	fenceValue uint64
}

// Device implements hal.Device for DirectX 12.
// It manages the D3D12 device, command queue, descriptor heaps, and synchronization.
type Device struct {
	raw      *d3d12.ID3D12Device
	instance *Instance

	// Command queue for graphics/compute operations.
	directQueue *d3d12.ID3D12CommandQueue

	// Descriptor heaps (shared for all resources).
	viewHeap    *DescriptorHeap // CBV/SRV/UAV (shader-visible, for bind groups)
	samplerHeap *DescriptorHeap // Samplers (shader-visible, for bind groups)
	rtvHeap     *DescriptorHeap // Render Target Views (non-shader-visible)
	dsvHeap     *DescriptorHeap // Depth Stencil Views (non-shader-visible)

	// Staging heaps (non-shader-visible, CPU-only) for creating SRVs and samplers.
	// DX12 requires CopyDescriptorsSimple source to be in a non-shader-visible heap
	// because shader-visible heaps use WRITE_COMBINE or GPU-local memory which is
	// prohibitively slow to read from (Microsoft docs: ID3D12Device::CopyDescriptorsSimple).
	stagingViewHeap    *DescriptorHeap // CBV/SRV/UAV staging
	stagingSamplerHeap *DescriptorHeap // Sampler staging

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

	// Debug info queue for validation messages (nil when debug layer is off).
	infoQueue *d3d12.ID3D12InfoQueue

	// Debug: operation step counter for tracking which call kills the device.
	debugStep int

	// Per-frame tracking for CPU/GPU overlap (DX12-OPT-002).
	frames         [maxFramesInFlight]frameState
	frameIndex     uint64
	freeAllocators []*d3d12.ID3D12CommandAllocator
	allocatorMu    sync.Mutex

	// In-memory HLSL->DXBC shader cache (TASK-DX12-PSO-CACHE-001).
	// Caches FXC compilation results keyed by HLSL source hash + entry point + stage + target.
	// Matches Rust wgpu ShaderCache pattern (wgpu-hal/src/dx12/mod.rs:1136).
	shaderCache ShaderCache

	// useDXIL enables direct DXIL compilation via naga dxil backend,
	// bypassing the HLSL->FXC path. Opt-in via GOGPU_DX12_DXIL=1 env var.
	// Requires SM 6.0+ and AgilitySDK 1.615+ for BYPASS hash support.
	useDXIL bool

	// dxilValidate runs naga.dxil.Validate (IDxcValidator) on every
	// DXIL blob right after Compile so real HRESULT errors surface in
	// the wgpu log instead of being folded into E_INVALIDARG by D3D12
	// pipeline creation. Opt-in via GOGPU_DX12_DXIL_VALIDATE=1.
	dxilValidate bool
}

// DescriptorHeap wraps a D3D12 descriptor heap with allocation tracking.
// Supports descriptor recycling via a free list — freed indices are reused
// before bumping the linear allocator. This prevents heap exhaustion during
// operations like swapchain resize that repeatedly allocate/free RTVs.
type DescriptorHeap struct {
	raw           *d3d12.ID3D12DescriptorHeap
	heapType      d3d12.D3D12_DESCRIPTOR_HEAP_TYPE
	cpuStart      d3d12.D3D12_CPU_DESCRIPTOR_HANDLE
	gpuStart      d3d12.D3D12_GPU_DESCRIPTOR_HANDLE
	incrementSize uint32
	capacity      uint32
	nextFree      uint32
	freeList      []uint32 // Recycled descriptor indices (LIFO stack)
	mu            sync.Mutex
}

// Allocate allocates descriptors from the heap.
// Returns the CPU handle for the first allocated descriptor.
// For single-descriptor allocations, recycled slots are preferred.
func (h *DescriptorHeap) Allocate(count uint32) (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Recycle from free list for single-descriptor allocations
	if count == 1 && len(h.freeList) > 0 {
		idx := h.freeList[len(h.freeList)-1]
		h.freeList = h.freeList[:len(h.freeList)-1]
		handle := h.cpuStart.Offset(int(idx), h.incrementSize)
		return handle, nil
	}

	if h.nextFree+count > h.capacity {
		hal.Logger().Error("dx12: descriptor heap exhausted",
			"heapType", h.heapType,
			"capacity", h.capacity,
			"used", h.nextFree,
			"freeList", len(h.freeList),
			"requested", count,
		)
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, fmt.Errorf("dx12: descriptor heap exhausted (capacity=%d, used=%d, requested=%d)",
			h.capacity, h.nextFree, count)
	}

	handle := h.cpuStart.Offset(int(h.nextFree), h.incrementSize)
	h.nextFree += count
	return handle, nil
}

// AllocateGPU allocates descriptors and returns both CPU and GPU handles.
// For single-descriptor allocations, recycled slots are preferred.
func (h *DescriptorHeap) AllocateGPU(count uint32) (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, d3d12.D3D12_GPU_DESCRIPTOR_HANDLE, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Recycle from free list for single-descriptor allocations
	if count == 1 && len(h.freeList) > 0 {
		idx := h.freeList[len(h.freeList)-1]
		h.freeList = h.freeList[:len(h.freeList)-1]
		cpuHandle := h.cpuStart.Offset(int(idx), h.incrementSize)
		gpuHandle := h.gpuStart.Offset(int(idx), h.incrementSize)
		return cpuHandle, gpuHandle, nil
	}

	if h.nextFree+count > h.capacity {
		hal.Logger().Error("dx12: descriptor heap exhausted",
			"heapType", h.heapType,
			"capacity", h.capacity,
			"used", h.nextFree,
			"freeList", len(h.freeList),
			"requested", count,
		)
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, d3d12.D3D12_GPU_DESCRIPTOR_HANDLE{},
			fmt.Errorf("dx12: descriptor heap exhausted (capacity=%d, used=%d, requested=%d)",
				h.capacity, h.nextFree, count)
	}

	cpuHandle := h.cpuStart.Offset(int(h.nextFree), h.incrementSize)
	gpuHandle := h.gpuStart.Offset(int(h.nextFree), h.incrementSize)
	h.nextFree += count
	return cpuHandle, gpuHandle, nil
}

// Free returns descriptor indices to the free list for reuse.
// The descriptors must no longer be referenced by any in-flight GPU work.
func (h *DescriptorHeap) Free(baseIndex, count uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := uint32(0); i < count; i++ {
		h.freeList = append(h.freeList, baseIndex+i)
	}
}

// HandleToIndex computes the descriptor index from a CPU handle.
func (h *DescriptorHeap) HandleToIndex(handle d3d12.D3D12_CPU_DESCRIPTOR_HANDLE) uint32 {
	return uint32((handle.Ptr - h.cpuStart.Ptr) / uintptr(h.incrementSize))
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

	// Query InfoQueue for debug messages (only available when debug layer is on).
	if instance.flags&gputypes.InstanceFlagsDebug != 0 {
		if iq := rawDevice.QueryInfoQueue(); iq != nil {
			dev.infoQueue = iq
			hal.Logger().Info("dx12: InfoQueue attached, debug messages enabled")
		} else {
			hal.Logger().Debug("dx12: InfoQueue not available, debug layer may not be active")
		}
	}

	// Enable DXIL direct compilation if requested via environment variable.
	// DXIL path uses naga dxil backend (SM 6.0, BYPASS hash) instead of HLSL->FXC.
	dev.useDXIL = os.Getenv("GOGPU_DX12_DXIL") == "1"
	dev.dxilValidate = os.Getenv("GOGPU_DX12_DXIL_VALIDATE") == "1"

	// Set a finalizer to ensure cleanup
	runtime.SetFinalizer(dev, (*Device).Destroy)

	if dev.useDXIL {
		hal.Logger().Info("dx12: device created (DXIL direct compilation via naga dxil backend)",
			"featureLevel", fmt.Sprintf("0x%x", featureLevel),
			"debugLayer", instance.flags&gputypes.InstanceFlagsDebug != 0,
		)
	} else {
		hal.Logger().Info("dx12: device created (HLSL→FXC compilation)",
			"featureLevel", fmt.Sprintf("0x%x", featureLevel),
			"debugLayer", instance.flags&gputypes.InstanceFlagsDebug != 0,
		)
	}

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

	// CBV/SRV/UAV heap (shader visible — for bind groups, referenced by GPU)
	d.viewHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_CBV_SRV_UAV,
		1_000_000, // D3D12 Tier 1/2 maximum for shader-visible CBV/SRV/UAV
		true,      // shader visible
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create CBV/SRV/UAV heap: %w", err)
	}

	// CBV/SRV/UAV staging heap (non-shader-visible — for creating SRVs)
	// CopyDescriptorsSimple requires source in non-shader-visible heap because
	// shader-visible heaps use WRITE_COMBINE memory that is slow/unreliable to read.
	d.stagingViewHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_CBV_SRV_UAV,
		65536, // 64K staging descriptors
		false, // non-shader-visible
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create staging CBV/SRV/UAV heap: %w", err)
	}

	// Sampler heap (shader visible — for bind groups, referenced by GPU)
	d.samplerHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_SAMPLER,
		2048,
		true,
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create sampler heap: %w", err)
	}

	// Sampler staging heap (non-shader-visible — for creating samplers)
	d.stagingSamplerHeap, err = d.createHeap(
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_SAMPLER,
		2048,
		false,
	)
	if err != nil {
		return fmt.Errorf("dx12: failed to create staging sampler heap: %w", err)
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

	hal.Logger().Info("dx12: descriptor heaps created",
		"viewHeapSize", d.viewHeap.capacity,
		"samplerHeapSize", d.samplerHeap.capacity,
	)

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

// Note: acquireAllocator removed — encoders now own their allocators permanently
// (Rust wgpu-hal pattern). See CommandEncoder.allocator and ResetAll().

// waitForFrameSlot waits until the GPU finishes the frame occupying the given slot.
// Returns immediately if no work was submitted for that slot.
func (d *Device) waitForFrameSlot(slot uint64) error {
	d.fenceMu.Lock()
	defer d.fenceMu.Unlock()

	target := d.frames[slot].fenceValue
	if target == 0 {
		return nil
	}

	if d.fence.GetCompletedValue() >= target {
		return nil
	}

	if err := d.fence.SetEventOnCompletion(target, uintptr(d.fenceEvent)); err != nil {
		return fmt.Errorf("dx12: SetEventOnCompletion failed: %w", err)
	}
	_, err := windows.WaitForSingleObject(d.fenceEvent, windows.INFINITE)
	if err != nil {
		return fmt.Errorf("dx12: WaitForSingleObject failed: %w", err)
	}
	return nil
}

// advanceFrame increments the frame index.
// The actual wait for the old frame occupying the new slot is deferred to
// recycleFrameSlot, which is called at the start of the next frame
// (in AcquireTexture). This allows the CPU to begin preparing the next
// frame immediately after Present, overlapping with GPU execution.
func (d *Device) advanceFrame() {
	d.frameIndex++
}

// recycleFrameSlot waits for the GPU to finish the old frame occupying the
// current slot, then recycles its command allocators. Called at the start
// of each frame (from AcquireTexture) to ensure the slot is free before
// the CPU begins recording new commands into it.
func (d *Device) recycleFrameSlot() error {
	slot := d.frameIndex % maxFramesInFlight

	// Wait for the old frame that occupied this slot to finish on GPU.
	if err := d.waitForFrameSlot(slot); err != nil {
		return err
	}

	// Clear fence value for this slot (protected by fenceMu).
	d.fenceMu.Lock()
	d.frames[slot].fenceValue = 0
	d.fenceMu.Unlock()

	// Note: Allocator recycling removed — encoders now own their allocators
	// permanently (Rust wgpu-hal pattern). Allocator Reset happens in
	// CommandEncoder.ResetAll() after GPU completion via PendingWrites.maintain().

	return nil
}

// signalFrameFence signals the device fence from the queue and records the value
// in the current frame slot. This enables per-frame fence tracking — advanceFrame
// only needs to wait for the specific slot's fence value, not all GPU work.
func (d *Device) signalFrameFence() error {
	d.fenceMu.Lock()
	d.fenceValue++
	value := d.fenceValue
	start := time.Now()
	err := d.directQueue.Signal(d.fence, value)
	d.frames[d.frameIndex%maxFramesInFlight].fenceValue = value
	d.fenceMu.Unlock()

	if err != nil {
		return fmt.Errorf("dx12: frame fence Signal failed: %w", err)
	}

	hal.Logger().Debug("dx12: fence signaled",
		"value", value,
		"elapsed", time.Since(start),
	)

	return nil
}

// currentFrameFenceValue returns the most recently signaled fence value.
// This is used as the submission index.
func (d *Device) currentFrameFenceValue() uint64 {
	d.fenceMu.Lock()
	v := d.fenceValue
	d.fenceMu.Unlock()
	return v
}

// completedFrameFenceValue returns the highest fence value completed by the GPU.
// Non-blocking — queries the D3D12 fence directly.
func (d *Device) completedFrameFenceValue() uint64 {
	return d.fence.GetCompletedValue()
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

// checkHealth verifies the device is still alive and logs the current step.
// Returns an error if the device has been removed.
func (d *Device) checkHealth(operation string) error {
	d.debugStep++
	d.DrainDebugMessages()
	if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
		hal.Logger().Error("dx12: device removed", "step", d.debugStep, "operation", operation, "reason", reason)
		d.logDREDBreadcrumbs()
		return fmt.Errorf("dx12: device removed at step %d (%s): %w", d.debugStep, operation, reason)
	}
	return nil
}

// CheckHealth is a public diagnostic method that checks if the DX12 device
// is still operational. Returns nil if healthy, or an error with the removal reason.
func (d *Device) CheckHealth(label string) error {
	return d.checkHealth(label)
}

// logDREDBreadcrumbs queries and logs DRED (Device Removed Extended Data) after
// a device removal event. This provides auto-breadcrumbs showing which GPU command
// was executing when the TDR occurred, and page fault information if applicable.
// No-op if DRED is not available or was not enabled.
func (d *Device) logDREDBreadcrumbs() {
	dred := d.raw.QueryDRED1()
	if dred == nil {
		hal.Logger().Debug("dx12: DRED not available for this device")
		return
	}
	defer dred.Release()

	// Query auto-breadcrumbs
	var breadcrumbs d3d12.D3D12DREDAutoBreadcrumbsOutput1
	if err := dred.GetAutoBreadcrumbsOutput1(&breadcrumbs); err != nil {
		hal.Logger().Warn("dx12: DRED GetAutoBreadcrumbsOutput1 failed", "err", err)
	} else {
		d.logBreadcrumbNodes(breadcrumbs.HeadAutoBreadcrumbNode)
	}

	// Query page fault information
	var pageFault d3d12.D3D12DREDPageFaultOutput1
	if err := dred.GetPageFaultAllocationOutput1(&pageFault); err != nil {
		hal.Logger().Warn("dx12: DRED GetPageFaultAllocationOutput1 failed", "err", err)
	} else {
		d.logPageFault(&pageFault)
	}
}

// logBreadcrumbNodes walks the DRED breadcrumb linked list and logs each node.
func (d *Device) logBreadcrumbNodes(node *d3d12.D3D12AutoBreadcrumbNode1) {
	if node == nil {
		hal.Logger().Info("dx12: DRED — no breadcrumb data available")
		return
	}

	const dredUnnamed = "<unnamed>"

	nodeIdx := 0
	for n := node; n != nil; n = n.Next {
		ops := n.BreadcrumbOps()
		lastCompleted := n.LastCompleted()

		// Read debug names (may be nil)
		cmdListName := dredUnnamed
		if n.CommandListDebugNameW != nil {
			if name := windows.UTF16PtrToString(n.CommandListDebugNameW); name != "" {
				cmdListName = name
			}
		}
		cmdQueueName := dredUnnamed
		if n.CommandQueueDebugNameW != nil {
			if name := windows.UTF16PtrToString(n.CommandQueueDebugNameW); name != "" {
				cmdQueueName = name
			}
		}

		hal.Logger().Error("dx12: DRED breadcrumb node",
			"node", nodeIdx,
			"commandList", cmdListName,
			"commandQueue", cmdQueueName,
			"totalOps", len(ops),
			"lastCompleted", lastCompleted,
		)

		// Log operations around the hang point (context window)
		if len(ops) > 0 {
			d.logBreadcrumbOps(ops, lastCompleted)
		}

		nodeIdx++
	}
}

// logBreadcrumbOps logs the operations around the last completed breadcrumb,
// showing which operation was executing when the GPU hung.
func (d *Device) logBreadcrumbOps(ops []d3d12.D3D12AutoBreadcrumbOp, lastCompleted uint32) {
	// Show a window around the hang point: 3 before + the hung op + 2 after
	const contextBefore = 3
	const contextAfter = 2

	start := int(lastCompleted) - contextBefore
	if start < 0 {
		start = 0
	}
	end := int(lastCompleted) + contextAfter + 2 // +2 for the hung op and one extra
	if end > len(ops) {
		end = len(ops)
	}

	for i := start; i < end; i++ {
		opName := d3d12.AutoBreadcrumbOpName(ops[i])
		status := "COMPLETED"
		marker := ""

		switch {
		case uint32(i) == lastCompleted:
			marker = " <- last completed"
		case uint32(i) == lastCompleted+1:
			status = "INCOMPLETE"
			marker = " <- GPU hung here"
		case uint32(i) > lastCompleted:
			status = "not reached"
		}

		hal.Logger().Error("dx12: DRED op",
			"index", i,
			"op", opName,
			"status", status+marker,
		)
	}
}

// logPageFault logs DRED page fault information.
func (d *Device) logPageFault(pf *d3d12.D3D12DREDPageFaultOutput1) {
	if pf.PageFaultVA == 0 && pf.HeadExistingAllocationNode == nil && pf.HeadRecentFreedAllocationNode == nil {
		hal.Logger().Info("dx12: DRED — no page fault detected")
		return
	}

	hal.Logger().Error("dx12: DRED page fault",
		"faultAddress", fmt.Sprintf("0x%016X", pf.PageFaultVA),
	)

	// Log existing allocations near the fault
	if pf.HeadExistingAllocationNode != nil {
		hal.Logger().Error("dx12: DRED — existing allocations at fault address:")
		d.logAllocationNodes(pf.HeadExistingAllocationNode, 5)
	}

	// Log recently freed allocations (use-after-free detection)
	if pf.HeadRecentFreedAllocationNode != nil {
		hal.Logger().Error("dx12: DRED — recently freed allocations (possible use-after-free):")
		d.logAllocationNodes(pf.HeadRecentFreedAllocationNode, 5)
	}
}

// logAllocationNodes logs up to maxNodes allocation nodes from a DRED linked list.
func (d *Device) logAllocationNodes(node *d3d12.D3D12DREDAllocationNode1, maxNodes int) {
	const unnamed = "<unnamed>"
	count := 0
	for n := node; n != nil && count < maxNodes; n = n.Next {
		name := unnamed
		if n.ObjectNameW != nil {
			if wname := windows.UTF16PtrToString(n.ObjectNameW); wname != "" {
				name = wname
			}
		}
		hal.Logger().Error("dx12: DRED allocation",
			"name", name,
			"type", n.AllocationType,
		)
		count++
	}
}

// DrainDebugMessages reads and logs all pending messages from the D3D12 InfoQueue.
// Returns the number of messages drained. No-op if debug layer is off.
func (d *Device) DrainDebugMessages() int {
	if d.infoQueue == nil {
		return 0
	}

	count := d.infoQueue.GetNumStoredMessages()
	if count == 0 {
		return 0
	}

	for i := uint64(0); i < count; i++ {
		msg := d.infoQueue.GetMessage(i)
		if msg == nil {
			continue
		}
		hal.Logger().Warn("dx12: debug message", "severity", msg.Severity, "id", msg.ID, "msg", msg.Description())
	}
	d.infoQueue.ClearStoredMessages()

	return int(count)
}

// cleanup releases all device resources without clearing the finalizer.
func (d *Device) cleanup() {
	// Drain any remaining debug messages before releasing resources.
	d.DrainDebugMessages()

	// Release pooled and in-flight allocators.
	d.allocatorMu.Lock()
	for _, raw := range d.freeAllocators {
		if raw != nil {
			raw.Release()
		}
	}
	d.freeAllocators = nil
	for i := range d.frames {
		for _, alloc := range d.frames[i].allocators {
			if alloc != nil && alloc.raw != nil {
				alloc.raw.Release()
			}
		}
		d.frames[i].allocators = nil
	}
	d.allocatorMu.Unlock()

	if d.infoQueue != nil {
		d.infoQueue.Release()
		d.infoQueue = nil
	}

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
	if d.stagingViewHeap != nil && d.stagingViewHeap.raw != nil {
		d.stagingViewHeap.raw.Release()
		d.stagingViewHeap = nil
	}
	if d.samplerHeap != nil && d.samplerHeap.raw != nil {
		d.samplerHeap.raw.Release()
		d.samplerHeap = nil
	}
	if d.stagingSamplerHeap != nil && d.stagingSamplerHeap.raw != nil {
		d.stagingSamplerHeap.raw.Release()
		d.stagingSamplerHeap = nil
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

// allocateDescriptor allocates a descriptor from the given staging heap.
// Recycles freed slots before bumping the watermark.
func allocateDescriptor(heap *DescriptorHeap, heapName string) (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	if heap == nil {
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: %s heap not initialized", heapName)
	}

	heap.mu.Lock()
	defer heap.mu.Unlock()

	// Prefer recycled slots.
	if len(heap.freeList) > 0 {
		idx := heap.freeList[len(heap.freeList)-1]
		heap.freeList = heap.freeList[:len(heap.freeList)-1]
		handle := heap.cpuStart.Offset(int(idx), heap.incrementSize)
		return handle, idx, nil
	}

	if heap.nextFree >= heap.capacity {
		hal.Logger().Error("dx12: descriptor heap exhausted",
			"heapType", heapName,
			"capacity", heap.capacity,
			"used", heap.nextFree,
			"freeList", len(heap.freeList),
		)
		return d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{}, 0, fmt.Errorf("dx12: %s heap exhausted", heapName)
	}

	index := heap.nextFree
	handle := heap.cpuStart.Offset(int(index), heap.incrementSize)
	heap.nextFree++
	return handle, index, nil
}

// allocateRTVDescriptor allocates a render target view descriptor.
func (d *Device) allocateRTVDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	return allocateDescriptor(d.rtvHeap, "RTV")
}

// allocateDSVDescriptor allocates a depth stencil view descriptor.
func (d *Device) allocateDSVDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	return allocateDescriptor(d.dsvHeap, "DSV")
}

// allocateSRVDescriptor allocates a shader resource view descriptor in the
// non-shader-visible staging heap. SRVs are created here and later copied to
// the shader-visible heap via CopyDescriptorsSimple in CreateBindGroup.
// DX12 requires CopyDescriptorsSimple source to be non-shader-visible.
func (d *Device) allocateSRVDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	return allocateDescriptor(d.stagingViewHeap, "staging CBV/SRV/UAV")
}

// allocateSamplerDescriptor allocates a sampler descriptor in the
// non-shader-visible staging heap. Samplers are created here and later copied
// to the shader-visible heap via CopyDescriptorsSimple in CreateBindGroup.
func (d *Device) allocateSamplerDescriptor() (d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, uint32, error) {
	return allocateDescriptor(d.stagingSamplerHeap, "staging sampler")
}

// -----------------------------------------------------------------------------
// hal.Device interface implementation
// -----------------------------------------------------------------------------

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: buffer descriptor is nil in DX12.CreateBuffer — core validation gap")
	}

	// Determine heap type based on usage.
	// Matches Rust wgpu-hal suballocation.rs:437-464 which uses HEAP_TYPE_CUSTOM
	// with WRITE_COMBINE + L0 for CpuToGpu (MapWrite) buffers, and COMMON state
	// that allows implicit promotion to COPY_DST. This enables staging belt
	// CopyBufferRegion into MapWrite buffers (prevents data race on Metal/DX12).
	var heapType d3d12.D3D12_HEAP_TYPE
	var cpuPageProperty d3d12.D3D12_CPU_PAGE_PROPERTY
	var memoryPool d3d12.D3D12_MEMORY_POOL
	var initialState d3d12.D3D12_RESOURCE_STATES

	switch {
	case desc.Usage&gputypes.BufferUsageMapRead != 0:
		// Readback buffer — GPU writes, CPU reads
		heapType = d3d12.D3D12_HEAP_TYPE_CUSTOM
		cpuPageProperty = d3d12.D3D12_CPU_PAGE_PROPERTY_WRITE_BACK
		memoryPool = d3d12.D3D12_MEMORY_POOL_L0
		initialState = d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	case desc.Usage&gputypes.BufferUsageMapWrite != 0 || desc.MappedAtCreation:
		// CPU-to-GPU buffer — CPU writes, GPU reads. CUSTOM heap with WRITE_COMBINE
		// allows COMMON state which permits implicit promotion to COPY_DST.
		// This is required for staging belt CopyBufferRegion to work.
		heapType = d3d12.D3D12_HEAP_TYPE_CUSTOM
		cpuPageProperty = d3d12.D3D12_CPU_PAGE_PROPERTY_WRITE_COMBINE
		memoryPool = d3d12.D3D12_MEMORY_POOL_L0
		initialState = d3d12.D3D12_RESOURCE_STATE_COMMON
	default:
		// Default (GPU-only) buffer
		heapType = d3d12.D3D12_HEAP_TYPE_DEFAULT
		cpuPageProperty = d3d12.D3D12_CPU_PAGE_PROPERTY_UNKNOWN
		memoryPool = d3d12.D3D12_MEMORY_POOL_UNKNOWN
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
		CPUPageProperty:      cpuPageProperty,
		MemoryPoolPreference: memoryPool,
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
		raw:             resource,
		size:            desc.Size, // Return original size, not aligned size
		usage:           desc.Usage,
		heapType:        heapType,
		cpuPageProperty: cpuPageProperty,
		gpuVA:           resource.GetGPUVirtualAddress(),
		device:          d,
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

// MapBuffer establishes a CPU-visible mapping for the given byte range.
//
// DX12 UPLOAD / READBACK / CUSTOM heaps are always backed by host-visible
// memory; ID3D12Resource::Map is effectively free when the buffer is already
// mapped (reference-counted inside D3D12). If MappedAtCreation kept the
// mapping live, we reuse the cached pointer; otherwise we call Map(0, ...)
// and remember it so subsequent re-maps and UnmapBuffer are symmetric.
func (d *Device) MapBuffer(buffer hal.Buffer, offset, size uint64) (hal.BufferMapping, error) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || buf.raw == nil {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	if offset+size > buf.size {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	if !buf.isMappable() {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}

	if buf.mappedPointer == nil {
		// For read-back buffers, specify the full range we may read.
		// For upload/write buffers, pass a zero range (no reads).
		var readRange *d3d12.D3D12_RANGE
		if buf.isReadback() {
			readRange = &d3d12.D3D12_RANGE{Begin: 0, End: uintptr(buf.size)}
		} else {
			readRange = &d3d12.D3D12_RANGE{Begin: 0, End: 0}
		}
		ptr, err := buf.raw.Map(0, readRange)
		if err != nil {
			return hal.BufferMapping{}, fmt.Errorf("dx12: MapBuffer: %w", err)
		}
		buf.mappedPointer = ptr
	}

	return hal.BufferMapping{
		Ptr:        unsafe.Pointer(uintptr(buf.mappedPointer) + uintptr(offset)),
		IsCoherent: true, // DX12 UPLOAD/READBACK are always coherent with CPU.
	}, nil
}

// UnmapBuffer releases a CPU-visible mapping.
//
// DX12 allows persistent mappings across many Map/Unmap calls; we drop the
// cached mappedPointer only when core explicitly requests unmap, and we
// pass a full-range written hint for writable heaps so the driver can
// invalidate write-combine buffers.
func (d *Device) UnmapBuffer(buffer hal.Buffer) error {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || buf.raw == nil {
		return nil
	}
	if buf.mappedPointer == nil {
		return nil
	}
	var writtenRange *d3d12.D3D12_RANGE
	if !buf.isReadback() {
		writtenRange = &d3d12.D3D12_RANGE{Begin: 0, End: uintptr(buf.size)}
	}
	buf.raw.Unmap(0, writtenRange)
	buf.mappedPointer = nil
	return nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: texture descriptor is nil in DX12.CreateTexture — core validation gap")
	}

	// Check device health before allocating resources.
	if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
		d.DrainDebugMessages() // Print validation errors that killed the device
		d.logDREDBreadcrumbs()
		return nil, fmt.Errorf("dx12: device already removed before CreateTexture (format=%d, samples=%d): %w",
			desc.Format, desc.SampleCount, reason)
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

	// All DEFAULT heap textures start in COMMON state (DX12 spec requirement).
	// Matches Rust wgpu (suballocation.rs:369). Auto-promotion handles the first
	// use transition (COMMON → COPY_DEST, COMMON → RENDER_TARGET, etc.).
	// Previous code used non-COMMON initial states which violates the spec and
	// causes incorrect barrier "from" states in PendingWrites (BUG-DX12-009).
	initialState := d3d12.D3D12_RESOURCE_STATE_COMMON

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
		if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
			return nil, fmt.Errorf("dx12: CreateCommittedResource for texture failed (device removed: %w, format=%d, samples=%d, %dx%d, flags=0x%x): %w",
				reason, createFormat, sampleCount, desc.Size.Width, desc.Size.Height, resourceFlags, err)
		}
		return nil, fmt.Errorf("dx12: CreateCommittedResource for texture failed (format=%d, samples=%d, %dx%d, flags=0x%x): %w",
			createFormat, sampleCount, desc.Size.Width, desc.Size.Height, resourceFlags, err)
	}

	tex := &Texture{
		raw:       resource,
		format:    desc.Format,
		dimension: desc.Dimension,
		size: hal.Extent3D{
			Width:              desc.Size.Width,
			Height:             desc.Size.Height,
			DepthOrArrayLayers: depthOrArraySize,
		},
		mipLevels:    mipLevels,
		samples:      sampleCount,
		usage:        desc.Usage,
		device:       d,
		currentState: initialState,
	}

	// Post-creation health check: detect if CreateCommittedResource silently poisoned the device.
	if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
		d.DrainDebugMessages()
		d.logDREDBreadcrumbs()
		hal.Logger().Error("dx12: device removed during CreateTexture",
			"label", desc.Label,
			"format", createFormat, "samples", sampleCount,
			"width", desc.Size.Width, "height", desc.Size.Height,
			"flags", fmt.Sprintf("0x%x", resourceFlags), "reason", reason)
	}

	return tex, nil
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
	// hasRTV=true so BeginRenderPass uses this RTV, but isExternal=true tells
	// Destroy() to skip freeing the RTV heap slot (the Surface owns it).
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

	isMultisampled := tex.samples > 1

	// Create RTV if texture supports render attachment and is not depth
	if tex.usage&gputypes.TextureUsageRenderAttachment != 0 && !isDepthFormat(viewFormat) {
		// Allocate RTV descriptor
		rtvHandle, rtvIndex, err := d.allocateRTVDescriptor()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to allocate RTV descriptor: %w", err)
		}

		var rtvDesc d3d12.D3D12_RENDER_TARGET_VIEW_DESC
		if isMultisampled && viewDim == gputypes.TextureViewDimension2D {
			// MSAA textures require TEXTURE2DMS view dimension.
			// Using TEXTURE2D for multi-sampled resources is invalid and
			// causes DX12 DEVICE_REMOVED on some drivers (Intel Iris Xe).
			rtvDesc = d3d12.D3D12_RENDER_TARGET_VIEW_DESC{
				Format:        dxgiFormat,
				ViewDimension: d3d12.D3D12_RTV_DIMENSION_TEXTURE2DMS,
			}
			// TEXTURE2DMS has no additional fields (no MipSlice, etc.)
		} else {
			rtvDesc = d3d12.D3D12_RENDER_TARGET_VIEW_DESC{
				Format:        dxgiFormat,
				ViewDimension: textureViewDimensionToRTV(viewDim),
			}
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

		var dsvDesc d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC
		if isMultisampled && viewDim == gputypes.TextureViewDimension2D {
			// MSAA depth/stencil textures require TEXTURE2DMS view dimension.
			dsvDesc = d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC{
				Format:        depthFormat,
				ViewDimension: d3d12.D3D12_DSV_DIMENSION_TEXTURE2DMS,
				Flags:         0,
			}
			// TEXTURE2DMS has no additional fields.
		} else {
			dsvDesc = d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC{
				Format:        depthFormat,
				ViewDimension: textureViewDimensionToDSV(viewDim),
				Flags:         0,
			}
			switch viewDim {
			case gputypes.TextureViewDimension1D:
				dsvDesc.SetTexture1D(baseMip)
			case gputypes.TextureViewDimension2D:
				dsvDesc.SetTexture2D(baseMip)
			case gputypes.TextureViewDimension2DArray:
				dsvDesc.SetTexture2DArray(baseMip, baseLayer, layerCount)
			}
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
		return nil, fmt.Errorf("BUG: sampler descriptor is nil in DX12.CreateSampler — core validation gap")
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

	// Allocate a slot in the shader-visible sampler heap (global sampler pool).
	// This is the index that gets written into sampler index buffers in bind groups,
	// matching Rust wgpu-hal's SamplerIndex architecture.
	poolCPU, poolGPU, err := d.samplerHeap.AllocateGPU(1)
	if err != nil {
		d.stagingSamplerHeap.Free(heapIndex, 1)
		return nil, fmt.Errorf("dx12: failed to allocate sampler pool slot: %w", err)
	}
	_ = poolGPU // GPU handle not needed directly; the shader uses the heap index
	poolSlot := d.samplerHeap.HandleToIndex(poolCPU)

	// Copy the sampler from staging to shader-visible heap.
	d.raw.CopyDescriptors(
		1, &poolCPU, nil,
		1, &handle, nil,
		d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_SAMPLER,
	)

	return &Sampler{
		handle:          handle,
		heapIndex:       heapIndex,
		samplerPoolSlot: poolSlot,
		device:          d,
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
		return nil, fmt.Errorf("BUG: bind group layout descriptor is nil in DX12.CreateBindGroupLayout — core validation gap")
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
// Uses the Rust wgpu-hal sampler heap pattern: sampler entries are collected
// as pool indices and written to a StructuredBuffer<uint> (sampler index buffer).
// The SRV for this buffer is placed in the CBV/SRV/UAV table so the shader can
// read nagaSamplerHeap[indexBuffer[binding_index]].
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: bind group descriptor is nil in DX12.CreateBindGroup — core validation gap")
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
	var viewEntries []gputypes.BindGroupEntry // CBV, SRV, UAV
	var samplerPoolIndices []uint32           // global sampler pool indices

	for _, entry := range desc.Entries {
		switch res := entry.Resource.(type) {
		case gputypes.SamplerBinding:
			sampler := (*Sampler)(unsafe.Pointer(res.Sampler)) //nolint:govet // intentional: HAL handle → concrete type
			samplerPoolIndices = append(samplerPoolIndices, sampler.samplerPoolSlot)
		default: // BufferBinding, TextureViewBinding
			viewEntries = append(viewEntries, entry)
		}
	}

	// Total view descriptors: regular entries + sampler index buffer SRV (if samplers present).
	totalViewDescs := uint32(len(viewEntries))
	if len(samplerPoolIndices) > 0 {
		totalViewDescs++ // +1 for sampler index buffer SRV
	}

	// Allocate and populate CBV/SRV/UAV descriptors (including sampler index buffer SRV).
	if err := d.populateBindGroupDescriptors(bg, totalViewDescs, viewEntries, samplerPoolIndices); err != nil {
		return nil, err
	}

	return bg, nil
}

// populateBindGroupDescriptors allocates view heap descriptors and writes CBV/SRV/UAV
// entries plus the sampler index buffer SRV into the contiguous GPU descriptor range.
func (d *Device) populateBindGroupDescriptors(bg *BindGroup, totalViewDescs uint32, viewEntries []gputypes.BindGroupEntry, samplerPoolIndices []uint32) error {
	if totalViewDescs == 0 || d.viewHeap == nil {
		return nil
	}

	cpuStart, gpuStart, err := d.viewHeap.AllocateGPU(totalViewDescs)
	if err != nil {
		return fmt.Errorf("dx12: failed to allocate view descriptors: %w", err)
	}
	bg.gpuDescHandle = gpuStart
	bg.viewHeapIndex = d.viewHeap.HandleToIndex(cpuStart)
	bg.viewCount = totalViewDescs

	if len(viewEntries) > 0 {
		if err := d.writeViewDescriptorsBatched(cpuStart, viewEntries); err != nil {
			return err
		}
	}

	// Create sampler index buffer and its SRV.
	if len(samplerPoolIndices) > 0 {
		indexBuf, err := d.createSamplerIndexBuffer(samplerPoolIndices)
		if err != nil {
			return fmt.Errorf("dx12: failed to create sampler index buffer: %w", err)
		}
		bg.samplerIndexBuffer = indexBuf

		// Create SRV for the sampler index buffer at the end of the view descriptors.
		srvDest := cpuStart.Offset(len(viewEntries), d.viewHeap.incrementSize)
		d.createBufferSRV(indexBuf, uint32(len(samplerPoolIndices)), 4, srvDest)
	}

	return nil
}

// createSamplerIndexBuffer creates a GPU buffer containing sampler pool indices.
// This is the StructuredBuffer<uint> that the shader reads to resolve sampler heap slots.
// Matches Rust wgpu-hal's sampler_index_buffer creation in create_bind_group.
func (d *Device) createSamplerIndexBuffer(indices []uint32) (*d3d12.ID3D12Resource, error) {
	bufferSize := uint64(len(indices) * 4) // uint32 = 4 bytes

	// Create an upload heap buffer (CPU-writable, GPU-readable).
	heapProps := d3d12.D3D12_HEAP_PROPERTIES{
		Type: d3d12.D3D12_HEAP_TYPE_UPLOAD,
	}
	resourceDesc := d3d12.D3D12_RESOURCE_DESC{
		Dimension:        d3d12.D3D12_RESOURCE_DIMENSION_BUFFER,
		Width:            bufferSize,
		Height:           1,
		DepthOrArraySize: 1,
		MipLevels:        1,
		SampleDesc:       d3d12.DXGI_SAMPLE_DESC{Count: 1},
		Layout:           d3d12.D3D12_TEXTURE_LAYOUT_ROW_MAJOR,
		Flags:            d3d12.D3D12_RESOURCE_FLAG_NONE,
	}

	resource, err := d.raw.CreateCommittedResource(
		&heapProps,
		d3d12.D3D12_HEAP_FLAG_NONE,
		&resourceDesc,
		d3d12.D3D12_RESOURCE_STATE_GENERIC_READ,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create committed resource: %w", err)
	}

	// Map, write indices, unmap.
	ptr, err := resource.Map(0, nil)
	if err != nil {
		resource.Release()
		return nil, fmt.Errorf("map sampler index buffer: %w", err)
	}

	dst := unsafe.Slice((*uint32)(ptr), len(indices))
	copy(dst, indices)
	resource.Unmap(0, nil)

	return resource, nil
}

// createBufferSRV creates a structured buffer SRV at the given CPU descriptor handle.
func (d *Device) createBufferSRV(resource *d3d12.ID3D12Resource, numElements, structureByteStride uint32, dest d3d12.D3D12_CPU_DESCRIPTOR_HANDLE) {
	srvDesc := d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC{
		Format:                  d3d12.DXGI_FORMAT_UNKNOWN,
		ViewDimension:           d3d12.D3D12_SRV_DIMENSION_BUFFER,
		Shader4ComponentMapping: d3d12.D3D12_DEFAULT_SHADER_4_COMPONENT_MAPPING,
	}
	// Set buffer SRV union: FirstElement=0, NumElements, StructureByteStride, Flags=0
	type bufferSRV struct {
		FirstElement        uint64
		NumElements         uint32
		StructureByteStride uint32
		Flags               uint32
	}
	buf := (*bufferSRV)(unsafe.Pointer(&srvDesc.Union[0]))
	buf.FirstElement = 0
	buf.NumElements = numElements
	buf.StructureByteStride = structureByteStride
	buf.Flags = 0

	d.raw.CreateShaderResourceView(resource, &srvDesc, dest)
}

// writeViewDescriptorsBatched writes CBV/SRV/UAV descriptors for all view entries.
// CBVs are created inline (cannot be batched), while SRV copies from scattered
// source handles are batched into a single CopyDescriptors call.
func (d *Device) writeViewDescriptorsBatched(cpuStart d3d12.D3D12_CPU_DESCRIPTOR_HANDLE, entries []gputypes.BindGroupEntry) error {
	// Collect SRV copy sources for batching.
	// destHandles[i] = destination in GPU-visible heap, srcHandles[i] = source from staging heap.
	var srvDestHandles []d3d12.D3D12_CPU_DESCRIPTOR_HANDLE
	var srvSrcHandles []d3d12.D3D12_CPU_DESCRIPTOR_HANDLE

	for i, entry := range entries {
		dest := cpuStart.Offset(i, d.viewHeap.incrementSize)

		switch res := entry.Resource.(type) {
		case gputypes.BufferBinding:
			buf := (*Buffer)(unsafe.Pointer(res.Buffer)) //nolint:govet // intentional: HAL handle → concrete type
			size := res.Size
			if size == 0 {
				size = buf.size - res.Offset
			}
			// Align CBV size to 256 bytes (D3D12 requirement).
			alignedSize := (size + 255) &^ 255
			d.raw.CreateConstantBufferView(&d3d12.D3D12_CONSTANT_BUFFER_VIEW_DESC{
				BufferLocation: buf.gpuVA + res.Offset,
				SizeInBytes:    uint32(alignedSize),
			}, dest)

		case gputypes.TextureViewBinding:
			view := (*TextureView)(unsafe.Pointer(res.TextureView)) //nolint:govet // intentional: HAL handle → concrete type
			if !view.hasSRV {
				return fmt.Errorf("dx12: texture view has no SRV for binding %d", entry.Binding)
			}
			srvDestHandles = append(srvDestHandles, dest)
			srvSrcHandles = append(srvSrcHandles, view.srvHandle)

		default:
			return fmt.Errorf("dx12: unsupported binding resource type: %T", entry.Resource)
		}
	}

	// Batch all SRV copies in a single CopyDescriptors call.
	// Each source is a separate range of size 1 (non-contiguous),
	// each destination is also a separate range of size 1 (may be non-contiguous
	// if interleaved with CBVs).
	if len(srvSrcHandles) > 0 {
		n := uint32(len(srvSrcHandles))
		d.raw.CopyDescriptors(
			n, &srvDestHandles[0], nil,
			n, &srvSrcHandles[0], nil,
			d3d12.D3D12_DESCRIPTOR_HEAP_TYPE_CBV_SRV_UAV,
		)
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
		return nil, fmt.Errorf("BUG: pipeline layout descriptor is nil in DX12.CreatePipelineLayout — core validation gap")
	}

	// Create root signature from bind group layouts
	result, err := d.createRootSignatureFromLayouts(desc.BindGroupLayouts)
	if err != nil {
		return nil, err
	}

	// Store references to bind group layouts
	bgLayouts := make([]*BindGroupLayout, len(desc.BindGroupLayouts))
	for i, l := range desc.BindGroupLayouts {
		bgLayout, ok := l.(*BindGroupLayout)
		if !ok {
			result.rootSignature.Release()
			return nil, fmt.Errorf("dx12: invalid bind group layout type at index %d", i)
		}
		bgLayouts[i] = bgLayout
	}

	if err := d.checkHealth("CreatePipelineLayout(" + desc.Label + ")"); err != nil {
		result.rootSignature.Release()
		return nil, err
	}

	hal.Logger().Debug("dx12: pipeline layout created",
		"label", desc.Label,
		"bindGroups", len(desc.BindGroupLayouts),
		"samplerRootIndex", result.samplerRootIndex,
	)

	return &PipelineLayout{
		rootSignature:    result.rootSignature,
		bindGroupLayouts: bgLayouts,
		groupMappings:    result.groupMappings,
		samplerRootIndex: result.samplerRootIndex,
		nagaOptions:      result.nagaOptions,
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
// For WGSL source, compilation is deferred to pipeline creation time when the
// PipelineLayout (with proper naga HLSL options) is available. This matches
// Rust wgpu-hal which compiles shaders in create_render_pipeline.
// For pre-compiled SPIR-V, bytecode is stored directly.
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: shader module descriptor is nil in DX12.CreateShaderModule — core validation gap")
	}

	module := &ShaderModule{
		entryPoints: make(map[string][]byte),
		device:      d,
	}

	switch {
	case desc.Source.WGSL != "":
		// Store raw WGSL for deferred compilation during pipeline creation.
		module.wgslSource = desc.Source.WGSL
		hal.Logger().Debug("dx12: shader module created (deferred compilation)",
			"source", "WGSL",
		)
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

	if err := d.checkHealth("CreateShaderModule(" + desc.Label + ")"); err != nil {
		return nil, err
	}
	return module, nil
}

// compileWGSLModule compiles WGSL source to per-entry-point bytecode.
// Routes to either DXIL direct compilation or HLSL→FXC based on Device.useDXIL.
// The naga options must come from the PipelineLayout, which contains the proper
// BindingMap and SamplerBufferBindingMap matching the root signature layout.
func (d *Device) compileWGSLModule(wgslSource string, nagaOpts *hlsl.Options, module *ShaderModule) error {
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

	if d.useDXIL {
		return d.compileWGSLModuleDXIL(wgslSource, irModule, nagaOpts, module)
	}
	return d.compileWGSLModuleHLSL(irModule, nagaOpts, module)
}

// hlslToDXILBindingMap converts an hlsl.BindingMap (used to build the
// D3D12 root signature for the HLSL→FXC path) into a dxil.BindingMap
// consumed by the naga DXIL backend. Both maps share the same
// (group, binding) → (space, register) contract — the only difference
// is the concrete struct types, since dxil does not import hlsl.
//
// Returns nil if the input is nil, preserving the "no remap" default.
func hlslToDXILBindingMap(m map[hlsl.ResourceBinding]hlsl.BindTarget) dxil.BindingMap {
	if m == nil {
		return nil
	}
	out := make(dxil.BindingMap, len(m))
	for k, v := range m {
		out[dxil.BindingLocation{Group: k.Group, Binding: k.Binding}] = dxil.BindTarget{
			Space:    uint32(v.Space),
			Register: v.Register,
		}
	}
	return out
}

func hlslToDXILSamplerBufferBindingMap(m map[uint32]hlsl.BindTarget) map[uint32]dxil.BindTarget {
	if m == nil {
		return nil
	}
	out := make(map[uint32]dxil.BindTarget, len(m))
	for group, v := range m {
		out[group] = dxil.BindTarget{
			Space:    uint32(v.Space),
			Register: v.Register,
		}
	}
	return out
}

func hlslToDXILSamplerHeapTargets(t hlsl.SamplerHeapBindTargets) *dxil.SamplerHeapBindTargets {
	return &dxil.SamplerHeapBindTargets{
		StandardSamplers: dxil.BindTarget{
			Space:    uint32(t.StandardSamplers.Space),
			Register: t.StandardSamplers.Register,
		},
		ComparisonSamplers: dxil.BindTarget{
			Space:    uint32(t.ComparisonSamplers.Space),
			Register: t.ComparisonSamplers.Register,
		},
	}
}

// compileWGSLModuleHLSL compiles IR to per-entry-point DXBC via HLSL→FXC.
// Pipeline: IR → HLSL → D3DCompile (d3dcompiler_47.dll) → DXBC
func (d *Device) compileWGSLModuleHLSL(irModule *ir.Module, nagaOpts *hlsl.Options, module *ShaderModule) error {
	// Generate HLSL using pipeline-specific naga options.
	// The options contain BindingMap (register assignments matching root signature)
	// and SamplerBufferBindingMap (sampler index buffer locations), ensuring the
	// shader's register usage matches the root signature exactly.
	hlslSource, info, err := hlsl.Compile(irModule, nagaOpts)
	if err != nil {
		return fmt.Errorf("HLSL generation: %w", err)
	}

	hal.Logger().Debug("dx12: compiling HLSL",
		"sourceLen", len(hlslSource),
		"entryPoints", len(irModule.EntryPoints),
	)

	// Load d3dcompiler_47.dll (deferred until cache miss)
	var compiler *d3dcompile.Lib

	// Compile each entry point separately, using shader cache.
	// Cache key = SHA-256(HLSL source) + entry point + stage + target.
	// This matches Rust wgpu's ShaderCache pattern (device.rs:390-428).
	for i := range irModule.EntryPoints {
		ep := &irModule.EntryPoints[i]
		target := shaderStageToTarget(ep.Stage)

		// Use the HLSL entry point name (naga may rename it)
		hlslName := ep.Name
		if info != nil && info.EntryPointNames != nil {
			if mapped, ok := info.EntryPointNames[ep.Name]; ok {
				hlslName = mapped
			}
		}

		// Check shader cache before calling FXC.
		cacheKey := NewShaderCacheKey(hlslSource, hlslName, ep.Stage, target)
		if cached, ok := d.shaderCache.Get(cacheKey); ok {
			module.entryPoints[ep.Name] = cached
			continue
		}

		// Cache miss — load compiler if not yet loaded and compile via FXC.
		if compiler == nil {
			compiler, err = d3dcompile.Load()
			if err != nil {
				return fmt.Errorf("load d3dcompiler: %w", err)
			}
		}

		bytecode, err := compiler.Compile(hlslSource, hlslName, target)
		if err != nil {
			return fmt.Errorf("D3DCompile entry point %q (hlsl: %q, target: %s): %w",
				ep.Name, hlslName, target, err)
		}

		// Store in cache for future pipelines using the same shader.
		d.shaderCache.Put(cacheKey, bytecode)
		module.entryPoints[ep.Name] = bytecode
	}

	return nil
}

// compileWGSLModuleDXIL compiles IR to per-entry-point DXIL bytecode directly.
// Pipeline: IR → naga dxil.Compile → DXBC container (with DXIL bitcode + BYPASS hash)
// Eliminates the HLSL→FXC dependency — no d3dcompiler_47.dll needed.
//
// The binding map from nagaOpts (built by PipelineLayout to match the D3D12
// root signature) is converted to dxil.BindingMap and passed to the DXIL
// backend so it emits (space, register) pairs matching the root signature.
// Without this, DXIL would use raw WGSL @group/@binding numbers and D3D12
// would reject the PSO with E_INVALIDARG (see FEAT-DXIL-002, BUG-DX12-011).
func (d *Device) compileWGSLModuleDXIL(wgslSource string, irModule *ir.Module, nagaOpts *hlsl.Options, module *ShaderModule) error {
	hal.Logger().Debug("dx12: compiling DXIL direct",
		"entryPoints", len(irModule.EntryPoints),
	)

	opts := dxil.DefaultOptions()
	if nagaOpts != nil {
		opts.BindingMap = hlslToDXILBindingMap(nagaOpts.BindingMap)
		opts.SamplerBufferBindingMap = hlslToDXILSamplerBufferBindingMap(nagaOpts.SamplerBufferBindingMap)
		opts.SamplerHeapTargets = hlslToDXILSamplerHeapTargets(nagaOpts.SamplerHeapTargets)
	}

	// Compile each entry point separately by creating a single-entry-point IR module.
	// dxil.Compile always compiles EntryPoints[0], so we isolate each entry point
	// into its own module copy. This matches the per-entry-point granularity of the
	// HLSL→FXC path and the shader cache.
	for i := range irModule.EntryPoints {
		ep := &irModule.EntryPoints[i]

		// Check shader cache before compiling.
		// DXIL cache key: SHA-256(WGSL source + entry point name) + "dxil" target.
		// Using WGSL source (not HLSL) because the DXIL path bypasses HLSL generation.
		cacheKey := NewShaderCacheKey(
			dxilCacheSource(wgslSource, ep.Name),
			ep.Name, ep.Stage, "dxil",
		)
		if cached, ok := d.shaderCache.Get(cacheKey); ok {
			module.entryPoints[ep.Name] = cached
			continue
		}

		// Create a single-entry-point module for dxil.Compile.
		// dxil.Compile always processes EntryPoints[0], so we need to isolate
		// the target entry point. Shared module data (types, constants, globals)
		// is referenced, not copied — only the EntryPoints slice differs.
		singleEPModule := &ir.Module{
			Types:             irModule.Types,
			Constants:         irModule.Constants,
			GlobalVariables:   irModule.GlobalVariables,
			GlobalExpressions: irModule.GlobalExpressions,
			Functions:         irModule.Functions,
			EntryPoints:       []ir.EntryPoint{irModule.EntryPoints[i]},
			Overrides:         irModule.Overrides,
			SpecialTypes:      irModule.SpecialTypes,
		}

		dxilBytes, err := dxil.Compile(singleEPModule, opts)
		if err != nil {
			return fmt.Errorf("dx12: DXIL compile entry point %q: %w", ep.Name, err)
		}

		// Pre-validate via naga's IDxcValidator wrapper when
		// GOGPU_DX12_DXIL_VALIDATE=1 is set. Surfaces the exact
		// HRESULT+text that dxil.dll would emit, instead of the
		// opaque E_INVALIDARG that D3D12 CreateGraphicsPipelineState
		// folds everything into. Also dumps the offending blob to
		// tmp/ for post-mortem dxilval --corpus replay.
		if d.dxilValidate {
			if verr := dxil.Validate(dxilBytes, dxil.ValidateFull); verr != nil {
				_ = os.WriteFile(
					fmt.Sprintf("D:/projects/gogpu/wgpu/tmp/dbg_stage%d_%s.dxil", int(ep.Stage), ep.Name),
					dxilBytes, 0o644,
				)
				return fmt.Errorf("dx12: DXIL validation rejected %q (stage %d): %w", ep.Name, ep.Stage, verr)
			}
		}

		// Store in cache for future pipelines using the same shader.
		d.shaderCache.Put(cacheKey, dxilBytes)
		module.entryPoints[ep.Name] = dxilBytes
	}

	return nil
}

// dxilCacheSource creates a unique cache source string for DXIL compilation.
// Combines WGSL source with entry point name to ensure different entry points
// from the same WGSL source get distinct cache keys.
func dxilCacheSource(wgslSource, entryPoint string) string {
	// Use SHA-256 of combined source to keep the key compact while unique.
	// NewShaderCacheKey will hash this again, but the intermediate string
	// needs to be deterministic and unique per (source, entry) pair.
	h := sha256.Sum256([]byte(wgslSource + "\x00" + entryPoint))
	return string(h[:])
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

// ensureShaderCompiled performs deferred WGSL compilation if needed.
// Must be called during pipeline creation when the PipelineLayout is available.
func (d *Device) ensureShaderCompiled(module *ShaderModule, layout *PipelineLayout) error {
	if !module.IsDeferred() {
		return nil // Already compiled (SPIR-V or previously compiled WGSL)
	}
	if layout == nil || layout.nagaOptions == nil {
		return fmt.Errorf("dx12: deferred WGSL compilation requires PipelineLayout with naga options")
	}
	return d.compileWGSLModule(module.wgslSource, layout.nagaOptions, module)
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	start := time.Now()
	if desc == nil {
		return nil, fmt.Errorf("BUG: render pipeline descriptor is nil in DX12.CreateRenderPipeline — core validation gap")
	}

	// Deferred shader compilation: compile WGSL with pipeline-specific naga options.
	var pipelineLayout *PipelineLayout
	if desc.Layout != nil {
		if pl, ok := desc.Layout.(*PipelineLayout); ok {
			pipelineLayout = pl
		}
	}
	if desc.Vertex.Module != nil {
		if sm, ok := desc.Vertex.Module.(*ShaderModule); ok {
			if err := d.ensureShaderCompiled(sm, pipelineLayout); err != nil {
				return nil, fmt.Errorf("dx12: vertex shader compilation failed: %w", err)
			}
		}
	}
	if desc.Fragment != nil && desc.Fragment.Module != nil {
		if sm, ok := desc.Fragment.Module.(*ShaderModule); ok {
			if err := d.ensureShaderCompiled(sm, pipelineLayout); err != nil {
				return nil, fmt.Errorf("dx12: fragment shader compilation failed: %w", err)
			}
		}
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
	d.DrainDebugMessages() // Check for validation warnings/errors during PSO creation
	if err != nil {
		slog.Error("dx12: CreateGraphicsPipelineState failed",
			"label", desc.Label,
			"vs", desc.Vertex.EntryPoint,
			"fs", func() string {
				if desc.Fragment != nil {
					return desc.Fragment.EntryPoint
				}
				return ""
			}(),
			"vertexBuffers", len(desc.Vertex.Buffers),
			"err", err,
		)
		if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
			d.logDREDBreadcrumbs()
			return nil, fmt.Errorf("dx12: CreateGraphicsPipelineState failed (device removed: %w): %w", reason, err)
		}
		return nil, fmt.Errorf("dx12: CreateGraphicsPipelineState failed: %w", err)
	}

	// Get root signature reference and group mappings for command list binding.
	// Must match the root signature used in the PSO.
	var rootSig *d3d12.ID3D12RootSignature
	var groupMappings []rootParamMapping
	samplerRootIdx := -1
	if pipelineLayout != nil {
		rootSig = pipelineLayout.rootSignature
		groupMappings = pipelineLayout.groupMappings
		samplerRootIdx = pipelineLayout.samplerRootIndex
	} else {
		// No layout → use the same empty root signature that was used in the PSO.
		rootSig, _ = d.getOrCreateEmptyRootSignature()
	}

	// Calculate vertex strides for IASetVertexBuffers
	vertexStrides := make([]uint32, len(desc.Vertex.Buffers))
	for i, buf := range desc.Vertex.Buffers {
		vertexStrides[i] = uint32(buf.ArrayStride)
	}

	if err := d.checkHealth("CreateRenderPipeline(" + desc.Label + ")"); err != nil {
		pso.Release()
		return nil, err
	}

	hal.Logger().Debug("dx12: render pipeline created",
		"label", desc.Label,
		"vertexEntry", desc.Vertex.EntryPoint,
		"elapsed", time.Since(start),
	)

	return &RenderPipeline{
		pso:              pso,
		rootSignature:    rootSig,
		groupMappings:    groupMappings,
		samplerRootIndex: samplerRootIdx,
		topology:         primitiveTopologyToD3D12(desc.Primitive.Topology),
		vertexStrides:    vertexStrides,
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
	start := time.Now()
	if desc == nil {
		return nil, fmt.Errorf("BUG: compute pipeline descriptor is nil in DX12.CreateComputePipeline — core validation gap")
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
	var pipelineLayout *PipelineLayout
	samplerRootIdx := -1
	if desc.Layout != nil {
		pl, ok := desc.Layout.(*PipelineLayout)
		if !ok {
			return nil, fmt.Errorf("dx12: invalid pipeline layout type")
		}
		pipelineLayout = pl
		rootSig = pl.rootSignature
		groupMappings = pl.groupMappings
		samplerRootIdx = pl.samplerRootIndex
	} else {
		emptyRS, err := d.getOrCreateEmptyRootSignature()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to get empty root signature for compute: %w", err)
		}
		rootSig = emptyRS
	}

	// Deferred shader compilation: compile WGSL with pipeline-specific naga options.
	if err := d.ensureShaderCompiled(shaderModule, pipelineLayout); err != nil {
		return nil, fmt.Errorf("dx12: compute shader compilation failed: %w", err)
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
	d.DrainDebugMessages() // Check for validation warnings/errors during PSO creation
	if err != nil {
		slog.Error("dx12: CreateComputePipelineState failed",
			"label", desc.Label,
			"entryPoint", desc.Compute.EntryPoint,
			"err", err,
		)
		if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
			d.logDREDBreadcrumbs()
			return nil, fmt.Errorf("dx12: CreateComputePipelineState failed (device removed: %w): %w", reason, err)
		}
		return nil, fmt.Errorf("dx12: CreateComputePipelineState failed: %w", err)
	}

	if err := d.checkHealth("CreateComputePipeline"); err != nil {
		pso.Release()
		return nil, err
	}

	hal.Logger().Debug("dx12: compute pipeline created",
		"entryPoint", desc.Compute.EntryPoint,
		"elapsed", time.Since(start),
	)

	return &ComputePipeline{
		pso:              pso,
		rootSignature:    rootSig,
		groupMappings:    groupMappings,
		samplerRootIndex: samplerRootIdx,
	}, nil
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	if p, ok := pipeline.(*ComputePipeline); ok && p != nil {
		p.Destroy()
	}
}

// CreateQuerySet creates a query set.
// TODO: implement using ID3D12Device::CreateQueryHeap for full timestamp support.
func (d *Device) CreateQuerySet(_ *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	return nil, hal.ErrTimestampsNotSupported
}

// DestroyQuerySet destroys a query set.
func (d *Device) DestroyQuerySet(_ hal.QuerySet) {
	// Stub: DX12 query set implementation pending.
}

// CreateCommandEncoder creates a command encoder.
// The encoder is a lightweight shell — command allocator and list are created
// lazily in BeginEncoding, enabling per-frame allocator pooling.
func (d *Device) CreateCommandEncoder(desc *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	var label string
	if desc != nil {
		label = desc.Label
	}

	// Each encoder permanently owns its own allocator (Rust wgpu-hal pattern).
	// The allocator is Reset only via ResetAll after GPU completion.
	alloc, err := d.raw.CreateCommandAllocator(d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateCommandAllocator failed: %w", err)
	}

	return &CommandEncoder{
		device:    d,
		allocator: alloc,
		label:     label,
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
	if cb, ok := cmdBuffer.(*CommandBuffer); ok && cb != nil {
		cb.Destroy()
	}
}

// CreateRenderBundleEncoder creates a render bundle encoder.
// Note: DX12 supports bundles natively, but not yet implemented.
func (d *Device) CreateRenderBundleEncoder(desc *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("dx12: render bundles not yet implemented")
}

// DestroyRenderBundle destroys a render bundle.
func (d *Device) DestroyRenderBundle(bundle hal.RenderBundle) {}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	return d.waitForGPU()
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
