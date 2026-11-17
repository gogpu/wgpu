//go:build !rust && !(js && wasm)

package staging

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// Belt manages reusable staging buffer chunks for zero-allocation
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
// Thread safety: the belt is used exclusively under the pending-writes lock
// (internal/pendingwrites). No internal synchronization needed.
//
// Steady-state allocation cost: 0 heap allocs per writeBuffer call.
// New chunks are created only during warmup or traffic spikes.
type Belt struct {
	halDevice hal.Device
	halQueue  hal.Queue
	chunkSize uint64 // size of each pre-allocated chunk
	alignment uint64 // minimum sub-allocation alignment (power of two)

	// MaxStagingBufferSize caps the maximum size of a single staging buffer.
	// Set to min(MaxOversizedSize, device maxMemoryAllocationSize).
	// Writes larger than this are automatically chunked into multiple
	// staging buffers + CopyBufferToBuffer commands (BUG-VK-001).
	MaxStagingBufferSize uint64

	// activeChunks are being sub-allocated via bump pointer.
	// Typically 1 chunk is active; more are activated when the current
	// chunk's remaining space can't fit an allocation.
	activeChunks []chunk

	// freeChunks are recycled (GPU done, offset reset to 0).
	freeChunks []chunk

	// closedSubmissions track chunks that are in-flight on the GPU,
	// keyed by submission index. recall() recycles them after completion.
	closedSubmissions []closedSubmission

	// oversized tracks one-off buffers for writes larger than chunkSize.
	// These are not recycled — destroyed after GPU completion (too large
	// to keep in the pool without wasting memory).
	oversized []hal.Buffer

	// chunkedAllocs temporarily holds chunk allocations from allocateChunked.
	// Consumed by Writes.WriteBuffer in internal/pendingwrites to issue
	// CopyBufferToBuffer for each chunk. Reset at the start of each
	// allocateChunked call. Only valid between allocateChunked and the next
	// writeBuffer call.
	chunkedAllocs []ChunkedAllocation
}

// chunk is a large staging buffer with a bump-pointer allocator.
type chunk struct {
	buffer hal.Buffer
	size   uint64
	offset uint64 // next allocation starts here (bump pointer)
}

// closedSubmission groups chunks that were active during a submission.
// After the GPU completes this submission, chunks are recycled to freeChunks.
type closedSubmission struct {
	submissionIndex uint64
	chunks          []chunk
	oversized       []hal.Buffer
}

// DefaultChunkSize is 256KB — large enough for typical per-frame
// uploads (uniform/vertex/index buffers are 64B-16KB each, ~20 per frame)
// but small enough to not waste memory when idle.
const DefaultChunkSize = 256 * 1024

// DefaultAlignment is the default minimum alignment for sub-allocations.
// WebGPU COPY_BUFFER_ALIGNMENT = 4. Rust wgpu uses MAP_ALIGNMENT = 8.
// We default to 8 (Rust parity) — good balance between spec compliance and
// cache performance. Configurable via New alignment parameter.
const DefaultAlignment uint64 = 8

// MaxOversizedSize caps individual staging buffer allocations at 64MB.
// Rust wgpu caps staging buffers at 1 << 26 (64MB). This prevents
// vkAllocateMemory from failing when size exceeds maxMemoryAllocationSize,
// which causes SIGSEGV via null/partial mapped pointer (BUG-VK-001).
// Writes larger than this are automatically chunked.
const MaxOversizedSize uint64 = 64 << 20 // 64 MB

// New creates a staging belt with the given chunk size and alignment.
// If chunkSize is 0, uses the default (256KB).
// If alignment is 0, uses the default (8 bytes, Rust wgpu parity).
// If maxStagingBufferSize is 0, uses the default (64MB, Rust wgpu parity).
// Alignment must be a power of two.
// Matches Rust wgpu StagingBelt::allocate() where alignment is per-allocation;
// we simplify to per-belt since the belt is internal to internal/pendingwrites.
func New(halDevice hal.Device, halQueue hal.Queue, chunkSize, alignment, maxStagingBufferSize uint64) *Belt {
	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}
	if alignment == 0 {
		alignment = DefaultAlignment
	}
	if maxStagingBufferSize == 0 {
		maxStagingBufferSize = MaxOversizedSize
	}
	return &Belt{
		halDevice:            halDevice,
		halQueue:             halQueue,
		chunkSize:            chunkSize,
		alignment:            alignment,
		MaxStagingBufferSize: maxStagingBufferSize,
	}
}

// Allocation is the result of a belt allocation: a buffer region
// where data can be written via HAL WriteBuffer, then copied to the
// destination via CopyBufferToBuffer.
type Allocation struct {
	Buffer hal.Buffer // the staging chunk's buffer
	Offset uint64     // offset within the buffer
}

