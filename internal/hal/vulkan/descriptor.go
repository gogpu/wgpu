//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
)

// DescriptorCounts tracks the number of descriptors by type.
// Used to determine pool sizes for allocation.
// Dynamic buffer counts are tracked separately because Vulkan requires
// separate pool size entries for VK_DESCRIPTOR_TYPE_*_BUFFER_DYNAMIC.
type DescriptorCounts struct {
	Samplers              uint32
	SampledImages         uint32
	StorageImages         uint32
	UniformBuffers        uint32
	StorageBuffers        uint32
	UniformBuffersDynamic uint32
	StorageBuffersDynamic uint32
	UniformTexelBuffer    uint32
	StorageTexelBuffer    uint32
	InputAttachments      uint32
	// AccelerationStructures counts VK_DESCRIPTOR_TYPE_ACCELERATION_STRUCTURE_KHR
	// descriptors. Only non-zero on devices with FeatureRayQuery enabled.
	AccelerationStructures uint32
}

// Total returns the total number of descriptors.
func (c DescriptorCounts) Total() uint32 {
	return c.Samplers + c.SampledImages + c.StorageImages +
		c.UniformBuffers + c.StorageBuffers +
		c.UniformBuffersDynamic + c.StorageBuffersDynamic +
		c.UniformTexelBuffer + c.StorageTexelBuffer + c.InputAttachments +
		c.AccelerationStructures
}

// IsEmpty returns true if no descriptors are needed.
func (c DescriptorCounts) IsEmpty() bool {
	return c.Total() == 0
}

// Multiply multiplies all counts by a factor.
func (c DescriptorCounts) Multiply(factor uint32) DescriptorCounts {
	return DescriptorCounts{
		Samplers:               c.Samplers * factor,
		SampledImages:          c.SampledImages * factor,
		StorageImages:          c.StorageImages * factor,
		UniformBuffers:         c.UniformBuffers * factor,
		StorageBuffers:         c.StorageBuffers * factor,
		UniformBuffersDynamic:  c.UniformBuffersDynamic * factor,
		StorageBuffersDynamic:  c.StorageBuffersDynamic * factor,
		UniformTexelBuffer:     c.UniformTexelBuffer * factor,
		StorageTexelBuffer:     c.StorageTexelBuffer * factor,
		InputAttachments:       c.InputAttachments * factor,
		AccelerationStructures: c.AccelerationStructures * factor,
	}
}

// DescriptorPool wraps a VkDescriptorPool with tracking.
type DescriptorPool struct {
	handle        vk.DescriptorPool
	maxSets       uint32
	allocatedSets uint32
	// optional records which of the descriptor types that not every pool
	// has (dynamic buffers, acceleration structures) this pool was created
	// with; only non-zero means present.
	optional DescriptorCounts
}

// supports reports whether the pool has pool sizes for every optional
// descriptor type counts needs. Allocating a set with a type the pool lacks
// must fail with VK_ERROR_OUT_OF_POOL_MEMORY, but with VK_KHR_maintenance1
// some drivers (NVIDIA) succeed anyway, which the validation layers flag.
func (p *DescriptorPool) supports(counts DescriptorCounts) bool {
	return (counts.UniformBuffersDynamic == 0 || p.optional.UniformBuffersDynamic > 0) &&
		(counts.StorageBuffersDynamic == 0 || p.optional.StorageBuffersDynamic > 0) &&
		(counts.AccelerationStructures == 0 || p.optional.AccelerationStructures > 0)
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
		if pool.allocatedSets >= pool.maxSets || !pool.supports(counts) {
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

	// Build pool sizes with ALL descriptor types.
	// Every pool includes all types, because Allocate serves a layout from any
	// existing pool first: bind group layouts of different kinds (uniform-only,
	// sampler+texture, dynamic buffers) share pools. Requested counts scale the
	// primary types; all other types get a baseline allocation. A pool without
	// a type its set layout uses is invalid even where the driver allocates
	// anyway (WARNING-CoreValidation-AllocateDescriptorSets-WrongType; strict
	// drivers return VK_ERROR_OUT_OF_POOL_MEMORY, P2-VK-016). Rust wgpu-hal
	// sizes pools from DescriptorCounts including the dynamic buffer types.
	poolSizes := []vk.DescriptorPoolSize{
		{Type: vk.DescriptorTypeSampler, DescriptorCount: max(counts.Samplers, 1) * poolSize},
		{Type: vk.DescriptorTypeSampledImage, DescriptorCount: max(counts.SampledImages, 1) * poolSize},
		{Type: vk.DescriptorTypeStorageImage, DescriptorCount: max(counts.StorageImages*poolSize, poolSize/4)},
		{Type: vk.DescriptorTypeUniformBuffer, DescriptorCount: max(counts.UniformBuffers, 1) * poolSize},
		{Type: vk.DescriptorTypeStorageBuffer, DescriptorCount: max(counts.StorageBuffers*poolSize, poolSize/2)},
		{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: max(counts.Samplers, 1) * poolSize},
		{Type: vk.DescriptorTypeUniformBufferDynamic, DescriptorCount: max(counts.UniformBuffersDynamic*poolSize, poolSize/4)},
		{Type: vk.DescriptorTypeStorageBufferDynamic, DescriptorCount: max(counts.StorageBuffersDynamic*poolSize, poolSize/4)},
	}
	// Acceleration structure descriptors are only valid with
	// VK_KHR_acceleration_structure enabled, so only pools created for an
	// AS-using layout include them (like the dynamic buffer types above).
	if counts.AccelerationStructures > 0 {
		poolSizes = append(poolSizes, vk.DescriptorPoolSize{
			Type:            vk.DescriptorTypeAccelerationStructureKhr,
			DescriptorCount: counts.AccelerationStructures * poolSize,
		})
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

	dp := &DescriptorPool{
		handle:        pool,
		maxSets:       poolSize,
		allocatedSets: 0,
		optional: DescriptorCounts{
			UniformBuffersDynamic:  counts.UniformBuffersDynamic,
			StorageBuffersDynamic:  counts.StorageBuffersDynamic,
			AccelerationStructures: counts.AccelerationStructures,
		},
	}
	a.setObjectName(vk.ObjectTypeDescriptorPool, uint64(pool),
		fmt.Sprintf("DescriptorPool(%d)", len(a.pools)))
	return dp, nil
}

// setObjectName labels a Vulkan object for debug/validation.
// No-op when VK_EXT_debug_utils is not available.
func (a *DescriptorAllocator) setObjectName(objectType vk.ObjectType, handle uint64, name string) {
	if !a.cmds.HasDebugUtils() || handle == 0 {
		return
	}
	nameBytes := append([]byte(name), 0)
	// PObjectName is a uintptr: pin the bytes so a stack move cannot leave it
	// pointing at the old stack (P1-VK-012).
	var pinner runtime.Pinner
	pinner.Pin(&nameBytes[0])
	nameInfo := vk.DebugUtilsObjectNameInfoEXT{
		SType:        vk.StructureTypeDebugUtilsObjectNameInfoExt,
		ObjectType:   objectType,
		ObjectHandle: handle,
		PObjectName:  uintptr(unsafe.Pointer(&nameBytes[0])),
	}
	_ = a.cmds.SetDebugUtilsObjectNameEXT(a.device, &nameInfo)
	pinner.Unpin()
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
