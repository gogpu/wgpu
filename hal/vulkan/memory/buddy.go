package memory

import (
	"errors"
	"math/bits"
)

// BuddyAllocator implements the buddy memory allocation algorithm.
//
// The allocator manages a contiguous region of memory by dividing it
// into power-of-2 sized blocks. When allocating, blocks are split
// recursively until the smallest fitting size is found. When freeing,
// adjacent "buddy" blocks are merged back together.
//
// Time complexity: O(log n) for both allocation and deallocation.
// Space overhead: O(n) bits for tracking block states.
type BuddyAllocator struct {
	// totalSize is the total managed memory size (must be power of 2).
	totalSize uint64

	// minBlockSize is the smallest allocatable unit (must be power of 2).
	// Typical value: 256 bytes (Vulkan alignment requirement).
	minBlockSize uint64

	// maxOrder is log2(totalSize / minBlockSize).
	// Order 0 = minBlockSize, order maxOrder = totalSize.
	maxOrder int

	// freeLists contains free blocks for each order.
	// freeLists[i] contains blocks of size minBlockSize << i.
	freeLists []map[uint64]struct{}

	// splitBlocks tracks which blocks have been split.
	// Key: (order << 48) | offset
	splitBlocks map[uint64]struct{}

	// allocatedBlocks tracks allocated blocks for validation.
	// Key: offset, Value: order
	allocatedBlocks map[uint64]int

	// stats tracks allocation statistics.
	stats BuddyStats
}

// BuddyStats contains allocator statistics.
type BuddyStats struct {
	TotalSize       uint64 // Total managed memory
	AllocatedSize   uint64 // Currently allocated
	AllocationCount uint64 // Number of active allocations
	PeakAllocated   uint64 // Peak allocated size
	TotalAllocated  uint64 // Cumulative allocated (for throughput)
	TotalFreed      uint64 // Cumulative freed
	SplitCount      uint64 // Number of block splits
	MergeCount      uint64 // Number of block merges
}

// BuddyBlock represents an allocated memory block.
type BuddyBlock struct {
	Offset uint64 // Offset within the managed region
	Size   uint64 // Actual size (power of 2, >= requested)
	order  int    // Internal: block order for deallocation
}

var (
	// ErrOutOfMemory indicates no suitable block is available.
	ErrOutOfMemory = errors.New("buddy: out of memory")

	// ErrInvalidSize indicates the requested size is invalid.
	ErrInvalidSize = errors.New("buddy: invalid size (zero or too large)")

	// ErrDoubleFree indicates an attempt to free an unallocated block.
	ErrDoubleFree = errors.New("buddy: double free or invalid block")

	// ErrInvalidConfig indicates invalid allocator configuration.
	ErrInvalidConfig = errors.New("buddy: invalid configuration")
)

// NewBuddyAllocator creates a new buddy allocator.
//
// Parameters:
//   - totalSize: Total memory to manage (must be power of 2)
//   - minBlockSize: Smallest allocatable unit (must be power of 2, <= totalSize)
//
// Returns error if parameters are invalid.
func NewBuddyAllocator(totalSize, minBlockSize uint64) (*BuddyAllocator, error) {
	// Validate parameters
	if totalSize == 0 || !isPowerOfTwo(totalSize) {
		return nil, ErrInvalidConfig
	}
	if minBlockSize == 0 || !isPowerOfTwo(minBlockSize) {
		return nil, ErrInvalidConfig
	}
	if minBlockSize > totalSize {
		return nil, ErrInvalidConfig
	}

	maxOrder := log2(totalSize / minBlockSize)

	b := &BuddyAllocator{
		totalSize:       totalSize,
		minBlockSize:    minBlockSize,
		maxOrder:        maxOrder,
		freeLists:       make([]map[uint64]struct{}, maxOrder+1),
		splitBlocks:     make(map[uint64]struct{}),
		allocatedBlocks: make(map[uint64]int),
		stats: BuddyStats{
			TotalSize: totalSize,
		},
	}

	// Initialize free lists
	for i := range b.freeLists {
		b.freeLists[i] = make(map[uint64]struct{})
	}

	// Initially, the entire region is one free block at max order
	b.freeLists[maxOrder][0] = struct{}{}

	return b, nil
}

// Alloc allocates a block of at least the requested size.
//
// The returned block size will be rounded up to the next power of 2,
// and at least minBlockSize. Returns ErrOutOfMemory if no suitable
// block is available, ErrInvalidSize if size is 0 or exceeds totalSize.
func (b *BuddyAllocator) Alloc(size uint64) (BuddyBlock, error) {
	if size == 0 || size > b.totalSize {
		return BuddyBlock{}, ErrInvalidSize
	}

	// Round up to power of 2 and at least minBlockSize
	allocSize := nextPowerOfTwo(size)
	if allocSize < b.minBlockSize {
		allocSize = b.minBlockSize
	}

	targetOrder := log2(allocSize / b.minBlockSize)
	if targetOrder > b.maxOrder {
		return BuddyBlock{}, ErrInvalidSize
	}

	// Find a free block, splitting larger blocks if necessary
	offset, ok := b.findAndSplit(targetOrder)
	if !ok {
		return BuddyBlock{}, ErrOutOfMemory
	}

	// Track allocation
	b.allocatedBlocks[offset] = targetOrder
	b.stats.AllocatedSize += allocSize
	b.stats.AllocationCount++
	b.stats.TotalAllocated += allocSize
	if b.stats.AllocatedSize > b.stats.PeakAllocated {
		b.stats.PeakAllocated = b.stats.AllocatedSize
	}

	return BuddyBlock{
		Offset: offset,
		Size:   allocSize,
		order:  targetOrder,
	}, nil
}

