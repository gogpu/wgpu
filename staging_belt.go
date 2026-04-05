package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// stagingBelt manages reusable staging buffer chunks for zero-allocation
// GPU copy operations. Instead of creating a new staging buffer per
// WriteBuffer/WriteTexture call (heap allocations every frame), it
// sub-allocates from pre-allocated chunks via bump pointer.
//
// Architecture matches Rust wgpu util::StagingBelt (belt.rs):
//   - Chunks are large buffers (default 256KB) with MapWrite|CopySrc usage
//   - allocate() bump-allocates within the current chunk
//   - If a chunk is full, a free chunk is recycled or a new one created
//   - finish() moves active chunks to closed (in-flight on GPU)
//   - recall() recycles completed chunks back to free pool
//
// Go simplification: HAL buffers with MappedAtCreation are persistently
// host-visible — no async map/unmap dance needed (unlike Rust wgpu where
// buffer.map_async + mpsc channel is required for recycling).
//
// Thread safety: the belt is used exclusively under pendingWrites.mu.
// No internal synchronization needed.
//
// Steady-state allocation cost: 0 heap allocs per writeBuffer call.
// New chunks are created only during warmup or traffic spikes.
type stagingBelt struct {
	halDevice hal.Device
	halQueue  hal.Queue
	chunkSize uint64 // size of each pre-allocated chunk
	alignment uint64 // minimum sub-allocation alignment (power of two)

	// activeChunks are being sub-allocated via bump pointer.
	// Typically 1 chunk is active; more are activated when the current
	// chunk's remaining space can't fit an allocation.
	activeChunks []stagingChunk

	// freeChunks are recycled (GPU done, offset reset to 0).
	freeChunks []stagingChunk

	// closedSubmissions track chunks that are in-flight on the GPU,
	// keyed by submission index. recall() recycles them after completion.
	closedSubmissions []closedSubmission

	// oversized tracks one-off buffers for writes larger than chunkSize.
	// These are not recycled — destroyed after GPU completion (too large
	// to keep in the pool without wasting memory).
	oversized []hal.Buffer
}

// stagingChunk is a large staging buffer with a bump-pointer allocator.
type stagingChunk struct {
	buffer hal.Buffer
	size   uint64
	offset uint64 // next allocation starts here (bump pointer)
}

// closedSubmission groups chunks that were active during a submission.
// After the GPU completes this submission, chunks are recycled to freeChunks.
type closedSubmission struct {
	submissionIndex uint64
	chunks          []stagingChunk
	oversized       []hal.Buffer
}

// stagingBeltDefaultChunkSize is 256KB — large enough for typical per-frame
// uploads (uniform/vertex/index buffers are 64B-16KB each, ~20 per frame)
// but small enough to not waste memory when idle.
const stagingBeltDefaultChunkSize = 256 * 1024

// stagingBeltDefaultAlignment is the default minimum alignment for sub-allocations.
// WebGPU COPY_BUFFER_ALIGNMENT = 4. Rust wgpu uses MAP_ALIGNMENT = 8.
// We default to 8 (Rust parity) — good balance between spec compliance and
// cache performance. Configurable via newStagingBelt alignment parameter.
const stagingBeltDefaultAlignment uint64 = 8

// newStagingBelt creates a staging belt with the given chunk size and alignment.
// If chunkSize is 0, uses the default (256KB).
// If alignment is 0, uses the default (8 bytes, Rust wgpu parity).
// Alignment must be a power of two.
// Matches Rust wgpu StagingBelt::allocate() where alignment is per-allocation;
// we simplify to per-belt since the belt is internal to pendingWrites.
func newStagingBelt(halDevice hal.Device, halQueue hal.Queue, chunkSize, alignment uint64) *stagingBelt { //nolint:unparam // alignment is configurable for future callers and testing
	if chunkSize == 0 {
		chunkSize = stagingBeltDefaultChunkSize
	}
	if alignment == 0 {
		alignment = stagingBeltDefaultAlignment
	}
	return &stagingBelt{
		halDevice: halDevice,
		halQueue:  halQueue,
		chunkSize: chunkSize,
		alignment: alignment,
	}
}

// stagingAllocation is the result of a belt allocation: a buffer region
// where data can be written via HAL WriteBuffer, then copied to the
// destination via CopyBufferToBuffer.
type stagingAllocation struct {
	buffer hal.Buffer // the staging chunk's buffer
	offset uint64     // offset within the buffer
}

