// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/memory"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// Device implements hal.Device for Vulkan.
type Device struct {
	handle         vk.Device
	physicalDevice vk.PhysicalDevice
	instance       *Instance
	graphicsFamily uint32
	allocator      *memory.GpuAllocator
	cmds           *vk.Commands
	commandPool    vk.CommandPool // Primary command pool for encoder allocation
}

// initAllocator initializes the memory allocator for this device.
func (d *Device) initAllocator() error {
	// Get physical device memory properties
	var vkProps vk.PhysicalDeviceMemoryProperties
	vk.GetPhysicalDeviceMemoryProperties(&d.instance.cmds, d.physicalDevice, &vkProps)

	// Convert to our format
	props := memory.DeviceMemoryProperties{
		MemoryTypes: make([]memory.MemoryType, vkProps.MemoryTypeCount),
		MemoryHeaps: make([]memory.MemoryHeap, vkProps.MemoryHeapCount),
	}

	for i := uint32(0); i < vkProps.MemoryTypeCount; i++ {
		props.MemoryTypes[i] = memory.MemoryType{
			PropertyFlags: vkProps.MemoryTypes[i].PropertyFlags,
			HeapIndex:     vkProps.MemoryTypes[i].HeapIndex,
		}
	}

	for i := uint32(0); i < vkProps.MemoryHeapCount; i++ {
		props.MemoryHeaps[i] = memory.MemoryHeap{
			Size:  uint64(vkProps.MemoryHeaps[i].Size),
			Flags: vkProps.MemoryHeaps[i].Flags,
		}
	}

	// Create allocator with default config
	allocator, err := memory.NewGpuAllocator(d.handle, props, memory.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to create memory allocator: %w", err)
	}

	d.allocator = allocator

	// Set device commands for memory operations
	vk.SetDeviceCommands(d.cmds)

	return nil
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: buffer descriptor is nil")
	}
	if desc.Size == 0 {
		return nil, fmt.Errorf("vulkan: buffer size must be > 0")
	}

	// Convert usage flags
	vkUsage := bufferUsageToVk(desc.Usage)

	// Create VkBuffer (without memory)
	createInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        vk.DeviceSize(desc.Size),
		Usage:       vkUsage,
		SharingMode: vk.SharingModeExclusive,
	}

	var buffer vk.Buffer
	result := vk.CreateBuffer(d.handle, &createInfo, nil, &buffer)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateBuffer failed: %d", result)
	}

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	vk.GetBufferMemoryRequirements(d.handle, buffer, &memReqs)

	// Determine usage flags for memory allocation
	memUsage := memory.UsageFastDeviceAccess
	if desc.Usage&(types.BufferUsageMapRead|types.BufferUsageMapWrite) != 0 {
		memUsage = memory.UsageHostAccess
		if desc.Usage&types.BufferUsageMapRead != 0 {
			memUsage |= memory.UsageDownload
		}
		if desc.Usage&types.BufferUsageMapWrite != 0 {
			memUsage |= memory.UsageUpload
		}
	}

	// Allocate memory
	memBlock, err := d.allocator.Alloc(memory.AllocationRequest{
		Size:           uint64(memReqs.Size),
		Alignment:      uint64(memReqs.Alignment),
		Usage:          memUsage,
		MemoryTypeBits: memReqs.MemoryTypeBits,
	})
	if err != nil {
		vk.DestroyBuffer(d.handle, buffer, nil)
		return nil, fmt.Errorf("vulkan: failed to allocate buffer memory: %w", err)
	}

	// Bind memory to buffer
	result = vk.BindBufferMemory(d.handle, buffer, memBlock.Memory, memBlock.Offset)
	if result != vk.Success {
		_ = d.allocator.Free(memBlock)
		vk.DestroyBuffer(d.handle, buffer, nil)
		return nil, fmt.Errorf("vulkan: vkBindBufferMemory failed: %d", result)
	}

	return &Buffer{
		handle: buffer,
		memory: memBlock,
		size:   desc.Size,
		usage:  desc.Usage,
		device: d,
	}, nil
}

