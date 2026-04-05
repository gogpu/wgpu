package memory

import (
	"errors"
	"math/bits"
	"sync"
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
//
// Thread-safe: all public methods are protected by a mutex.
// PERF-BUDDY-001: replaced maps with slices for O(1) push/pop free list
// operations and added mutex for concurrent safety.
type BuddyAllocator struct {
	mu sync.Mutex

	// totalSize is the total managed memory size (must be power of 2).
	totalSize uint64

	// minBlockSize is the smallest allocatable unit (must be power of 2).
	// Typical value: 256 bytes (Vulkan alignment requirement).
	minBlockSize uint64

	// maxOrder is log2(totalSize / minBlockSize).
	// Order 0 = minBlockSize, order maxOrder = totalSize.
	maxOrder int

	// freeLists contains free blocks for each order as sorted slices.
	// freeLists[i] contains offsets of free blocks of size minBlockSize << i.
	// Push/pop from end = O(1) amortized vs map iteration O(n) + hash overhead.
	freeLists [][]uint64

	// splitBlocks is a bitset tracking which blocks have been split.
	// Indexed by (order * maxBlocksPerOrder + blockIndex).
	// Replaces map[uint64]struct{} — O(1) lookup with zero allocation.
	splitBlocks []uint64 // bitset, 64 bits per element

	// allocatedBlocks tracks allocated blocks for validation.
	// Key: offset, Value: order. Kept as a map because alloc/free
	// validation needs arbitrary offset lookup (not ordered iteration).
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

	// Calculate total bits needed for the splitBlocks bitset.
	// Each order i has totalSize / (minBlockSize << i) possible blocks.
	// We index as order * maxBlocksAtOrder0 + blockIndex.
	// maxBlocksAtOrder0 = totalSize / minBlockSize.
	maxBlocksAtOrder0 := totalSize / minBlockSize
	totalBits := uint64(maxOrder+1) * maxBlocksAtOrder0
	bitsetSize := (totalBits + 63) / 64

	b := &BuddyAllocator{
		totalSize:       totalSize,
		minBlockSize:    minBlockSize,
		maxOrder:        maxOrder,
		freeLists:       make([][]uint64, maxOrder+1),
		splitBlocks:     make([]uint64, bitsetSize),
		allocatedBlocks: make(map[uint64]int, 64),
		stats: BuddyStats{
			TotalSize: totalSize,
		},
	}

	// Initially, the entire region is one free block at max order
	b.freeLists[maxOrder] = append(b.freeLists[maxOrder], 0)

	return b, nil
}

// Alloc allocates a block of at least the requested size.
//
// The returned block size will be rounded up to the next power of 2,
// and at least minBlockSize. Returns ErrOutOfMemory if no suitable
// block is available, ErrInvalidSize if size is 0 or exceeds totalSize.
//
// Thread-safe.
func (b *BuddyAllocator) Alloc(size uint64) (BuddyBlock, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

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
//
// Thread-safe.
func (b *BuddyAllocator) Free(block BuddyBlock) error {
	b.mu.Lock()
	defer b.mu.Unlock()

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
// Thread-safe.
func (b *BuddyAllocator) Stats() BuddyStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stats
}

// Reset releases all allocations and resets the allocator to initial state.
// Thread-safe.
func (b *BuddyAllocator) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clear all free lists
	for i := range b.freeLists {
		b.freeLists[i] = b.freeLists[i][:0]
	}

	// Clear splitBlocks bitset
	for i := range b.splitBlocks {
		b.splitBlocks[i] = 0
	}

	// Clear allocated blocks map
	b.allocatedBlocks = make(map[uint64]int, 64)

	// Reset to single max-order block
	b.freeLists[b.maxOrder] = append(b.freeLists[b.maxOrder], 0)

	// Reset stats (keep totals for historical tracking)
	b.stats.AllocatedSize = 0
	b.stats.AllocationCount = 0
}

// splitBitIndex returns the bitset index for a split block at the given order and offset.
// The index is: order * maxBlocksAtOrder0 + (offset / minBlockSize).
func (b *BuddyAllocator) splitBitIndex(order int, offset uint64) uint64 {
	maxBlocksAtOrder0 := b.totalSize / b.minBlockSize
	blockIndex := offset / b.minBlockSize
	return uint64(order)*maxBlocksAtOrder0 + blockIndex
}

// setSplit marks a block as split in the bitset.
func (b *BuddyAllocator) setSplit(order int, offset uint64) {
	idx := b.splitBitIndex(order, offset)
	b.splitBlocks[idx/64] |= 1 << (idx % 64)
}

// clearSplit unmarks a block as split in the bitset.
func (b *BuddyAllocator) clearSplit(order int, offset uint64) {
	idx := b.splitBitIndex(order, offset)
	b.splitBlocks[idx/64] &^= 1 << (idx % 64)
}

// findAndSplit finds a free block of the target order, splitting larger blocks if needed.
func (b *BuddyAllocator) findAndSplit(targetOrder int) (uint64, bool) {
	// First, try to find a free block at the exact order (pop from end = O(1))
	if n := len(b.freeLists[targetOrder]); n > 0 {
		offset := b.freeLists[targetOrder][n-1]
		b.freeLists[targetOrder] = b.freeLists[targetOrder][:n-1]
		return offset, true
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

	// Pop the block to split (from end = O(1))
	n := len(b.freeLists[splitOrder])
	offset := b.freeLists[splitOrder][n-1]
	b.freeLists[splitOrder] = b.freeLists[splitOrder][:n-1]

	// Split down to target order
	for order := splitOrder; order > targetOrder; order-- {
		blockSize := b.minBlockSize << order
		halfSize := blockSize >> 1

		// Mark this block as split
		b.setSplit(order, offset)
		b.stats.SplitCount++

		// The right buddy goes to free list
		buddyOffset := offset + halfSize
		b.freeLists[order-1] = append(b.freeLists[order-1], buddyOffset)

		// Continue with the left half
	}

	return offset, true
}

// freeAndMerge adds a block to free list and merges with buddy if both are free.
func (b *BuddyAllocator) freeAndMerge(offset uint64, order int) {
	for order <= b.maxOrder {
		blockSize := b.minBlockSize << order

		// Calculate buddy offset
		var buddyOffset uint64
		if (offset & blockSize) == 0 {
			buddyOffset = offset + blockSize
		} else {
			buddyOffset = offset - blockSize
		}

		// At max order there's no buddy to merge with
		if order == b.maxOrder {
			b.freeLists[order] = append(b.freeLists[order], offset)
			return
		}

		// Check if buddy is free by scanning the free list.
		// Free lists are typically small (< 100 entries), so linear scan
		// is faster than map lookup due to cache locality and no hash overhead.
		buddyIdx := -1
		freeList := b.freeLists[order]
		for i, off := range freeList {
			if off == buddyOffset {
				buddyIdx = i
				break
			}
		}

		if buddyIdx == -1 {
			// Buddy is not free, just add this block to free list
			b.freeLists[order] = append(b.freeLists[order], offset)
			return
		}

		// Buddy is free! Remove it and merge.
		// Swap-remove: O(1) removal from unordered slice.
		lastIdx := len(freeList) - 1
		freeList[buddyIdx] = freeList[lastIdx]
		b.freeLists[order] = freeList[:lastIdx]
		b.stats.MergeCount++

		// Remove split marker from parent
		parentOffset := offset & ^blockSize // Align to 2*blockSize
		parentOrder := order + 1
		b.clearSplit(parentOrder, parentOffset)

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
