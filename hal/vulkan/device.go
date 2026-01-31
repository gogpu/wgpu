// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/memory"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Device implements hal.Device for Vulkan.
type Device struct {
	handle              vk.Device
	physicalDevice      vk.PhysicalDevice
	instance            *Instance
	graphicsFamily      uint32
	allocator           *memory.GpuAllocator
	cmds                *vk.Commands
	commandPool         vk.CommandPool       // Primary command pool for encoder allocation
	descriptorAllocator *DescriptorAllocator // Descriptor pool management for bind groups
	queue               *Queue               // Primary queue (for swapchain synchronization)
	renderPassCache     *RenderPassCache     // Cache for VkRenderPass and VkFramebuffer objects
}

// initAllocator initializes the memory allocator for this device.
func (d *Device) initAllocator() error {
	// Get physical device memory properties
	var vkProps vk.PhysicalDeviceMemoryProperties
	d.instance.cmds.GetPhysicalDeviceMemoryProperties(d.physicalDevice, &vkProps)

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
	allocator, err := memory.NewGpuAllocator(d.handle, d.cmds, props, memory.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to create memory allocator: %w", err)
	}

	d.allocator = allocator

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
	result := d.cmds.CreateBuffer(d.handle, &createInfo, nil, &buffer)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateBuffer failed: %d", result)
	}

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	d.cmds.GetBufferMemoryRequirements(d.handle, buffer, &memReqs)

	// Determine usage flags for memory allocation
	// For CopyDst buffers or MappedAtCreation, we need host-visible memory
	// so that WriteBuffer can write directly without staging
	memUsage := memory.UsageFastDeviceAccess
	if desc.Usage&(gputypes.BufferUsageMapRead|gputypes.BufferUsageMapWrite) != 0 ||
		desc.Usage&gputypes.BufferUsageCopyDst != 0 || desc.MappedAtCreation {
		memUsage = memory.UsageHostAccess | memory.UsageUpload
		if desc.Usage&gputypes.BufferUsageMapRead != 0 {
			memUsage |= memory.UsageDownload
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
		d.cmds.DestroyBuffer(d.handle, buffer, nil)
		return nil, fmt.Errorf("vulkan: failed to allocate buffer memory: %w", err)
	}

	// Bind memory to buffer
	result = d.cmds.BindBufferMemory(d.handle, buffer, memBlock.Memory, vk.DeviceSize(memBlock.Offset))
	if result != vk.Success {
		_ = d.allocator.Free(memBlock)
		d.cmds.DestroyBuffer(d.handle, buffer, nil)
		return nil, fmt.Errorf("vulkan: vkBindBufferMemory failed: %d", result)
	}

	// Map memory for host-visible buffers so WriteBuffer can write directly
	if memUsage&memory.UsageHostAccess != 0 {
		var mappedPtr uintptr
		result = d.cmds.MapMemory(d.handle, memBlock.Memory, vk.DeviceSize(memBlock.Offset),
			vk.DeviceSize(desc.Size), 0, uintptr(unsafe.Pointer(&mappedPtr)))
		if result != vk.Success {
			_ = d.allocator.Free(memBlock)
			d.cmds.DestroyBuffer(d.handle, buffer, nil)
			return nil, fmt.Errorf("vulkan: vkMapMemory failed: %d", result)
		}
		memBlock.MappedPtr = mappedPtr
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
		d.cmds.DestroyBuffer(d.handle, vkBuffer.handle, nil)
		vkBuffer.handle = 0
	}

	if vkBuffer.memory != nil {
		// Unmap memory if it was mapped
		if vkBuffer.memory.MappedPtr != 0 {
			d.cmds.UnmapMemory(d.handle, vkBuffer.memory.Memory)
			vkBuffer.memory.MappedPtr = 0
		}
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
		ArrayLayers:   1, // Note(v0.6.0): Array textures require VkImageViewType mapping.
		Samples:       vk.SampleCountFlagBits(samples),
		Tiling:        vk.ImageTilingOptimal,
		Usage:         vkUsage,
		SharingMode:   vk.SharingModeExclusive,
		InitialLayout: vk.ImageLayoutUndefined,
	}

	var image vk.Image
	result := d.cmds.CreateImage(d.handle, &createInfo, nil, &image)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateImage failed: %d", result)
	}

	// Get memory requirements
	var memReqs vk.MemoryRequirements
	d.cmds.GetImageMemoryRequirements(d.handle, image, &memReqs)

	// Allocate memory (textures always use device-local)
	memBlock, err := d.allocator.Alloc(memory.AllocationRequest{
		Size:           uint64(memReqs.Size),
		Alignment:      uint64(memReqs.Alignment),
		Usage:          memory.UsageFastDeviceAccess,
		MemoryTypeBits: memReqs.MemoryTypeBits,
	})
	if err != nil {
		d.cmds.DestroyImage(d.handle, image, nil)
		return nil, fmt.Errorf("vulkan: failed to allocate texture memory: %w", err)
	}

	// Bind memory to image
	result = d.cmds.BindImageMemory(d.handle, image, memBlock.Memory, vk.DeviceSize(memBlock.Offset))
	if result != vk.Success {
		_ = d.allocator.Free(memBlock)
		d.cmds.DestroyImage(d.handle, image, nil)
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
		d.cmds.DestroyImage(d.handle, vkTexture.handle, nil)
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
	// Extract image handle and metadata based on texture type.
	// We support both regular Texture and SwapchainTexture.
	var (
		imageHandle   vk.Image
		textureFormat gputypes.TextureFormat
		dimension     gputypes.TextureDimension
		mipLevels     uint32
		textureSize   Extent3D
	)

	switch t := texture.(type) {
	case *Texture:
		if t == nil {
			return nil, fmt.Errorf("vulkan: nil texture")
		}
		imageHandle = t.handle
		textureFormat = t.format
		dimension = t.dimension
		mipLevels = t.mipLevels
		textureSize = t.size
	case *SwapchainTexture:
		if t == nil {
			return nil, fmt.Errorf("vulkan: nil swapchain texture")
		}
		// For swapchain textures, reuse the pre-created view from the swapchain.
		// Creating new views for swapchain images can cause rendering issues.
		return &TextureView{
			handle:      t.view,
			texture:     nil,
			device:      d,
			size:        t.size,
			image:       t.handle,
			isSwapchain: true,
			vkFormat:    textureFormatToVk(t.format),
		}, nil
	default:
		return nil, fmt.Errorf("vulkan: invalid texture type %T", texture)
	}

	// Handle nil descriptor - use defaults from texture
	if desc == nil {
		desc = &hal.TextureViewDescriptor{}
	}

	// Determine format - use texture format if not specified
	format := desc.Format
	if format == gputypes.TextureFormatUndefined {
		format = textureFormat
	}

	// Determine view type - derive from texture dimension if not specified
	var viewType vk.ImageViewType
	if desc.Dimension == gputypes.TextureViewDimensionUndefined {
		viewType = textureDimensionToViewType(dimension)
	} else {
		viewType = textureViewDimensionToVk(desc.Dimension)
	}

	// Determine mip level count
	mipLevelCount := desc.MipLevelCount
	if mipLevelCount == 0 {
		mipLevelCount = mipLevels - desc.BaseMipLevel
	}

	// Determine array layer count
	arrayLayerCount := desc.ArrayLayerCount
	if arrayLayerCount == 0 {
		arrayLayerCount = 1
		// For cube maps and arrays, calculate appropriate layer count
		switch viewType {
		case vk.ImageViewTypeCube:
			arrayLayerCount = 6
		case vk.ImageViewTypeCubeArray, vk.ImageViewType2dArray, vk.ImageViewType1dArray:
			arrayLayerCount = textureSize.Depth - desc.BaseArrayLayer
		}
	}

	createInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    imageHandle,
		ViewType: viewType,
		Format:   textureFormatToVk(format),
		Components: vk.ComponentMapping{
			R: vk.ComponentSwizzleIdentity,
			G: vk.ComponentSwizzleIdentity,
			B: vk.ComponentSwizzleIdentity,
			A: vk.ComponentSwizzleIdentity,
		},
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     textureAspectToVk(desc.Aspect, format),
			BaseMipLevel:   desc.BaseMipLevel,
			LevelCount:     mipLevelCount,
			BaseArrayLayer: desc.BaseArrayLayer,
			LayerCount:     arrayLayerCount,
		},
	}

	var imageView vk.ImageView
	result := vkCreateImageView(d.cmds, d.handle, &createInfo, nil, &imageView)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateImageView failed: %d", result)
	}

	// Store texture reference and track if this is a swapchain image.
	var texRef *Texture
	var isSwapchain bool
	switch t := texture.(type) {
	case *Texture:
		texRef = t
	case *SwapchainTexture:
		isSwapchain = true
	}

	return &TextureView{
		handle:      imageView,
		texture:     texRef,
		device:      d,
		size:        textureSize,
		image:       imageHandle,
		isSwapchain: isSwapchain,
	}, nil
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	vkView, ok := view.(*TextureView)
	if !ok || vkView == nil {
		return
	}

	// Don't destroy swapchain views - they're owned by the swapchain
	if vkView.isSwapchain {
		vkView.device = nil
		return
	}

	if vkView.handle != 0 {
		vkDestroyImageView(d.cmds, d.handle, vkView.handle, nil)
		vkView.handle = 0
	}

	vkView.device = nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *hal.SamplerDescriptor) (hal.Sampler, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: sampler descriptor is nil")
	}

	// Determine if comparison is enabled
	compareEnable := vk.Bool32(vk.False)
	compareOp := vk.CompareOpNever
	if desc.Compare != gputypes.CompareFunctionUndefined {
		compareEnable = vk.Bool32(vk.True)
		compareOp = compareFunctionToVk(desc.Compare)
	}

	// Determine if anisotropy is enabled
	anisotropyEnable := vk.Bool32(vk.False)
	maxAnisotropy := float32(1.0)
	if desc.Anisotropy > 1 {
		anisotropyEnable = vk.Bool32(vk.True)
		maxAnisotropy = float32(desc.Anisotropy)
	}

	// LOD clamp values
	lodMinClamp := desc.LodMinClamp
	lodMaxClamp := desc.LodMaxClamp
	if lodMaxClamp == 0 {
		lodMaxClamp = vk.LodClampNone
	}

	createInfo := vk.SamplerCreateInfo{
		SType:                   vk.StructureTypeSamplerCreateInfo,
		MagFilter:               filterModeToVk(desc.MagFilter),
		MinFilter:               filterModeToVk(desc.MinFilter),
		MipmapMode:              mipmapFilterModeToVk(desc.MipmapFilter),
		AddressModeU:            addressModeToVk(desc.AddressModeU),
		AddressModeV:            addressModeToVk(desc.AddressModeV),
		AddressModeW:            addressModeToVk(desc.AddressModeW),
		MipLodBias:              0.0,
		AnisotropyEnable:        anisotropyEnable,
		MaxAnisotropy:           maxAnisotropy,
		CompareEnable:           compareEnable,
		CompareOp:               compareOp,
		MinLod:                  lodMinClamp,
		MaxLod:                  lodMaxClamp,
		BorderColor:             vk.BorderColorFloatTransparentBlack,
		UnnormalizedCoordinates: vk.Bool32(vk.False),
	}

	var sampler vk.Sampler
	result := vkCreateSampler(d.cmds, d.handle, &createInfo, nil, &sampler)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateSampler failed: %d", result)
	}

	return &Sampler{
		handle: sampler,
		device: d,
	}, nil
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	vkSampler, ok := sampler.(*Sampler)
	if !ok || vkSampler == nil {
		return
	}

	if vkSampler.handle != 0 {
		vkDestroySampler(d.cmds, d.handle, vkSampler.handle, nil)
		vkSampler.handle = 0
	}

	vkSampler.device = nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: bind group layout descriptor is nil")
	}

	// Convert entries to Vulkan descriptor set layout bindings and track counts
	bindings := make([]vk.DescriptorSetLayoutBinding, 0, len(desc.Entries))
	var counts DescriptorCounts

	for _, entry := range desc.Entries {
		binding := vk.DescriptorSetLayoutBinding{
			Binding:         entry.Binding,
			DescriptorCount: 1,
			StageFlags:      shaderStagesToVk(entry.Visibility),
		}

		// Determine descriptor type based on which binding is set
		switch {
		case entry.Buffer != nil:
			binding.DescriptorType = bufferBindingTypeToVk(entry.Buffer.Type)
			if entry.Buffer.Type == gputypes.BufferBindingTypeUniform {
				counts.UniformBuffers++
			} else {
				counts.StorageBuffers++
			}
		case entry.Sampler != nil:
			binding.DescriptorType = vk.DescriptorTypeSampler
			counts.Samplers++
		case entry.Texture != nil:
			binding.DescriptorType = vk.DescriptorTypeSampledImage
			counts.SampledImages++
		case entry.StorageTexture != nil:
			binding.DescriptorType = vk.DescriptorTypeStorageImage
			counts.StorageImages++
		}

		bindings = append(bindings, binding)
	}

	createInfo := vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: uint32(len(bindings)),
	}

	if len(bindings) > 0 {
		createInfo.PBindings = &bindings[0]
	}

	var layout vk.DescriptorSetLayout
	result := vkCreateDescriptorSetLayout(d.cmds, d.handle, &createInfo, nil, &layout)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateDescriptorSetLayout failed: %d", result)
	}

	return &BindGroupLayout{
		handle: layout,
		counts: counts,
		device: d,
	}, nil
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(layout hal.BindGroupLayout) {
	vkLayout, ok := layout.(*BindGroupLayout)
	if !ok || vkLayout == nil {
		return
	}

	if vkLayout.handle != 0 {
		vkDestroyDescriptorSetLayout(d.cmds, d.handle, vkLayout.handle, nil)
		vkLayout.handle = 0
	}

	vkLayout.device = nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: bind group descriptor is nil")
	}

	// Get the layout
	vkLayout, ok := desc.Layout.(*BindGroupLayout)
	if !ok || vkLayout == nil {
		return nil, fmt.Errorf("vulkan: invalid bind group layout")
	}

	// Initialize descriptor allocator if needed
	if d.descriptorAllocator == nil {
		d.descriptorAllocator = NewDescriptorAllocator(d.handle, d.cmds, DefaultDescriptorAllocatorConfig())
	}

	// Allocate descriptor set
	set, pool, err := d.descriptorAllocator.Allocate(vkLayout.handle, vkLayout.counts)
	if err != nil {
		return nil, fmt.Errorf("vulkan: failed to allocate descriptor set: %w", err)
	}

	// Update descriptor set with bindings
	if err := d.updateDescriptorSet(set, desc.Entries); err != nil {
		// Free the set on error
		_ = d.descriptorAllocator.Free(pool, set)
		return nil, fmt.Errorf("vulkan: failed to update descriptor set: %w", err)
	}

	return &BindGroup{
		handle: set,
		pool:   pool,
		device: d,
	}, nil
}