// Free releases a previously allocated block.
//
// Returns ErrDoubleFree if the block was not allocated or already freed.
func (b *BuddyAllocator) Free(block BuddyBlock) error {
	// Validate the block was allocated
	order, ok := b.allocatedBlocks[block.Offset]
	if !ok {
		return ErrDoubleFree
	}
	if order != block.order {
		return ErrDoubleFree
	}

	delete(b.allocatedBlocks, block.Offset)

	blockSize := b.minBlockSize << order
	b.stats.AllocatedSize -= blockSize
	b.stats.AllocationCount--
	b.stats.TotalFreed += blockSize

	// Add to free list and merge with buddy if possible
	b.freeAndMerge(block.Offset, order)

	return nil
}

// Stats returns current allocator statistics.
func (b *BuddyAllocator) Stats() BuddyStats {
	return b.stats
}

// Reset releases all allocations and resets the allocator to initial state.
func (b *BuddyAllocator) Reset() {
	// Clear all free lists
	for i := range b.freeLists {
		b.freeLists[i] = make(map[uint64]struct{})
	}

	// Clear tracking maps
	b.splitBlocks = make(map[uint64]struct{})
	b.allocatedBlocks = make(map[uint64]int)

	// Reset to single max-order block
	b.freeLists[b.maxOrder][0] = struct{}{}

	// Reset stats (keep totals for historical tracking)
	b.stats.AllocatedSize = 0
	b.stats.AllocationCount = 0
}

// findAndSplit finds a free block of the target order, splitting larger blocks if needed.
func (b *BuddyAllocator) findAndSplit(targetOrder int) (uint64, bool) {
	// First, try to find a free block at the exact order
	if len(b.freeLists[targetOrder]) > 0 {
		// Get any free block (map iteration is random, that's fine)
		for offset := range b.freeLists[targetOrder] {
			delete(b.freeLists[targetOrder], offset)
			return offset, true
		}
	}

	// No free block at target order, find a larger block to split
	splitOrder := -1
	for order := targetOrder + 1; order <= b.maxOrder; order++ {
		if len(b.freeLists[order]) > 0 {
			splitOrder = order
			break
		}
	}

	if splitOrder == -1 {
		return 0, false // No suitable block found
	}

	// Get the block to split
	var offset uint64
	for o := range b.freeLists[splitOrder] {
		offset = o
		delete(b.freeLists[splitOrder], o)
		break
	}

	// Split down to target order
	for order := splitOrder; order > targetOrder; order-- {
		blockSize := b.minBlockSize << order
		halfSize := blockSize >> 1

		// Mark this block as split
		splitKey := (uint64(order) << 48) | offset
		b.splitBlocks[splitKey] = struct{}{}
		b.stats.SplitCount++

		// The right buddy goes to free list
		buddyOffset := offset + halfSize
		b.freeLists[order-1][buddyOffset] = struct{}{}

		// Continue with the left half
	}

	return offset, true
}

// freeAndMerge adds a block to free list and merges with buddy if both are free.
func (b *BuddyAllocator) freeAndMerge(offset uint64, order int) {
	for order <= b.maxOrder {
		blockSize := b.minBlockSize << order

		// Calculate buddy offset
		// If offset is aligned to 2*blockSize, buddy is to the right
		// Otherwise, buddy is to the left
		var buddyOffset uint64
		if (offset & blockSize) == 0 {
			buddyOffset = offset + blockSize
		} else {
			buddyOffset = offset - blockSize
		}

		// Check if buddy is free (and not at max order where there's no buddy)
		if order == b.maxOrder {
			b.freeLists[order][offset] = struct{}{}
			return
		}

		_, buddyFree := b.freeLists[order][buddyOffset]
		if !buddyFree {
			// Buddy is not free, just add this block to free list
			b.freeLists[order][offset] = struct{}{}
			return
		}

		// Buddy is free! Merge them.
		delete(b.freeLists[order], buddyOffset)
		b.stats.MergeCount++

		// Remove split marker from parent
		parentOffset := offset & ^blockSize // Align to 2*blockSize
		parentOrder := order + 1
		splitKey := (uint64(parentOrder) << 48) | parentOffset
		delete(b.splitBlocks, splitKey)

		// Continue merging at the next level
		offset = parentOffset
		order = parentOrder
	}
}

// Helper functions

// isPowerOfTwo checks if n is a power of 2.
func isPowerOfTwo(n uint64) bool {
	return n > 0 && (n&(n-1)) == 0
}

// nextPowerOfTwo returns the smallest power of 2 >= n.
func nextPowerOfTwo(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	if isPowerOfTwo(n) {
		return n
	}
	return 1 << (64 - bits.LeadingZeros64(n))
}

// log2 returns floor(log2(n)) for n > 0.
func log2(n uint64) int {
	if n == 0 {
		return 0
	}
	return 63 - bits.LeadingZeros64(n)
}
