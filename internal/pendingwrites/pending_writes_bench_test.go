//go:build !rust && !(js && wasm)

package pendingwrites

import (
	"testing"

	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/staging"
)

// benchQueue is the zero-allocation queue double for benchmarks: unlike
// testQueue it records nothing, so the measured allocations are the
// pending-writes path's own.
type benchQueue struct {
	hal.Queue
	batching bool
}

func (q *benchQueue) SupportsCommandBufferCopies() bool { return q.batching }

func (q *benchQueue) WriteBuffer(hal.Buffer, uint64, []byte) error { return nil }

// BenchmarkPendingWrites_WriteBufferNonBatching measures the hot path for
// GLES/Software backends where WriteBuffer delegates directly to HAL.
func BenchmarkPendingWrites_WriteBufferNonBatching(b *testing.B) {
	dev := &testDevice{}
	q := &benchQueue{}
	pw := New(dev, q, nil, nil)
	defer pw.Destroy()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 1024})
	data := make([]byte, 256)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = pw.WriteBuffer(nil, buf, track.InvalidTrackerIndex, 0, data)
	}
}

// BenchmarkPendingWrites_WriteBufferBatching measures the staging path for
// DX12/Vulkan/Metal backends with StagingBelt (ring-buffer chunks).
// Simulates the real frame loop: write → flush → submit → maintain(recall).
// After warmup, steady-state should show 0 allocs (chunks recycled).
//
// Uses benchQueue (not the recording test queue) to avoid mock overhead that
// would pollute alloc measurements.
func BenchmarkPendingWrites_WriteBufferBatching(b *testing.B) {
	dev := &testDevice{}
	q := &benchQueue{batching: true}
	pool := &testEncoderPool{device: dev}
	defer pool.destroy()
	pw := New(dev, q, pool, nil)
	defer pw.Destroy()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 1024})
	data := make([]byte, 256)

	// Warmup: create chunk, flush, recall — so steady-state has a free chunk.
	_ = pw.WriteBuffer(nil, buf, track.InvalidTrackerIndex, 0, data)
	pw.mu.Lock()
	pw.flush()
	if pw.belt != nil {
		pw.belt.SetLastSubmissionIndex(1)
	}
	pw.maintain(1) // recall completed → chunk moves to freeChunks
	pw.mu.Unlock()

	b.ResetTimer()
	b.ReportAllocs()
	subIdx := uint64(2)
	for i := 0; i < b.N; i++ {
		_ = pw.WriteBuffer(nil, buf, track.InvalidTrackerIndex, 0, data)
		// Simulate frame boundary: flush + maintain every 20 writes (typical frame).
		if i%20 == 19 {
			pw.mu.Lock()
			pw.flush()
			if pw.belt != nil {
				pw.belt.SetLastSubmissionIndex(subIdx)
			}
			pw.maintain(subIdx) // recall immediately (the doubles are synchronous)
			subIdx++
			pw.mu.Unlock()
		}
	}
}

// BenchmarkPendingWrites_FlushEmpty measures flush with no pending work.
func BenchmarkPendingWrites_FlushEmpty(b *testing.B) {
	dev := &testDevice{}
	q := &benchQueue{batching: true}
	pool := &testEncoderPool{device: dev}
	defer pool.destroy()
	pw := New(dev, q, pool, nil)
	defer pw.Destroy()

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
	dev := &testDevice{}
	q := &benchQueue{}
	pw := New(dev, q, nil, nil)
	defer pw.Destroy()

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

// BenchmarkStagingBelt_AllocateSteadyState measures pure belt allocation
// without mock overhead. After warmup, this is the true zero-alloc path.
func BenchmarkStagingBelt_AllocateSteadyState(b *testing.B) {
	dev := &testDevice{}
	q := &benchQueue{}
	belt := staging.New(dev, q, 0, 0, 0)
	defer belt.Destroy()

	data := make([]byte, 256)

	// Warmup: create chunk, finish, recall.
	belt.Allocate(256, data)
	belt.Finish(1)
	belt.Recall(1)

	b.ResetTimer()
	b.ReportAllocs()
	subIdx := uint64(2)
	for i := 0; i < b.N; i++ {
		belt.Allocate(256, data)
		if i%20 == 19 {
			belt.Finish(subIdx)
			belt.Recall(subIdx)
			subIdx++
		}
	}
}
