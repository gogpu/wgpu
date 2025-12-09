//go:build windows

package memory

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// AllocatorConfig configures the GPU memory allocator.
type AllocatorConfig struct {
	// BlockSize is the size of memory blocks allocated from Vulkan.
	// Default: 64MB. Must be power of 2.
	BlockSize uint64

	// MinAllocationSize is the minimum allocation granularity.
	// Default: 256 bytes. Must be power of 2.
	MinAllocationSize uint64

	// DedicatedThreshold is the size above which allocations
	// get their own VkDeviceMemory instead of suballocation.
	// Default: 32MB.
	DedicatedThreshold uint64

	// MaxBlocksPerHeap limits memory blocks per heap.
	// Default: 8 (512MB per heap with 64MB blocks).
	MaxBlocksPerHeap int
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() AllocatorConfig {
	return AllocatorConfig{
		BlockSize:          64 << 20, // 64 MB
		MinAllocationSize:  256,      // 256 bytes (Vulkan min alignment)
		DedicatedThreshold: 32 << 20, // 32 MB
		MaxBlocksPerHeap:   8,
	}
}

// MemoryPool manages allocations for a single memory type.
type MemoryPool struct {
	memoryTypeIndex uint32
	blockSize       uint64
	minAllocSize    uint64
	maxBlocks       int

	// blocks holds all allocated Vulkan memory blocks.
	blocks []*poolBlock

	// stats tracks pool statistics.
	stats PoolStats
}

// poolBlock represents a single VkDeviceMemory allocation with buddy allocator.
type poolBlock struct {
	memory vk.DeviceMemory
	size   uint64
	buddy  *BuddyAllocator
}

// PoolStats contains memory pool statistics.
type PoolStats struct {
	BlockCount      int    // Number of VkDeviceMemory allocations
	TotalSize       uint64 // Total allocated from Vulkan
	UsedSize        uint64 // Currently used by suballocations
	AllocationCount uint64 // Number of active suballocations
}

// GpuAllocator is the main GPU memory allocator.
//
// Thread-safe. Use Alloc/Free for all allocations.
type GpuAllocator struct {
	mu sync.Mutex

	device   vk.Device
	config   AllocatorConfig
	selector *MemoryTypeSelector

	// pools contains per-memory-type pools.
	// Index matches Vulkan memory type index.
	pools []*MemoryPool

	// dedicated tracks dedicated allocations.
	dedicated map[vk.DeviceMemory]*MemoryBlock

	// stats tracks global statistics.
	stats AllocatorStats
}

// AllocatorStats contains allocator-wide statistics.
type AllocatorStats struct {
	TotalAllocated       uint64 // Total memory allocated from Vulkan
	TotalUsed            uint64 // Total memory in use
	PooledAllocations    uint64 // Number of pooled allocations
	DedicatedAllocations uint64 // Number of dedicated allocations
	AllocationCount      uint64 // Total active allocations
}

var (
	// ErrNoSuitableMemoryType indicates no memory type matches requirements.
	ErrNoSuitableMemoryType = errors.New("allocator: no suitable memory type")

	// ErrAllocationFailed indicates Vulkan memory allocation failed.
	ErrAllocationFailed = errors.New("allocator: allocation failed")

	// ErrInvalidBlock indicates an invalid memory block.
	ErrInvalidBlock = errors.New("allocator: invalid memory block")
)

// NewGpuAllocator creates a new GPU memory allocator.
//
// Parameters:
//   - device: Vulkan device handle
//   - props: Device memory properties from vkGetPhysicalDeviceMemoryProperties
//   - config: Allocator configuration (use DefaultConfig() for defaults)
func NewGpuAllocator(device vk.Device, props DeviceMemoryProperties, config AllocatorConfig) (*GpuAllocator, error) {
	// Validate config
	if !isPowerOfTwo(config.BlockSize) {
		return nil, fmt.Errorf("BlockSize must be power of 2: %d", config.BlockSize)
	}
	if !isPowerOfTwo(config.MinAllocationSize) {
		return nil, fmt.Errorf("MinAllocationSize must be power of 2: %d", config.MinAllocationSize)
	}
	if config.MinAllocationSize > config.BlockSize {
		return nil, fmt.Errorf("MinAllocationSize (%d) > BlockSize (%d)", config.MinAllocationSize, config.BlockSize)
	}

	selector := NewMemoryTypeSelector(props)

	// Create pools for each memory type
	pools := make([]*MemoryPool, len(props.MemoryTypes))
	for i := range props.MemoryTypes {
		pools[i] = &MemoryPool{
			memoryTypeIndex: uint32(i),
			blockSize:       config.BlockSize,
			minAllocSize:    config.MinAllocationSize,
			maxBlocks:       config.MaxBlocksPerHeap,
			blocks:          make([]*poolBlock, 0),
		}
	}

	return &GpuAllocator{
		device:    device,
		config:    config,
		selector:  selector,
		pools:     pools,
		dedicated: make(map[vk.DeviceMemory]*MemoryBlock),
	}, nil
}

// Alloc allocates GPU memory.
//
// For large allocations (>= DedicatedThreshold), creates a dedicated
// VkDeviceMemory. For smaller allocations, suballocates from a pool
// using buddy allocation.
func (a *GpuAllocator) Alloc(req AllocationRequest) (*MemoryBlock, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Select memory type
	memTypeIndex, ok := a.selector.SelectMemoryType(req)
	if !ok {
		return nil, ErrNoSuitableMemoryType
	}

	// Ensure alignment is at least minAllocationSize
	alignment := req.Alignment
	if alignment < a.config.MinAllocationSize {
		alignment = a.config.MinAllocationSize
	}

	// Round size up to alignment
	size := req.Size
	if size%alignment != 0 {
		size = ((size / alignment) + 1) * alignment
	}

	// Choose allocation strategy
	if size >= a.config.DedicatedThreshold {
		return a.allocDedicated(size, memTypeIndex)
	}

	return a.allocPooled(size, memTypeIndex)
}

// allocDedicated creates a dedicated VkDeviceMemory allocation.
func (a *GpuAllocator) allocDedicated(size uint64, memTypeIndex uint32) (*MemoryBlock, error) {
	memory, err := a.vulkanAllocate(size, memTypeIndex)
	if err != nil {
		return nil, err
	}

	block := &MemoryBlock{
		Memory:          memory,
		Offset:          0,
		Size:            size,
		memoryTypeIndex: memTypeIndex,
		dedicated:       true,
	}

	a.dedicated[memory] = block
	a.stats.TotalAllocated += size
	a.stats.TotalUsed += size
	a.stats.DedicatedAllocations++
	a.stats.AllocationCount++

	return block, nil
}

// allocPooled suballocates from a memory pool.
func (a *GpuAllocator) allocPooled(size uint64, memTypeIndex uint32) (*MemoryBlock, error) {
	pool := a.pools[memTypeIndex]

	// Try to allocate from existing blocks
	for _, block := range pool.blocks {
		buddyBlock, err := block.buddy.Alloc(size)
		if err == nil {
			memBlock := &MemoryBlock{
				Memory:          block.memory,
				Offset:          buddyBlock.Offset,
				Size:            buddyBlock.Size,
				memoryTypeIndex: memTypeIndex,
				dedicated:       false,
				buddyBlock:      &buddyBlock,
			}

			pool.stats.UsedSize += buddyBlock.Size
			pool.stats.AllocationCount++
			a.stats.TotalUsed += buddyBlock.Size
			a.stats.PooledAllocations++
			a.stats.AllocationCount++

			return memBlock, nil
		}
	}

	// Need to allocate a new block
	if len(pool.blocks) >= pool.maxBlocks {
		// Too many blocks, try dedicated allocation
		return a.allocDedicated(size, memTypeIndex)
	}

	// Allocate new Vulkan memory block
	memory, err := a.vulkanAllocate(pool.blockSize, memTypeIndex)
	if err != nil {
		return nil, err
	}

	// Create buddy allocator for the block
	buddy, err := NewBuddyAllocator(pool.blockSize, pool.minAllocSize)
	if err != nil {
		a.vulkanFree(memory)
		return nil, err
	}

	newBlock := &poolBlock{
		memory: memory,
		size:   pool.blockSize,
		buddy:  buddy,
	}
	pool.blocks = append(pool.blocks, newBlock)
	pool.stats.BlockCount++
	pool.stats.TotalSize += pool.blockSize
	a.stats.TotalAllocated += pool.blockSize

	// Allocate from the new block
	buddyBlock, err := buddy.Alloc(size)
	if err != nil {
		return nil, err
	}

	memBlock := &MemoryBlock{
		Memory:          memory,
		Offset:          buddyBlock.Offset,
		Size:            buddyBlock.Size,
		memoryTypeIndex: memTypeIndex,
		dedicated:       false,
		buddyBlock:      &buddyBlock,
	}

	pool.stats.UsedSize += buddyBlock.Size
	pool.stats.AllocationCount++
	a.stats.TotalUsed += buddyBlock.Size
	a.stats.PooledAllocations++
	a.stats.AllocationCount++

	return memBlock, nil
}

// Free releases a memory block.
func (a *GpuAllocator) Free(block *MemoryBlock) error {
	if block == nil {
		return ErrInvalidBlock
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if block.dedicated {
		return a.freeDedicated(block)
	}

	return a.freePooled(block)
}

// freeDedicated releases a dedicated allocation.
func (a *GpuAllocator) freeDedicated(block *MemoryBlock) error {
	if _, ok := a.dedicated[block.Memory]; !ok {
		return ErrInvalidBlock
	}

	a.vulkanFree(block.Memory)
	delete(a.dedicated, block.Memory)

	a.stats.TotalAllocated -= block.Size
	a.stats.TotalUsed -= block.Size
	a.stats.DedicatedAllocations--
	a.stats.AllocationCount--

	return nil
}

// freePooled releases a pooled allocation.
func (a *GpuAllocator) freePooled(block *MemoryBlock) error {
	if block.buddyBlock == nil {
		return ErrInvalidBlock
	}

	pool := a.pools[block.memoryTypeIndex]

	// Find the pool block containing this allocation
	for _, poolBlock := range pool.blocks {
		if poolBlock.memory != block.Memory {
			continue
		}

		if err := poolBlock.buddy.Free(*block.buddyBlock); err != nil {
			return err
		}

		pool.stats.UsedSize -= block.buddyBlock.Size
		pool.stats.AllocationCount--
		a.stats.TotalUsed -= block.buddyBlock.Size
		a.stats.PooledAllocations--
		a.stats.AllocationCount--

		return nil
	}

	return ErrInvalidBlock
}

// Stats returns current allocator statistics.
func (a *GpuAllocator) Stats() AllocatorStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stats
}

// PoolStats returns statistics for a specific memory type pool.
func (a *GpuAllocator) PoolStats(memTypeIndex uint32) (PoolStats, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if int(memTypeIndex) >= len(a.pools) {
		return PoolStats{}, false
	}

	return a.pools[memTypeIndex].stats, true
}

// Destroy releases all allocations and cleans up.
//
// Call this before destroying the Vulkan device.
func (a *GpuAllocator) Destroy() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Free all dedicated allocations
	for memory := range a.dedicated {
		a.vulkanFree(memory)
	}
	a.dedicated = make(map[vk.DeviceMemory]*MemoryBlock)

	// Free all pool blocks
	for _, pool := range a.pools {
		for _, block := range pool.blocks {
			a.vulkanFree(block.memory)
		}
		pool.blocks = nil
		pool.stats = PoolStats{}
	}

	a.stats = AllocatorStats{}
}

// vulkanAllocate wraps vkAllocateMemory.
func (a *GpuAllocator) vulkanAllocate(size uint64, memTypeIndex uint32) (vk.DeviceMemory, error) {
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  vk.DeviceSize(size),
		MemoryTypeIndex: memTypeIndex,
	}

	var memory vk.DeviceMemory
	result := vk.AllocateMemory(a.device, &allocInfo, nil, &memory)
	if result != vk.Success {
		return 0, fmt.Errorf("%w: vkAllocateMemory returned %d", ErrAllocationFailed, result)
	}

	return memory, nil
}

// vulkanFree wraps vkFreeMemory.
func (a *GpuAllocator) vulkanFree(memory vk.DeviceMemory) {
	vk.FreeMemory(a.device, memory, nil)
}

// Selector returns the memory type selector.
func (a *GpuAllocator) Selector() *MemoryTypeSelector {
	return a.selector
}