// DestroyBuffer destroys a GPU buffer.
func (d *Device) DestroyBuffer(buffer hal.Buffer) {
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer == nil {
		return
	}

	if vkBuffer.handle != 0 {
		vk.DestroyBuffer(d.handle, vkBuffer.handle, nil)
		vkBuffer.handle = 0
	}

	if vkBuffer.memory != nil {
		_ = d.allocator.Free(vkBuffer.memory)
		vkBuffer.memory = nil
	}

	vkBuffer.device = nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: texture descriptor is nil")
	}
	if desc.Size.Width == 0 || desc.Size.Height == 0 {
		return nil, fmt.Errorf("vulkan: texture size must be > 0")
	}

	// Convert parameters
	vkFormat := textureFormatToVk(desc.Format)
	vkUsage := textureUsageToVk(desc.Usage)
	imageType := textureDimensionToVkImageType(desc.Dimension)

	// Determine depth/array layers
	depth := desc.Size.DepthOrArrayLayers
	if depth == 0 {
		depth = 1
	}
	mipLevels := desc.MipLevelCount
	if mipLevels == 0 {
		mipLevels = 1
	}
	samples := desc.SampleCount
	if samples == 0 {
		samples = 1
	}

	// Create VkImage (without memory)
	createInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: imageType,
		Format:    vkFormat,
		Extent: vk.Extent3D{
			Width:  desc.Size.Width,
			Height: desc.Size.Height,
			Depth:  depth,
		},
		MipLevels:     mipLevels,
		ArrayLayers:   1, // TODO: Support array textures
		Samples:       vk.SampleCountFlagBits(samples),
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vkUsage,
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var image vk.Image
	result := vk.CreateImage(d.handle, &createInfo, nil, &image)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateImage failed: %d", result)
	}

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	vk.GetImageMemoryRequirements(d.handle, image, &memReqs)

	// Allocate memory (textures always use device-local)
	memBlock, err := d.allocator.Alloc(memory.AllocationRequest{
		Size:           uint64(memReqs.Size),
		Alignment:      uint64(memReqs.Alignment),
		Usage:          memory.UsageFastDeviceAccess,
		MemoryTypeBits: memReqs.MemoryTypeBits,
	})
	if err != nil {
		vk.DestroyImage(d.handle, image, nil)
		return nil, fmt.Errorf("vulkan: failed to allocate texture memory: %w", err)
	}

	// Bind memory to image
	result = vk.BindImageMemory(d.handle, image, memBlock.Memory, memBlock.Offset)
	if result != vk.Success {
		_ = d.allocator.Free(memBlock)
		vk.DestroyImage(d.handle, image, nil)
		return nil, fmt.Errorf("vulkan: vkBindImageMemory failed: %d", result)
	}

	return &Texture{
		handle:    image,
		memory:    memBlock,
		size:      Extent3D{Width: desc.Size.Width, Height: desc.Size.Height, Depth: depth},
		format:    desc.Format,
		usage:     desc.Usage,
		mipLevels: mipLevels,
		samples:   samples,
		dimension: desc.Dimension,
		device:    d,
	}, nil
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(texture hal.Texture) {
	vkTexture, ok := texture.(*Texture)
	if !ok || vkTexture == nil {
		return
	}

	if vkTexture.handle != 0 && !vkTexture.isExternal {
		vk.DestroyImage(d.handle, vkTexture.handle, nil)
		vkTexture.handle = 0
	}

	if vkTexture.memory != nil {
		_ = d.allocator.Free(vkTexture.memory)
		vkTexture.memory = nil
	}

	vkTexture.device = nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture hal.Texture, desc *hal.TextureViewDescriptor) (hal.TextureView, error) {
	// TODO: Implement VkImageView creation
	return nil, fmt.Errorf("vulkan: CreateTextureView not implemented")
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	// TODO: Implement
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *hal.SamplerDescriptor) (hal.Sampler, error) {
	// TODO: Implement VkSampler creation
	return nil, fmt.Errorf("vulkan: CreateSampler not implemented")
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	// TODO: Implement
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	// TODO: Implement VkDescriptorSetLayout creation
	return nil, fmt.Errorf("vulkan: CreateBindGroupLayout not implemented")
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(layout hal.BindGroupLayout) {
	// TODO: Implement
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	// TODO: Implement VkDescriptorSet allocation
	return nil, fmt.Errorf("vulkan: CreateBindGroup not implemented")
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(group hal.BindGroup) {
	// TODO: Implement
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	// TODO: Implement VkPipelineLayout creation
	return nil, fmt.Errorf("vulkan: CreatePipelineLayout not implemented")
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {
	// TODO: Implement
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	// TODO: Implement VkShaderModule creation from SPIR-V
	return nil, fmt.Errorf("vulkan: CreateShaderModule not implemented")
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	// TODO: Implement
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	// TODO: Implement VkPipeline creation
	return nil, fmt.Errorf("vulkan: CreateRenderPipeline not implemented")
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	// TODO: Implement
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	// TODO: Implement VkPipeline creation for compute
	return nil, fmt.Errorf("vulkan: CreateComputePipeline not implemented")
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	// TODO: Implement
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(desc *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	// Ensure command pool exists
	if d.commandPool == 0 {
		if err := d.initCommandPool(); err != nil {
			return nil, err
		}
	}

	// Allocate command buffer
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        d.commandPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}

	var cmdBuffer vk.CommandBuffer
	result := vkAllocateCommandBuffers(d.cmds, d.handle, &allocInfo, &cmdBuffer)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkAllocateCommandBuffers failed: %d", result)
	}

	pool := &CommandPool{
		handle: d.commandPool,
		device: d,
	}

	return &CommandEncoder{
		device:    d,
		pool:      pool,
		cmdBuffer: cmdBuffer,
		label:     desc.Label,
	}, nil
}

// initCommandPool initializes the device command pool.
func (d *Device) initCommandPool() error {
	createInfo := vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		Flags:            vk.CommandPoolCreateFlags(vk.CommandPoolCreateResetCommandBufferBit),
		QueueFamilyIndex: d.graphicsFamily,
	}

	var pool vk.CommandPool
	result := vkCreateCommandPool(d.cmds, d.handle, &createInfo, nil, &pool)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkCreateCommandPool failed: %d", result)
	}

	d.commandPool = pool
	return nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	// TODO: Implement VkFence or VkSemaphore
	return nil, fmt.Errorf("vulkan: CreateFence not implemented")
}