// allocate reserves `size` bytes from the belt and writes `data` into
// the allocated region. Returns the buffer and offset for use in
// CopyBufferToBuffer commands.
//
// Algorithm (matches Rust wgpu StagingBelt::allocate):
//  1. Try to fit in an existing active chunk (scan for space)
//  2. If no active chunk has space, try a free chunk (recycled)
//  3. If no free chunk available, create a new chunk
//  4. For oversized writes (> chunkSize), create a one-off buffer
//
// Zero allocations in steady state — all chunks are pre-allocated.
func (b *stagingBelt) allocate(size uint64, data []byte) (stagingAllocation, error) {
	if size == 0 {
		return stagingAllocation{}, nil
	}

	alignedSize := alignUp64(size, b.alignment)

	// Oversized write: create a one-off staging buffer (not recycled).
	// This avoids wasting chunk pool memory on rare large transfers.
	if alignedSize > b.chunkSize {
		return b.allocateOversized(size, data)
	}

	// Try active chunks first (most common path — fits in current chunk).
	for i := range b.activeChunks {
		alloc, ok := b.activeChunks[i].tryAllocate(alignedSize)
		if ok {
			if err := b.halQueue.WriteBuffer(alloc.buffer, alloc.offset, data); err != nil {
				b.activeChunks[i].rollback(alignedSize)
				return stagingAllocation{}, fmt.Errorf("staging belt: write to active chunk: %w", err)
			}
			return alloc, nil
		}
	}

	// No active chunk has space — try recycling a free chunk.
	if len(b.freeChunks) > 0 {
		chunk := b.freeChunks[len(b.freeChunks)-1]
		b.freeChunks = b.freeChunks[:len(b.freeChunks)-1]
		alloc, _ := chunk.tryAllocate(alignedSize)
		b.activeChunks = append(b.activeChunks, chunk)
		if err := b.halQueue.WriteBuffer(alloc.buffer, alloc.offset, data); err != nil {
			b.activeChunks[len(b.activeChunks)-1].rollback(alignedSize)
			return stagingAllocation{}, fmt.Errorf("staging belt: write to recycled chunk: %w", err)
		}
		return alloc, nil
	}

	// No free chunks — create a new one.
	chunk, err := b.createChunk(b.chunkSize)
	if err != nil {
		return stagingAllocation{}, err
	}
	alloc, _ := chunk.tryAllocate(alignedSize)
	b.activeChunks = append(b.activeChunks, chunk)
	if err := b.halQueue.WriteBuffer(alloc.buffer, alloc.offset, data); err != nil {
		chunk.rollback(alignedSize)
		return stagingAllocation{}, fmt.Errorf("staging belt: write to new chunk: %w", err)
	}
	return alloc, nil
}

// allocateOversized creates a one-off staging buffer for writes larger
// than chunkSize. The buffer is tracked separately and destroyed after
// GPU completion (not recycled into the chunk pool).
func (b *stagingBelt) allocateOversized(size uint64, data []byte) (stagingAllocation, error) {
	desc := hal.BufferDescriptor{
		Label:            "(wgpu internal) staging oversized",
		Size:             size,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	}
	buf, err := b.halDevice.CreateBuffer(&desc)
	if err != nil {
		return stagingAllocation{}, fmt.Errorf("staging belt: create oversized buffer: %w", err)
	}
	if err := b.halQueue.WriteBuffer(buf, 0, data); err != nil {
		b.halDevice.DestroyBuffer(buf)
		return stagingAllocation{}, fmt.Errorf("staging belt: write oversized buffer: %w", err)
	}
	b.oversized = append(b.oversized, buf)
	return stagingAllocation{buffer: buf, offset: 0}, nil
}

// finish moves all active chunks and oversized buffers to closed state.
// Called at flush() time when the command encoder is about to be submitted.
// The submission index is not yet known — call setLastSubmissionIndex()
// after HAL Submit returns with the actual index.
//
// The belt is the sole owner of all staging resources (chunks + oversized).
// recall() handles destruction of oversized buffers after GPU completion.
// No buffers are returned to the caller — this avoids double-destroy bugs.
func (b *stagingBelt) finish(submissionIndex uint64) {
	if len(b.activeChunks) == 0 && len(b.oversized) == 0 {
		return
	}

	b.closedSubmissions = append(b.closedSubmissions, closedSubmission{
		submissionIndex: submissionIndex,
		chunks:          b.activeChunks,
		oversized:       b.oversized,
	})

	// Reset active lists. Use nil (not [:0]) to avoid backing array aliasing
	// with closedSubmission slices — otherwise new appends could overwrite
	// closed submission data before recall() processes it.
	b.activeChunks = nil
	b.oversized = nil
}

