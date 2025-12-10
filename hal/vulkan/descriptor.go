// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// DescriptorCounts tracks the number of descriptors by type.
// Used to determine pool sizes for allocation.
type DescriptorCounts struct {
	Samplers           uint32
	SampledImages      uint32
	StorageImages      uint32
	UniformBuffers     uint32
	StorageBuffers     uint32
	UniformTexelBuffer uint32
	StorageTexelBuffer uint32
	InputAttachments   uint32
}

// Total returns the total number of descriptors.
func (c DescriptorCounts) Total() uint32 {
	return c.Samplers + c.SampledImages + c.StorageImages +
		c.UniformBuffers + c.StorageBuffers +
		c.UniformTexelBuffer + c.StorageTexelBuffer + c.InputAttachments
}

// IsEmpty returns true if no descriptors are needed.
func (c DescriptorCounts) IsEmpty() bool {
	return c.Total() == 0
}

// Multiply multiplies all counts by a factor.
func (c DescriptorCounts) Multiply(factor uint32) DescriptorCounts {
	return DescriptorCounts{
		Samplers:           c.Samplers * factor,
		SampledImages:      c.SampledImages * factor,
		StorageImages:      c.StorageImages * factor,
		UniformBuffers:     c.UniformBuffers * factor,
		StorageBuffers:     c.StorageBuffers * factor,
		UniformTexelBuffer: c.UniformTexelBuffer * factor,
		StorageTexelBuffer: c.StorageTexelBuffer * factor,
		InputAttachments:   c.InputAttachments * factor,
	}
}

// DescriptorPool wraps a VkDescriptorPool with tracking.
type DescriptorPool struct {
	handle        vk.DescriptorPool
	maxSets       uint32
	allocatedSets uint32
}

// DescriptorAllocator manages descriptor pool allocation.
//
// Thread-safe. Uses on-demand pool growth strategy with FREE_DESCRIPTOR_SET flag
// for individual descriptor set freeing. Follows wgpu patterns adapted for Go.
type DescriptorAllocator struct {
	mu     sync.Mutex
	device vk.Device
	cmds   *vk.Commands
	pools  []*DescriptorPool

	// Configuration
	initialPoolSize uint32 // Initial sets per pool (default: 64)
	maxPoolSize     uint32 // Maximum sets per pool (default: 4096)
	growthFactor    uint32 // Growth factor for new pools (default: 2)

	// Statistics
	totalAllocated uint32
	totalFreed     uint32
}

// DescriptorAllocatorConfig configures the descriptor allocator.
type DescriptorAllocatorConfig struct {
	// InitialPoolSize is the number of sets in the first pool.
	// Default: 64
	InitialPoolSize uint32

	// MaxPoolSize is the maximum sets per pool.
	// Default: 4096
	MaxPoolSize uint32

	// GrowthFactor is the multiplier for each new pool size.
	// Default: 2
	GrowthFactor uint32
}

// DefaultDescriptorAllocatorConfig returns sensible defaults.
func DefaultDescriptorAllocatorConfig() DescriptorAllocatorConfig {
	return DescriptorAllocatorConfig{
		InitialPoolSize: 64,
		MaxPoolSize:     4096,
		GrowthFactor:    2,
	}
}

// NewDescriptorAllocator creates a new descriptor allocator.
func NewDescriptorAllocator(device vk.Device, cmds *vk.Commands, config DescriptorAllocatorConfig) *DescriptorAllocator {
	if config.InitialPoolSize == 0 {
		config.InitialPoolSize = 64
	}
	if config.MaxPoolSize == 0 {
		config.MaxPoolSize = 4096
	}
	if config.GrowthFactor == 0 {
		config.GrowthFactor = 2
	}

	return &DescriptorAllocator{
		device:          device,
		cmds:            cmds,
		pools:           make([]*DescriptorPool, 0),
		initialPoolSize: config.InitialPoolSize,
		maxPoolSize:     config.MaxPoolSize,
		growthFactor:    config.GrowthFactor,
	}
}