// updateDescriptorSet writes resource bindings to a descriptor set.
func (d *Device) updateDescriptorSet(set vk.DescriptorSet, entries []gputypes.BindGroupEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Build write descriptor sets
	// Note: We need to keep the info structs alive until vkUpdateDescriptorSets returns
	writes := make([]vk.WriteDescriptorSet, 0, len(entries))
	bufferInfos := make([]vk.DescriptorBufferInfo, 0)
	imageInfos := make([]vk.DescriptorImageInfo, 0)

	for _, entry := range entries {
		write := vk.WriteDescriptorSet{
			SType:           vk.StructureTypeWriteDescriptorSet,
			DstSet:          set,
			DstBinding:      entry.Binding,
			DstArrayElement: 0,
			DescriptorCount: 1,
		}

		switch res := entry.Resource.(type) {
		case gputypes.BufferBinding:
			bufferInfo := vk.DescriptorBufferInfo{
				Buffer: vk.Buffer(res.Buffer),
				Offset: vk.DeviceSize(res.Offset),
				Range:  vk.DeviceSize(res.Size),
			}
			if res.Size == 0 {
				bufferInfo.Range = vk.DeviceSize(vk.WholeSize)
			}
			bufferInfos = append(bufferInfos, bufferInfo)
			write.DescriptorType = vk.DescriptorTypeUniformBuffer // Default, will be overridden by layout
			write.PBufferInfo = &bufferInfos[len(bufferInfos)-1]

		case gputypes.SamplerBinding:
			imageInfo := vk.DescriptorImageInfo{
				Sampler: vk.Sampler(res.Sampler),
			}
			imageInfos = append(imageInfos, imageInfo)
			write.DescriptorType = vk.DescriptorTypeSampler
			write.PImageInfo = &imageInfos[len(imageInfos)-1]

		case gputypes.TextureViewBinding:
			imageInfo := vk.DescriptorImageInfo{
				ImageView:   vk.ImageView(res.TextureView),
				ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
			}
			imageInfos = append(imageInfos, imageInfo)
			write.DescriptorType = vk.DescriptorTypeSampledImage
			write.PImageInfo = &imageInfos[len(imageInfos)-1]

		default:
			return fmt.Errorf("unsupported binding resource type: %T", entry.Resource)
		}

		writes = append(writes, write)
	}

	if len(writes) > 0 {
		vkUpdateDescriptorSets(d.cmds, d.handle, uint32(len(writes)), &writes[0], 0, nil)
	}

	return nil
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(group hal.BindGroup) {
	vkGroup, ok := group.(*BindGroup)
	if !ok || vkGroup == nil {
		return
	}

	if vkGroup.handle != 0 && vkGroup.pool != nil && d.descriptorAllocator != nil {
		_ = d.descriptorAllocator.Free(vkGroup.pool, vkGroup.handle)
		vkGroup.handle = 0
		vkGroup.pool = nil
	}

	vkGroup.device = nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: pipeline layout descriptor is nil")
	}

	// Convert bind group layouts to descriptor set layouts
	setLayouts := make([]vk.DescriptorSetLayout, 0, len(desc.BindGroupLayouts))
	for _, layout := range desc.BindGroupLayouts {
		vkLayout, ok := layout.(*BindGroupLayout)
		if !ok || vkLayout == nil {
			return nil, fmt.Errorf("vulkan: invalid bind group layout")
		}
		setLayouts = append(setLayouts, vkLayout.handle)
	}

	// Convert push constant ranges
	pushConstantRanges := make([]vk.PushConstantRange, 0, len(desc.PushConstantRanges))
	for _, pcr := range desc.PushConstantRanges {
		pushConstantRanges = append(pushConstantRanges, vk.PushConstantRange{
			StageFlags: shaderStagesToVk(pcr.Stages),
			Offset:     pcr.Range.Start,
			Size:       pcr.Range.End - pcr.Range.Start,
		})
	}

	createInfo := vk.PipelineLayoutCreateInfo{
		SType:          vk.StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount: uint32(len(setLayouts)),
	}

	if len(setLayouts) > 0 {
		createInfo.PSetLayouts = &setLayouts[0]
	}

	if len(pushConstantRanges) > 0 {
		createInfo.PushConstantRangeCount = uint32(len(pushConstantRanges))
		createInfo.PPushConstantRanges = &pushConstantRanges[0]
	}

	var layout vk.PipelineLayout
	result := vkCreatePipelineLayout(d.cmds, d.handle, &createInfo, nil, &layout)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreatePipelineLayout failed: %d", result)
	}

	return &PipelineLayout{
		handle: layout,
		device: d,
	}, nil
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {
	vkLayout, ok := layout.(*PipelineLayout)
	if !ok || vkLayout == nil {
		return
	}

	if vkLayout.handle != 0 {
		vkDestroyPipelineLayout(d.cmds, d.handle, vkLayout.handle, nil)
		vkLayout.handle = 0
	}

	vkLayout.device = nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: shader module descriptor is nil")
	}

	var spirv []uint32

	// Compile shader source to SPIR-V.
	// WGSL compilation via naga is required for Intel Iris Xe compatibility -
	// hardcoded SPIR-V from external tools can fail silently on Intel drivers.
	switch {
	case desc.Source.WGSL != "":
		spirvBytes, err := naga.Compile(desc.Source.WGSL)
		if err != nil {
			return nil, fmt.Errorf("vulkan: naga WGSL compilation failed: %w", err)
		}
		// Convert bytes to uint32 slice
		if len(spirvBytes)%4 != 0 {
			return nil, fmt.Errorf("vulkan: naga output size not aligned to 4 bytes")
		}
		spirv = make([]uint32, len(spirvBytes)/4)
		for i := range spirv {
			spirv[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
		}
	case len(desc.Source.SPIRV) > 0:
		spirv = desc.Source.SPIRV
	default:
		return nil, fmt.Errorf("vulkan: shader module requires WGSL or SPIR-V source")
	}

	createInfo := vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uintptr(len(spirv) * 4), // Size in bytes (uint32 = 4 bytes)
		PCode:    &spirv[0],
	}

	var module vk.ShaderModule
	result := vkCreateShaderModule(d.cmds, d.handle, &createInfo, nil, &module)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateShaderModule failed: %d", result)
	}

	return &ShaderModule{
		handle: module,
		device: d,
	}, nil
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	vkModule, ok := module.(*ShaderModule)
	if !ok || vkModule == nil {
		return
	}

	if vkModule.handle != 0 {
		vkDestroyShaderModule(d.cmds, d.handle, vkModule.handle, nil)
		vkModule.handle = 0
	}

	vkModule.device = nil
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