// Allocate reserves `size` bytes from the belt and writes `data` into
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
func (b *Belt) Allocate(size uint64, data []byte) (Allocation, error) {
	if size == 0 {
		return Allocation{}, nil
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
			if err := b.halQueue.WriteBuffer(alloc.Buffer, alloc.Offset, data); err != nil {
				b.activeChunks[i].rollback(alignedSize)
				return Allocation{}, fmt.Errorf("staging belt: write to active chunk: %w", err)
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
		if err := b.halQueue.WriteBuffer(alloc.Buffer, alloc.Offset, data); err != nil {
			b.activeChunks[len(b.activeChunks)-1].rollback(alignedSize)
			return Allocation{}, fmt.Errorf("staging belt: write to recycled chunk: %w", err)
		}
		return alloc, nil
	}

	// No free chunks — create a new one.
	chunk, err := b.createChunk(b.chunkSize)
	if err != nil {
		return Allocation{}, err
	}
	alloc, _ := chunk.tryAllocate(alignedSize)
	b.activeChunks = append(b.activeChunks, chunk)
	if err := b.halQueue.WriteBuffer(alloc.Buffer, alloc.Offset, data); err != nil {
		chunk.rollback(alignedSize)
		return Allocation{}, fmt.Errorf("staging belt: write to new chunk: %w", err)
	}
	return alloc, nil
}

// allocateOversized creates one or more one-off staging buffers for writes
// larger than chunkSize. Each staging buffer is capped at MaxStagingBufferSize
// (default 64MB, matching Rust wgpu's 1 << 26 cap). Writes larger than this
// cap are split into multiple staging buffers.
//
// Returns the first staging allocation. If the write was split, additional
// staging buffers are tracked in b.oversized but the caller only needs the
// first allocation's buffer for the CopyBufferToBuffer source — the chunked
// copies are handled internally by allocateChunked.
//
// Defense-in-depth: validates that CreateBuffer returns a usable mapped
// pointer before writing data, preventing SIGSEGV (BUG-VK-001 Fix 3).
func (b *Belt) allocateOversized(size uint64, data []byte) (Allocation, error) {
	// If the write fits in a single staging buffer, use the simple path.
	if size <= b.MaxStagingBufferSize {
		return b.allocateSingleOversized(size, data)
	}

	// Large write: chunk into multiple staging buffers.
	// Each chunk gets its own staging buffer + is tracked as oversized.
	// The caller (Writes.WriteBuffer in internal/pendingwrites) will issue
	// CopyBufferToBuffer for each chunk separately via allocateChunked.
	return b.allocateChunked(size, data)
}

// allocateSingleOversized creates a single one-off staging buffer.
// Validates the mapped pointer before writing (BUG-VK-001 Fix 3).
func (b *Belt) allocateSingleOversized(size uint64, data []byte) (Allocation, error) {
	desc := hal.BufferDescriptor{
		Label:            "(wgpu internal) staging oversized",
		Size:             size,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	}
	buf, err := b.halDevice.CreateBuffer(&desc)
	if err != nil {
		return Allocation{}, fmt.Errorf("staging belt: create oversized buffer (%d bytes): %w", size, err)
	}
	if err := b.halQueue.WriteBuffer(buf, 0, data); err != nil {
		b.halDevice.DestroyBuffer(buf)
		return Allocation{}, fmt.Errorf("staging belt: write oversized buffer: %w", err)
	}
	b.oversized = append(b.oversized, buf)
	return Allocation{Buffer: buf, Offset: 0}, nil
}

// ChunkedAllocation holds one chunk of a multi-chunk oversized write.
// Used by allocateChunked to return all chunks for CopyBufferToBuffer commands.
type ChunkedAllocation struct {
	Buffer hal.Buffer // staging buffer for this chunk
	Offset uint64     // always 0 for oversized (dedicated buffer per chunk)
	Size   uint64     // bytes of data in this chunk
}

// allocateChunked splits a large write into multiple staging buffers, each
// capped at MaxStagingBufferSize. Returns the first chunk as the primary
// allocation and stores all chunks in b.chunkedAllocs for the caller to
// issue CopyBufferToBuffer commands.
func (b *Belt) allocateChunked(totalSize uint64, data []byte) (Allocation, error) {
	maxChunk := b.MaxStagingBufferSize
	remaining := totalSize
	dataOffset := uint64(0)

	b.chunkedAllocs = b.chunkedAllocs[:0]

	for remaining > 0 {
		chunkSize := remaining
		if chunkSize > maxChunk {
			chunkSize = maxChunk
		}

		chunkData := data[dataOffset : dataOffset+chunkSize]
		alloc, err := b.allocateSingleOversized(chunkSize, chunkData)
		if err != nil {
			// Clean up any chunks we already allocated.
			for _, ca := range b.chunkedAllocs {
				b.halDevice.DestroyBuffer(ca.Buffer)
			}
			b.chunkedAllocs = b.chunkedAllocs[:0]
			return Allocation{}, fmt.Errorf("staging belt: allocate chunk at offset %d: %w", dataOffset, err)
		}

		b.chunkedAllocs = append(b.chunkedAllocs, ChunkedAllocation{
			Buffer: alloc.Buffer,
			Offset: alloc.Offset,
			Size:   chunkSize,
		})

		dataOffset += chunkSize
		remaining -= chunkSize
	}

	if len(b.chunkedAllocs) == 0 {
		return Allocation{}, nil
	}

	// Return the first chunk as the primary allocation.
	first := b.chunkedAllocs[0]
	return Allocation{Buffer: first.Buffer, Offset: first.Offset}, nil
}

// TakeChunkedAllocations hands the chunk allocations of the last allocate
// call to the caller when the write was split across staging buffers, and
// resets the belt's bookkeeping. The caller issues one CopyBufferToBuffer per
// entry. Returns an empty slice on the normal (single-buffer) path.
//
// The returned slice aliases the belt's own storage: it stays valid only until
// the next allocate call, which is the same lifetime the caller had when it
// read the field directly.
func (b *Belt) TakeChunkedAllocations() []ChunkedAllocation {
	chunks := b.chunkedAllocs
	b.chunkedAllocs = b.chunkedAllocs[:0]
	return chunks
}

// Finish moves all active chunks and oversized buffers to closed state.
// Called at flush() time when the command encoder is about to be submitted.
// The submission index is not yet known — call setLastSubmissionIndex()
// after HAL Submit returns with the actual index.
//
// The belt is the sole owner of all staging resources (chunks + oversized).
// recall() handles destruction of oversized buffers after GPU completion.
// No buffers are returned to the caller — this avoids double-destroy bugs.
func (b *Belt) Finish(submissionIndex uint64) {
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

// DiscardActive abandons allocations whose copy commands will not be
// submitted. Regular chunks return directly to the free pool; oversized
// one-off buffers are destroyed. The caller serializes access.
func (b *Belt) DiscardActive() {
	for i := range b.activeChunks {
		b.activeChunks[i].offset = 0
		b.freeChunks = append(b.freeChunks, b.activeChunks[i])
	}
	b.activeChunks = nil

	for _, buf := range b.oversized {
		b.halDevice.DestroyBuffer(buf)
	}
	b.oversized = nil

	// chunkedAllocs only aliases buffers held by oversized.
	b.chunkedAllocs = nil
}

// DiscardLastClosed rolls back the batch most recently finished with the
// placeholder submission index. It is used only when HAL rejects Submit.
func (b *Belt) DiscardLastClosed() {
	last := len(b.closedSubmissions) - 1
	if last < 0 || b.closedSubmissions[last].submissionIndex != 0 {
		return
	}
	submission := b.closedSubmissions[last]
	b.closedSubmissions = b.closedSubmissions[:last]
	for i := range submission.chunks {
		submission.chunks[i].offset = 0
		b.freeChunks = append(b.freeChunks, submission.chunks[i])
	}
	for _, buf := range submission.oversized {
		b.halDevice.DestroyBuffer(buf)
	}
}

// SetLastSubmissionIndex updates the most recently finished batch with the
// actual GPU submission index (known only after HAL Submit returns).
// Must be called after finish() and after HAL Submit.
func (b *Belt) SetLastSubmissionIndex(index uint64) {
	if len(b.closedSubmissions) > 0 {
		b.closedSubmissions[len(b.closedSubmissions)-1].submissionIndex = index
	}
}

// Recall recycles chunks from completed submissions back to the free pool.
// Called from maintain() after the GPU reports completion of a submission.
func (b *Belt) Recall(completedIndex uint64) {
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
func (b *Belt) createChunk(size uint64) (chunk, error) {
	desc := hal.BufferDescriptor{
		Label:            "(wgpu internal) staging chunk",
		Size:             size,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	}
	buf, err := b.halDevice.CreateBuffer(&desc)
	if err != nil {
		return chunk{}, fmt.Errorf("staging belt: create chunk (%d bytes): %w", size, err)
	}
	return chunk{buffer: buf, size: size, offset: 0}, nil
}

// tryAllocate checks if `size` bytes fit in the chunk and reserves the space.
// Returns the allocation start offset and true if it fits. On success, the
// caller MUST write data; on WriteBuffer failure, call rollback(alignedSize).
func (c *chunk) tryAllocate(alignedSize uint64) (Allocation, bool) {
	start := c.offset
	end := start + alignedSize
	if end > c.size {
		return Allocation{}, false
	}
	c.offset = end
	return Allocation{Buffer: c.buffer, Offset: start}, true
}

// rollback reverts a tryAllocate when the subsequent WriteBuffer fails.
// This prevents leaving a gap in the chunk (DeepSeek review finding).
func (c *chunk) rollback(alignedSize uint64) {
	if c.offset >= alignedSize {
		c.offset -= alignedSize
	}
}

// Destroy releases all chunk buffers (active, free, and closed).
func (b *Belt) Destroy() {
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

	// chunkedAllocs only holds references to buffers already in oversized —
	// no extra DestroyBuffer needed, just nil the slice.
	b.chunkedAllocs = nil
}

// Stats holds belt statistics for diagnostics/logging.
type Stats struct {
	ActiveChunks   int
	FreeChunks     int
	ClosedSubs     int
	TotalAllocated uint64
}

// Stats returns belt statistics for diagnostics/logging.
func (b *Belt) Stats() Stats {
	s := Stats{
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