// DestroyFence destroys a fence.
func (d *Device) DestroyFence(fence hal.Fence) {
	// TODO: Implement
}

// Wait waits for a fence to reach the specified value.
func (d *Device) Wait(fence hal.Fence, value uint64, timeout time.Duration) (bool, error) {
	// TODO: Implement vkWaitForFences
	return false, fmt.Errorf("vulkan: Wait not implemented")
}

// Destroy releases the device.
func (d *Device) Destroy() {
	if d.commandPool != 0 {
		vkDestroyCommandPool(d.cmds, d.handle, d.commandPool, nil)
		d.commandPool = 0
	}

	if d.allocator != nil {
		d.allocator.Destroy()
		d.allocator = nil
	}

	if d.handle != 0 {
		vkDestroyDevice(d.handle, nil)
		d.handle = 0
	}
}

// Vulkan function wrapper

func vkDestroyDevice(device vk.Device, allocator unsafe.Pointer) {
	proc := vk.GetInstanceProcAddr(0, "vkDestroyDevice")
	if proc == 0 {
		return
	}
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(proc,
		uintptr(device),
		uintptr(allocator))
}

func vkCreateCommandPool(cmds *vk.Commands, device vk.Device, createInfo *vk.CommandPoolCreateInfo, allocator unsafe.Pointer, pool *vk.CommandPool) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.CreateCommandPool(),
		uintptr(device),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(pool)))
	return vk.Result(ret)
}

func vkDestroyCommandPool(cmds *vk.Commands, device vk.Device, pool vk.CommandPool, allocator unsafe.Pointer) {
	//nolint:errcheck // Vulkan void function, no return value to check
	syscall.SyscallN(cmds.DestroyCommandPool(),
		uintptr(device),
		uintptr(pool),
		uintptr(allocator))
}

func vkAllocateCommandBuffers(cmds *vk.Commands, device vk.Device, allocInfo *vk.CommandBufferAllocateInfo, cmdBuffers *vk.CommandBuffer) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.AllocateCommandBuffers(),
		uintptr(device),
		uintptr(unsafe.Pointer(allocInfo)),
		uintptr(unsafe.Pointer(cmdBuffers)))
	return vk.Result(ret)
}