// WaitIdle waits for all GPU operations to complete.
func (d *Device) WaitIdle() error {
	result := d.cmds.DeviceWaitIdle(d.handle)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkDeviceWaitIdle failed: %d", result)
	}
	return nil
}

// ResetCommandPool resets all command buffers in the pool.
// Call this after ensuring all submitted command buffers have completed (e.g., after WaitIdle).
func (d *Device) ResetCommandPool() error {
	if d.commandPool == 0 {
		return nil
	}
	result := d.cmds.ResetCommandPool(d.handle, d.commandPool, 0)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkResetCommandPool failed: %d", result)
	}
	return nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	createInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: 0, // Not signaled initially
	}

	var fence vk.Fence
	result := vkCreateFence(d.cmds, d.handle, &createInfo, nil, &fence)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateFence failed: %d", result)
	}

	return &Fence{
		handle: fence,
		device: d,
	}, nil
}

// DestroyFence destroys a fence.
func (d *Device) DestroyFence(fence hal.Fence) {
	vkFence, ok := fence.(*Fence)
	if !ok || vkFence == nil {
		return
	}

	if vkFence.handle != 0 {
		vkDestroyFence(d.cmds, d.handle, vkFence.handle, nil)
		vkFence.handle = 0
	}

	vkFence.device = nil
}