// Allocate allocates a descriptor set from the given layout.
func (a *DescriptorAllocator) Allocate(layout vk.DescriptorSetLayout, counts DescriptorCounts) (vk.DescriptorSet, *DescriptorPool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Try to allocate from existing pools
	for _, pool := range a.pools {
		if pool.allocatedSets >= pool.maxSets {
			continue
		}

		set, err := a.allocateFromPool(pool, layout)
		if err == nil {
			pool.allocatedSets++
			a.totalAllocated++
			return set, pool, nil
		}

		// VK_ERROR_OUT_OF_POOL_MEMORY or VK_ERROR_FRAGMENTED_POOL
		// Try next pool
	}

	// No space in existing pools, create a new one
	newPool, err := a.createPool(counts)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create descriptor pool: %w", err)
	}

	a.pools = append(a.pools, newPool)

	// Allocate from new pool
	set, err := a.allocateFromPool(newPool, layout)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to allocate from new pool: %w", err)
	}

	newPool.allocatedSets++
	a.totalAllocated++
	return set, newPool, nil
}

// Free frees a descriptor set back to its pool.
func (a *DescriptorAllocator) Free(pool *DescriptorPool, set vk.DescriptorSet) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := vkFreeDescriptorSets(a.cmds, a.device, pool.handle, 1, &set)
	if result != vk.Success {
		return fmt.Errorf("vkFreeDescriptorSets failed: %d", result)
	}

	pool.allocatedSets--
	a.totalFreed++
	return nil
}

// allocateFromPool allocates a descriptor set from a specific pool.
func (a *DescriptorAllocator) allocateFromPool(pool *DescriptorPool, layout vk.DescriptorSetLayout) (vk.DescriptorSet, error) {
	allocInfo := vk.DescriptorSetAllocateInfo{
		SType:              vk.StructureTypeDescriptorSetAllocateInfo,
		DescriptorPool:     pool.handle,
		DescriptorSetCount: 1,
		PSetLayouts:        &layout,
	}

	var set vk.DescriptorSet
	result := vkAllocateDescriptorSets(a.cmds, a.device, &allocInfo, &set)
	if result != vk.Success {
		return 0, fmt.Errorf("vkAllocateDescriptorSets failed: %d", result)
	}

	return set, nil
}

