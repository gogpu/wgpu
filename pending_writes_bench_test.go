package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// BenchmarkPendingWrites_WriteBufferNonBatching measures the hot path for
// GLES/Software backends where WriteBuffer delegates directly to HAL.
func BenchmarkPendingWrites_WriteBufferNonBatching(b *testing.B) {
	dev := &noop.Device{}
	q := &noop.Queue{}
	pw := newPendingWrites(dev, q)
	defer pw.destroy()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 1024})
	data := make([]byte, 256)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = pw.writeBuffer(buf, gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst, 0, data)
	}
}

// BenchmarkPendingWrites_WriteBufferBatching measures the staging path for
// DX12/Vulkan/Metal backends with StagingBelt (ring-buffer chunks).
// Simulates the real frame loop: write → flush → submit → maintain(recall).
// After warmup, steady-state should show 0 allocs (chunks recycled).
func BenchmarkPendingWrites_WriteBufferBatching(b *testing.B) {
	dev := &noop.Device{}
	q := &mockBatchingQueue{Queue: noop.Queue{}}
	pw := newPendingWrites(dev, q)
	defer pw.destroy()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 1024})
	data := make([]byte, 256)
	usage := gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst

	// Warmup: create chunk, flush, recall — so steady-state has a free chunk.
	_ = pw.writeBuffer(buf, usage, 0, data)
	pw.mu.Lock()
	pw.flush()
	if pw.belt != nil {
		pw.belt.setLastSubmissionIndex(1)
	}
	pw.maintain(1) // recall completed → chunk moves to freeChunks
	pw.mu.Unlock()

	b.ResetTimer()
	b.ReportAllocs()
	subIdx := uint64(2)
	for i := 0; i < b.N; i++ {
		_ = pw.writeBuffer(buf, usage, 0, data)
		// Simulate frame boundary: flush + maintain every 20 writes (typical frame).
		if i%20 == 19 {
			pw.mu.Lock()
			pw.flush()
			if pw.belt != nil {
				pw.belt.setLastSubmissionIndex(subIdx)
			}
			pw.maintain(subIdx) // recall immediately (noop is synchronous)
			subIdx++
			pw.mu.Unlock()
		}
	}
}

// BenchmarkPendingWrites_FlushEmpty measures flush with no pending work.
func BenchmarkPendingWrites_FlushEmpty(b *testing.B) {
	dev := &noop.Device{}
	q := &mockBatchingQueue{Queue: noop.Queue{}}
	pw := newPendingWrites(dev, q)
	defer pw.destroy()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pw.mu.Lock()
		pw.flush()
		pw.mu.Unlock()
	}
}

// BenchmarkPendingWrites_Maintain measures cleanup of completed submissions.
func BenchmarkPendingWrites_Maintain(b *testing.B) {
	dev := &noop.Device{}
	q := &noop.Queue{}
	pw := newPendingWrites(dev, q)
	defer pw.destroy()

	// Pre-populate inflight list
	for i := 0; i < 10; i++ {
		pw.inflight = append(pw.inflight, inflightSubmission{
			submissionIndex: uint64(i + 100),
		})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pw.maintain(uint64(i % 105))
	}
}

// BenchmarkAlignUp measures the alignment helper.
func BenchmarkAlignUp(b *testing.B) {
	b.ReportAllocs()
	var sum uint32
	for i := 0; i < b.N; i++ {
		sum += alignUp(uint32(i), 256)
	}
	_ = sum
}

// BenchmarkBufferReadUsage measures the read-usage extraction.
func BenchmarkBufferReadUsage(b *testing.B) {
	b.ReportAllocs()
	var sum gputypes.BufferUsage
	for i := 0; i < b.N; i++ {
		sum |= bufferReadUsage(gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst | gputypes.BufferUsageUniform)
	}
	_ = sum
}

// BenchmarkStagingBelt_AllocateSteadyState measures pure belt allocation
// without mock overhead. After warmup, this is the true zero-alloc path.
func BenchmarkStagingBelt_AllocateSteadyState(b *testing.B) {
	dev := &noop.Device{}
	q := &noop.Queue{}
	belt := newStagingBelt(dev, q, 0, 0)
	defer belt.destroy()

	data := make([]byte, 256)

	// Warmup: create chunk, finish, recall.
	belt.allocate(256, data)
	belt.finish(1)
	belt.recall(1)

	b.ResetTimer()
	b.ReportAllocs()
	subIdx := uint64(2)
	for i := 0; i < b.N; i++ {
		belt.allocate(256, data)
		if i%20 == 19 {
			belt.finish(subIdx)
			belt.recall(subIdx)
			subIdx++
		}
	}
}