// Wait waits for a fence to reach the specified value.
// Note: Standard Vulkan fences don't support timeline values, so value is ignored.
// For timeline semantics, use VK_KHR_timeline_semaphore extension.
func (d *Device) Wait(fence hal.Fence, _ uint64, timeout time.Duration) (bool, error) {
	vkFence, ok := fence.(*Fence)
	if !ok || vkFence == nil {
		return false, fmt.Errorf("vulkan: invalid fence")
	}

	// Convert timeout to nanoseconds
	timeoutNs := uint64(timeout.Nanoseconds())
	if timeout < 0 {
		timeoutNs = ^uint64(0) // UINT64_MAX for infinite wait
	}

	result := vkWaitForFences(d.cmds, d.handle, 1, &vkFence.handle, vk.Bool32(vk.True), timeoutNs)
	switch result {
	case vk.Success:
		return true, nil
	case vk.Timeout:
		return false, nil
	case vk.ErrorDeviceLost:
		return false, hal.ErrDeviceLost
	default:
		return false, fmt.Errorf("vulkan: vkWaitForFences failed: %d", result)
	}
}

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(fence hal.Fence) error {
	vkFence, ok := fence.(*Fence)
	if !ok || vkFence == nil {
		return fmt.Errorf("vulkan: invalid fence")
	}

	result := vkResetFences(d.cmds, d.handle, 1, &vkFence.handle)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkResetFences failed: %d", result)
	}
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
// Uses vkGetFenceStatus for efficient polling without blocking.
func (d *Device) GetFenceStatus(fence hal.Fence) (bool, error) {
	vkFence, ok := fence.(*Fence)
	if !ok || vkFence == nil {
		return false, fmt.Errorf("vulkan: invalid fence")
	}

	result := d.cmds.GetFenceStatus(d.handle, vkFence.handle)
	switch result {
	case vk.Success:
		return true, nil // Fence is signaled
	case vk.NotReady:
		return false, nil // Fence is not signaled yet
	case vk.ErrorDeviceLost:
		return false, hal.ErrDeviceLost
	default:
		return false, fmt.Errorf("vulkan: vkGetFenceStatus failed: %d", result)
	}
}