// createPool creates a new descriptor pool.
func (a *DescriptorAllocator) createPool(counts DescriptorCounts) (*DescriptorPool, error) {
	// Calculate pool size based on growth
	poolSize := a.initialPoolSize
	for i := 0; i < len(a.pools); i++ {
		poolSize *= a.growthFactor
		if poolSize > a.maxPoolSize {
			poolSize = a.maxPoolSize
			break
		}
	}

	// Build pool sizes for each descriptor type
	var poolSizes []vk.DescriptorPoolSize

	// Use reasonable defaults if counts are empty (common case for simple pipelines)
	//nolint:nestif // Pool sizing requires checking each descriptor type.
	if counts.IsEmpty() {
		// Default pool sizes for general use
		poolSizes = []vk.DescriptorPoolSize{
			{Type: vk.DescriptorTypeSampler, DescriptorCount: poolSize},
			{Type: vk.DescriptorTypeSampledImage, DescriptorCount: poolSize},
			{Type: vk.DescriptorTypeStorageImage, DescriptorCount: poolSize / 4},
			{Type: vk.DescriptorTypeUniformBuffer, DescriptorCount: poolSize},
			{Type: vk.DescriptorTypeStorageBuffer, DescriptorCount: poolSize / 2},
			{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: poolSize},
		}
	} else {
		// Use exact counts multiplied by pool size
		if counts.Samplers > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeSampler,
				DescriptorCount: counts.Samplers * poolSize,
			})
		}
		if counts.SampledImages > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeSampledImage,
				DescriptorCount: counts.SampledImages * poolSize,
			})
		}
		if counts.StorageImages > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeStorageImage,
				DescriptorCount: counts.StorageImages * poolSize,
			})
		}
		if counts.UniformBuffers > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeUniformBuffer,
				DescriptorCount: counts.UniformBuffers * poolSize,
			})
		}
		if counts.StorageBuffers > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeStorageBuffer,
				DescriptorCount: counts.StorageBuffers * poolSize,
			})
		}
		if counts.UniformTexelBuffer > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeUniformTexelBuffer,
				DescriptorCount: counts.UniformTexelBuffer * poolSize,
			})
		}
		if counts.StorageTexelBuffer > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeStorageTexelBuffer,
				DescriptorCount: counts.StorageTexelBuffer * poolSize,
			})
		}
		if counts.InputAttachments > 0 {
			poolSizes = append(poolSizes, vk.DescriptorPoolSize{
				Type:            vk.DescriptorTypeInputAttachment,
				DescriptorCount: counts.InputAttachments * poolSize,
			})
		}
	}

	// Ensure we have at least one pool size
	if len(poolSizes) == 0 {
		poolSizes = []vk.DescriptorPoolSize{
			{Type: vk.DescriptorTypeUniformBuffer, DescriptorCount: poolSize},
		}
	}

	createInfo := vk.DescriptorPoolCreateInfo{
		SType: vk.StructureTypeDescriptorPoolCreateInfo,
		// VK_DESCRIPTOR_POOL_CREATE_FREE_DESCRIPTOR_SET_BIT allows individual set freeing
		Flags:         vk.DescriptorPoolCreateFlags(vk.DescriptorPoolCreateFreeDescriptorSetBit),
		MaxSets:       poolSize,
		PoolSizeCount: uint32(len(poolSizes)),
		PPoolSizes:    &poolSizes[0],
	}

	var pool vk.DescriptorPool
	result := vkCreateDescriptorPool(a.cmds, a.device, &createInfo, nil, &pool)
	if result != vk.Success {
		return nil, fmt.Errorf("vkCreateDescriptorPool failed: %d", result)
	}

	return &DescriptorPool{
		handle:        pool,
		maxSets:       poolSize,
		allocatedSets: 0,
	}, nil
}

// Destroy releases all descriptor pools.
func (a *DescriptorAllocator) Destroy() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, pool := range a.pools {
		vkDestroyDescriptorPool(a.cmds, a.device, pool.handle, nil)
	}
	a.pools = nil
}

// Stats returns allocator statistics.
func (a *DescriptorAllocator) Stats() (pools int, allocated, freed uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pools), a.totalAllocated, a.totalFreed
}

// Vulkan function wrappers

func vkCreateDescriptorPool(cmds *vk.Commands, device vk.Device, createInfo *vk.DescriptorPoolCreateInfo, _ unsafe.Pointer, pool *vk.DescriptorPool) vk.Result {
	return cmds.CreateDescriptorPool(device, createInfo, nil, pool)
}

func vkDestroyDescriptorPool(cmds *vk.Commands, device vk.Device, pool vk.DescriptorPool, _ unsafe.Pointer) {
	cmds.DestroyDescriptorPool(device, pool, nil)
}

func vkAllocateDescriptorSets(cmds *vk.Commands, device vk.Device, allocInfo *vk.DescriptorSetAllocateInfo, sets *vk.DescriptorSet) vk.Result {
	return cmds.AllocateDescriptorSets(device, allocInfo, sets)
}

func vkFreeDescriptorSets(cmds *vk.Commands, device vk.Device, pool vk.DescriptorPool, count uint32, sets *vk.DescriptorSet) vk.Result {
	return cmds.FreeDescriptorSets(device, pool, count, sets)
}

func vkUpdateDescriptorSets(cmds *vk.Commands, device vk.Device, writeCount uint32, writes *vk.WriteDescriptorSet, copyCount uint32, copies *vk.CopyDescriptorSet) {
	cmds.UpdateDescriptorSets(device, writeCount, writes, copyCount, copies)
}