// setLastSubmissionIndex updates the most recently finished batch with the
// actual GPU submission index (known only after HAL Submit returns).
// Must be called after finish() and after HAL Submit.
func (b *stagingBelt) setLastSubmissionIndex(index uint64) {
	if len(b.closedSubmissions) > 0 {
		b.closedSubmissions[len(b.closedSubmissions)-1].submissionIndex = index
	}
}

// recall recycles chunks from completed submissions back to the free pool.
// Called from maintain() after the GPU reports completion of a submission.
func (b *stagingBelt) recall(completedIndex uint64) {
	cutoff := 0
	for i, sub := range b.closedSubmissions {
		if sub.submissionIndex > completedIndex {
			break
		}
		cutoff = i + 1

		// Recycle regular chunks (reset bump pointer, move to free pool).
		for j := range sub.chunks {
			sub.chunks[j].offset = 0
			b.freeChunks = append(b.freeChunks, sub.chunks[j])
		}

		// Destroy oversized one-off buffers (too large to pool).
		for _, buf := range sub.oversized {
			b.halDevice.DestroyBuffer(buf)
		}
	}

	if cutoff > 0 {
		b.closedSubmissions = b.closedSubmissions[cutoff:]
	}
}

// createChunk allocates a new staging chunk buffer.
func (b *stagingBelt) createChunk(size uint64) (stagingChunk, error) {
	desc := hal.BufferDescriptor{
		Label:            "(wgpu internal) staging chunk",
		Size:             size,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	}
	buf, err := b.halDevice.CreateBuffer(&desc)
	if err != nil {
		return stagingChunk{}, fmt.Errorf("staging belt: create chunk (%d bytes): %w", size, err)
	}
	return stagingChunk{buffer: buf, size: size, offset: 0}, nil
}

// tryAllocate checks if `size` bytes fit in the chunk and reserves the space.
// Returns the allocation start offset and true if it fits. On success, the
// caller MUST write data; on WriteBuffer failure, call rollback(alignedSize).
func (c *stagingChunk) tryAllocate(alignedSize uint64) (stagingAllocation, bool) {
	start := c.offset
	end := start + alignedSize
	if end > c.size {
		return stagingAllocation{}, false
	}
	c.offset = end
	return stagingAllocation{buffer: c.buffer, offset: start}, true
}

// rollback reverts a tryAllocate when the subsequent WriteBuffer fails.
// This prevents leaving a gap in the chunk (DeepSeek review finding).
func (c *stagingChunk) rollback(alignedSize uint64) {
	if c.offset >= alignedSize {
		c.offset -= alignedSize
	}
}

// destroy releases all chunk buffers (active, free, and closed).
func (b *stagingBelt) destroy() {
	for _, chunk := range b.activeChunks {
		b.halDevice.DestroyBuffer(chunk.buffer)
	}
	b.activeChunks = nil

	for _, chunk := range b.freeChunks {
		b.halDevice.DestroyBuffer(chunk.buffer)
	}
	b.freeChunks = nil

	for _, sub := range b.closedSubmissions {
		for _, chunk := range sub.chunks {
			b.halDevice.DestroyBuffer(chunk.buffer)
		}
		for _, buf := range sub.oversized {
			b.halDevice.DestroyBuffer(buf)
		}
	}
	b.closedSubmissions = nil

	for _, buf := range b.oversized {
		b.halDevice.DestroyBuffer(buf)
	}
	b.oversized = nil
}

// beltStats holds belt statistics for diagnostics/logging.
type beltStats struct {
	ActiveChunks   int
	FreeChunks     int
	ClosedSubs     int
	TotalAllocated uint64
}

// stats returns belt statistics for diagnostics/logging.
func (b *stagingBelt) stats() beltStats {
	s := beltStats{
		ActiveChunks: len(b.activeChunks),
		FreeChunks:   len(b.freeChunks),
		ClosedSubs:   len(b.closedSubmissions),
	}
	for _, c := range b.activeChunks {
		s.TotalAllocated += c.size
	}
	for _, c := range b.freeChunks {
		s.TotalAllocated += c.size
	}
	for _, sub := range b.closedSubmissions {
		for _, c := range sub.chunks {
			s.TotalAllocated += c.size
		}
	}
	return s
}

// alignUp64 rounds n up to the nearest multiple of alignment.
func alignUp64(n, alignment uint64) uint64 {
	if alignment == 0 {
		return n
	}
	return (n + alignment - 1) / alignment * alignment
}