// Destroy releases the device.
func (d *Device) Destroy() {
	if d.commandPool != 0 {
		vkDestroyCommandPool(d.cmds, d.handle, d.commandPool, nil)
		d.commandPool = 0
	}

	if d.descriptorAllocator != nil {
		d.descriptorAllocator.Destroy()
		d.descriptorAllocator = nil
	}

	if d.renderPassCache != nil {
		d.renderPassCache.Destroy()
		d.renderPassCache = nil
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

// GetRenderPassCache returns the render pass cache, creating it if needed.
func (d *Device) GetRenderPassCache() *RenderPassCache {
	if d.renderPassCache == nil {
		d.renderPassCache = NewRenderPassCache(d.handle, d.cmds)
	}
	return d.renderPassCache
}

// Vulkan function wrappers using Commands methods

func vkCreateCommandPool(cmds *vk.Commands, device vk.Device, createInfo *vk.CommandPoolCreateInfo, allocator *vk.AllocationCallbacks, pool *vk.CommandPool) vk.Result {
	return cmds.CreateCommandPool(device, createInfo, allocator, pool)
}

func vkDestroyCommandPool(cmds *vk.Commands, device vk.Device, pool vk.CommandPool, allocator *vk.AllocationCallbacks) {
	cmds.DestroyCommandPool(device, pool, allocator)
}

func vkAllocateCommandBuffers(cmds *vk.Commands, device vk.Device, allocInfo *vk.CommandBufferAllocateInfo, cmdBuffers *vk.CommandBuffer) vk.Result {
	return cmds.AllocateCommandBuffers(device, allocInfo, cmdBuffers)
}

func vkCreateSampler(cmds *vk.Commands, device vk.Device, createInfo *vk.SamplerCreateInfo, allocator *vk.AllocationCallbacks, sampler *vk.Sampler) vk.Result {
	return cmds.CreateSampler(device, createInfo, allocator, sampler)
}

func vkDestroySampler(cmds *vk.Commands, device vk.Device, sampler vk.Sampler, allocator *vk.AllocationCallbacks) {
	cmds.DestroySampler(device, sampler, allocator)
}

func vkCreateShaderModule(cmds *vk.Commands, device vk.Device, createInfo *vk.ShaderModuleCreateInfo, allocator *vk.AllocationCallbacks, module *vk.ShaderModule) vk.Result {
	return cmds.CreateShaderModule(device, createInfo, allocator, module)
}

func vkDestroyShaderModule(cmds *vk.Commands, device vk.Device, module vk.ShaderModule, allocator *vk.AllocationCallbacks) {
	cmds.DestroyShaderModule(device, module, allocator)
}

func vkCreatePipelineLayout(cmds *vk.Commands, device vk.Device, createInfo *vk.PipelineLayoutCreateInfo, allocator *vk.AllocationCallbacks, layout *vk.PipelineLayout) vk.Result {
	return cmds.CreatePipelineLayout(device, createInfo, allocator, layout)
}

func vkDestroyPipelineLayout(cmds *vk.Commands, device vk.Device, layout vk.PipelineLayout, allocator *vk.AllocationCallbacks) {
	cmds.DestroyPipelineLayout(device, layout, allocator)
}

func vkCreateDescriptorSetLayout(cmds *vk.Commands, device vk.Device, createInfo *vk.DescriptorSetLayoutCreateInfo, allocator *vk.AllocationCallbacks, layout *vk.DescriptorSetLayout) vk.Result {
	return cmds.CreateDescriptorSetLayout(device, createInfo, allocator, layout)
}

func vkDestroyDescriptorSetLayout(cmds *vk.Commands, device vk.Device, layout vk.DescriptorSetLayout, allocator *vk.AllocationCallbacks) {
	cmds.DestroyDescriptorSetLayout(device, layout, allocator)
}

func vkCreateImageView(cmds *vk.Commands, device vk.Device, createInfo *vk.ImageViewCreateInfo, allocator *vk.AllocationCallbacks, view *vk.ImageView) vk.Result {
	return cmds.CreateImageView(device, createInfo, allocator, view)
}

func vkDestroyImageView(cmds *vk.Commands, device vk.Device, view vk.ImageView, allocator *vk.AllocationCallbacks) {
	cmds.DestroyImageView(device, view, allocator)
}

func vkCreateFence(cmds *vk.Commands, device vk.Device, createInfo *vk.FenceCreateInfo, allocator *vk.AllocationCallbacks, fence *vk.Fence) vk.Result {
	return cmds.CreateFence(device, createInfo, allocator, fence)
}

func vkDestroyFence(cmds *vk.Commands, device vk.Device, fence vk.Fence, allocator *vk.AllocationCallbacks) {
	cmds.DestroyFence(device, fence, allocator)
}

func vkWaitForFences(cmds *vk.Commands, device vk.Device, fenceCount uint32, fences *vk.Fence, waitAll vk.Bool32, timeout uint64) vk.Result {
	return cmds.WaitForFences(device, fenceCount, fences, waitAll, timeout)
}

func vkResetFences(cmds *vk.Commands, device vk.Device, fenceCount uint32, fences *vk.Fence) vk.Result {
	return cmds.ResetFences(device, fenceCount, fences)
}
