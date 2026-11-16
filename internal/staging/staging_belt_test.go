//go:build !rust && !(js && wasm)

package staging

import (
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

// The doubles are local on purpose: a test import is a graph edge like any
// other, and the destination's only claimed edge is staging -> hal.

// testDevice is the HAL device double: the belt creates and destroys staging
// buffers through it, and every CreateBuffer returns a distinct buffer so the
// tests can compare chunk identity.
type testDevice struct {
	hal.Device
	destroyed []hal.Buffer
}

func newTestDevice() *testDevice { return &testDevice{} }

func (d *testDevice) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	return &testBuffer{}, nil
}

func (d *testDevice) DestroyBuffer(buffer hal.Buffer) {
	d.destroyed = append(d.destroyed, buffer)
}

// testBuffer is the opaque HAL buffer double: the belt only stores, compares
// and destroys it.
type testBuffer struct{ hal.Buffer }

// testQueue is the HAL queue double: staging writes are accepted and dropped.
type testQueue struct{ hal.Queue }

func (q *testQueue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error { return nil }

func createTestBelt(t *testing.T, chunkSize uint64) *Belt {
	t.Helper()
	dev := newTestDevice()
	q := &testQueue{}
	return New(dev, q, chunkSize, 0, 0)
}

func TestStagingBelt_AllocateSubAllocates(t *testing.T) {
	belt := createTestBelt(t, 1024)
	defer belt.Destroy()

	// Two small allocations should fit in the same chunk.
	data1 := make([]byte, 32)
	data2 := make([]byte, 64)

	alloc1, err := belt.Allocate(32, data1)
	if err != nil {
		t.Fatalf("allocate 1: %v", err)
	}
	alloc2, err := belt.Allocate(64, data2)
	if err != nil {
		t.Fatalf("allocate 2: %v", err)
	}

	// Both allocations should be from the same buffer (same chunk).
	if alloc1.Buffer != alloc2.Buffer {
		t.Error("expected both allocations from the same chunk buffer")
	}

	// Second allocation offset should be after first (aligned to 16).
	expectedOffset := alignUp64(32, DefaultAlignment)
	if alloc2.Offset != expectedOffset {
		t.Errorf("expected alloc2.Offset=%d, got %d", expectedOffset, alloc2.Offset)
	}

	// Only 1 active chunk.
	s := belt.Stats()
	active, free, closed := s.ActiveChunks, s.FreeChunks, s.ClosedSubs
	if active != 1 {
		t.Errorf("expected 1 active chunk, got %d", active)
	}
	if free != 0 {
		t.Errorf("expected 0 free chunks, got %d", free)
	}
	if closed != 0 {
		t.Errorf("expected 0 closed submissions, got %d", closed)
	}
}

func TestStagingBelt_AllocateRecyclesChunk(t *testing.T) {
	belt := createTestBelt(t, 1024)
	defer belt.Destroy()

	// Allocate to create a chunk.
	data := make([]byte, 16)
	if _, err := belt.Allocate(16, data); err != nil {
		t.Fatalf("allocate: %v", err)
	}

	// Finish moves active chunks to closed.
	belt.Finish(1)

	s := belt.Stats()
	if s.ActiveChunks != 0 {
		t.Errorf("after finish: expected 0 active, got %d", s.ActiveChunks)
	}
	if s.ClosedSubs != 1 {
		t.Errorf("after finish: expected 1 closed, got %d", s.ClosedSubs)
	}

	// Recall recycles closed chunks to free.
	belt.Recall(1)

	s = belt.Stats()
	if s.FreeChunks != 1 {
		t.Errorf("after recall: expected 1 free, got %d", s.FreeChunks)
	}
	if s.ClosedSubs != 0 {
		t.Errorf("after recall: expected 0 closed, got %d", s.ClosedSubs)
	}

	// Next allocation should reuse the free chunk (no new chunk created).
	alloc, err := belt.Allocate(16, data)
	if err != nil {
		t.Fatalf("allocate after recall: %v", err)
	}
	if alloc.Buffer == nil {
		t.Error("expected non-nil buffer from recycled chunk")
	}

	s = belt.Stats()
	if s.ActiveChunks != 1 {
		t.Errorf("after reuse: expected 1 active, got %d", s.ActiveChunks)
	}
	if s.FreeChunks != 0 {
		t.Errorf("after reuse: expected 0 free (chunk was recycled), got %d", s.FreeChunks)
	}
}

func TestStagingBelt_AllocateOversized(t *testing.T) {
	chunkSize := uint64(256)
	belt := createTestBelt(t, chunkSize)
	defer belt.Destroy()

	// Allocate more than chunkSize — should create a one-off buffer.
	oversizedData := make([]byte, chunkSize+100)
	alloc, err := belt.Allocate(chunkSize+100, oversizedData)
	if err != nil {
		t.Fatalf("allocate oversized: %v", err)
	}

	if alloc.Buffer == nil {
		t.Error("expected non-nil buffer for oversized allocation")
	}
	if alloc.Offset != 0 {
		t.Errorf("expected offset=0 for oversized, got %d", alloc.Offset)
	}

	// Oversized goes to belt.oversized, not activeChunks.
	s := belt.Stats()
	active := s.ActiveChunks
	if active != 0 {
		t.Errorf("expected 0 active chunks for oversized alloc, got %d", active)
	}
	if len(belt.oversized) != 1 {
		t.Errorf("expected 1 oversized buffer, got %d", len(belt.oversized))
	}

	// finish() moves oversized to closedSubmissions (belt owns them).
	belt.Finish(1)
	s = belt.Stats()
	if s.ClosedSubs != 1 {
		t.Errorf("expected 1 closed submission after finish, got %d", s.ClosedSubs)
	}
}

func TestStagingBelt_DiscardPathsRetireOnlyUnsubmittedBatches(t *testing.T) {
	t.Run("active allocations", func(t *testing.T) {
		belt := createTestBelt(t, 64)
		defer belt.Destroy()

		if _, err := belt.Allocate(16, make([]byte, 16)); err != nil {
			t.Fatalf("allocate regular chunk: %v", err)
		}
		if _, err := belt.Allocate(96, make([]byte, 96)); err != nil {
			t.Fatalf("allocate oversized buffer: %v", err)
		}
		belt.DiscardActive()
		stats := belt.Stats()
		if stats.ActiveChunks != 0 || stats.FreeChunks != 1 || len(belt.oversized) != 0 || len(belt.chunkedAllocs) != 0 {
			t.Fatalf("discarded active state = %+v oversized=%d chunked=%d", stats, len(belt.oversized), len(belt.chunkedAllocs))
		}
	})

	t.Run("closed submissions", func(t *testing.T) {
		belt := createTestBelt(t, 64)
		defer belt.Destroy()
		belt.DiscardLastClosed()

		if _, err := belt.Allocate(16, make([]byte, 16)); err != nil {
			t.Fatalf("allocate placeholder batch: %v", err)
		}
		if _, err := belt.Allocate(96, make([]byte, 96)); err != nil {
			t.Fatalf("allocate placeholder oversized buffer: %v", err)
		}
		belt.Finish(0)
		belt.DiscardLastClosed()
		stats := belt.Stats()
		if stats.ClosedSubs != 0 || stats.FreeChunks != 1 {
			t.Fatalf("rolled-back placeholder state = %+v", stats)
		}

		if _, err := belt.Allocate(16, make([]byte, 16)); err != nil {
			t.Fatalf("allocate submitted batch: %v", err)
		}
		belt.Finish(7)
		belt.DiscardLastClosed()
		if stats := belt.Stats(); stats.ClosedSubs != 1 {
			t.Fatalf("submitted batch was discarded: %+v", stats)
		}
	})
}

func TestStagingBelt_FinishAndRecall(t *testing.T) {
	belt := createTestBelt(t, 1024)
	defer belt.Destroy()

	data := make([]byte, 32)

	// Submission 1: allocate, finish.
	if _, err := belt.Allocate(32, data); err != nil {
		t.Fatalf("alloc 1: %v", err)
	}
	belt.Finish(0)
	belt.SetLastSubmissionIndex(10)

	// Submission 2: allocate (new chunk since previous was closed), finish.
	if _, err := belt.Allocate(32, data); err != nil {
		t.Fatalf("alloc 2: %v", err)
	}
	belt.Finish(0)
	belt.SetLastSubmissionIndex(20)

	s := belt.Stats()
	active, free, closed := s.ActiveChunks, s.FreeChunks, s.ClosedSubs
	if active != 0 {
		t.Errorf("expected 0 active, got %d", active)
	}
	if free != 0 {
		t.Errorf("expected 0 free, got %d", free)
	}
	if closed != 2 {
		t.Errorf("expected 2 closed, got %d", closed)
	}

	// Recall submission 1 only (completedIndex=10).
	belt.Recall(10)

	s = belt.Stats()
	free, closed = s.FreeChunks, s.ClosedSubs
	if free != 1 {
		t.Errorf("after recall(10): expected 1 free, got %d", free)
	}
	if closed != 1 {
		t.Errorf("after recall(10): expected 1 closed, got %d", closed)
	}

	// Recall submission 2 (completedIndex=20).
	belt.Recall(20)

	s = belt.Stats()
	free, closed = s.FreeChunks, s.ClosedSubs
	if free != 2 {
		t.Errorf("after recall(20): expected 2 free, got %d", free)
	}
	if closed != 0 {
		t.Errorf("after recall(20): expected 0 closed, got %d", closed)
	}
}

func TestStagingBelt_ZeroAllocSteadyState(t *testing.T) {
	belt := createTestBelt(t, 1024)
	defer belt.Destroy()

	data := make([]byte, 64)

	// Warmup: create one chunk.
	if _, err := belt.Allocate(64, data); err != nil {
		t.Fatalf("warmup alloc: %v", err)
	}
	belt.Finish(0)
	belt.SetLastSubmissionIndex(1)
	belt.Recall(1)

	// After warmup, we should have 1 free chunk.
	s := belt.Stats()
	free := s.FreeChunks
	if free != 1 {
		t.Fatalf("expected 1 free chunk after warmup, got %d", free)
	}

	// Steady state: allocate + finish + recall should reuse the same chunk.
	// No new chunks should be created.
	for i := uint64(0); i < 100; i++ {
		if _, err := belt.Allocate(64, data); err != nil {
			t.Fatalf("steady state alloc %d: %v", i, err)
		}

		belt.Finish(0)
		belt.SetLastSubmissionIndex(100 + i)
		belt.Recall(100 + i)
	}

	s = belt.Stats()
	active, free, closed, totalAllocated := s.ActiveChunks, s.FreeChunks, s.ClosedSubs, s.TotalAllocated
	if active != 0 {
		t.Errorf("steady state: expected 0 active, got %d", active)
	}
	if free != 1 {
		t.Errorf("steady state: expected 1 free chunk (no new allocs), got %d", free)
	}
	if closed != 0 {
		t.Errorf("steady state: expected 0 closed, got %d", closed)
	}
	// Total allocated should be exactly 1 chunk (1024 bytes).
	if totalAllocated != 1024 {
		t.Errorf("steady state: expected totalAllocated=1024, got %d", totalAllocated)
	}
}

func TestStagingBelt_AlignUp64(t *testing.T) {
	tests := []struct {
		name      string
		n         uint64
		alignment uint64
		want      uint64
	}{
		{"already aligned", 16, 16, 16},
		{"needs alignment", 10, 16, 16},
		{"zero", 0, 16, 0},
		{"alignment 0", 42, 0, 42},
		{"alignment 1", 42, 1, 42},
		{"large value", 1000, 256, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alignUp64(tt.n, tt.alignment)
			if got != tt.want {
				t.Errorf("alignUp64(%d, %d) = %d, want %d", tt.n, tt.alignment, got, tt.want)
			}
		})
	}
}

func TestStagingBelt_DefaultChunkSize(t *testing.T) {
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 0, 0, 0) // 0 = defaults
	defer belt.Destroy()

	if belt.chunkSize != DefaultChunkSize {
		t.Errorf("expected default chunkSize=%d, got %d", DefaultChunkSize, belt.chunkSize)
	}
}

func TestStagingBelt_EmptyAllocate(t *testing.T) {
	belt := createTestBelt(t, 1024)
	defer belt.Destroy()

	alloc, err := belt.Allocate(0, nil)
	if err != nil {
		t.Fatalf("allocate 0 bytes: %v", err)
	}
	if alloc.Buffer != nil {
		t.Error("expected nil buffer for 0-byte allocation")
	}

	s := belt.Stats()
	active := s.ActiveChunks
	if active != 0 {
		t.Errorf("expected 0 active chunks after 0-byte alloc, got %d", active)
	}
}

func TestStagingBelt_FinishNoWork(t *testing.T) {
	belt := createTestBelt(t, 1024)
	defer belt.Destroy()

	belt.Finish(1)
	s := belt.Stats()
	if s.ClosedSubs != 0 {
		t.Errorf("expected 0 closed submissions from empty finish, got %d", s.ClosedSubs)
	}
}

func TestStagingBelt_ChunkedAllocation(t *testing.T) {
	// Create a belt with MaxStagingBufferSize = 100 bytes.
	// An oversized write of 250 bytes should be split into 3 chunks:
	// 100 + 100 + 50.
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 64, 0, 100) // chunkSize=64, maxStaging=100
	defer belt.Destroy()

	data := make([]byte, 250)
	for i := range data {
		data[i] = byte(i % 256)
	}

	alloc, err := belt.Allocate(250, data)
	if err != nil {
		t.Fatalf("allocate chunked: %v", err)
	}

	// First chunk should be returned as primary allocation.
	if alloc.Buffer == nil {
		t.Fatal("expected non-nil buffer from chunked allocation")
	}
	if alloc.Offset != 0 {
		t.Errorf("expected offset=0 for chunked allocation, got %d", alloc.Offset)
	}

	// Should have 3 chunks in chunkedAllocs.
	if len(belt.chunkedAllocs) != 3 {
		t.Fatalf("expected 3 chunked allocs, got %d", len(belt.chunkedAllocs))
	}

	// Verify chunk sizes.
	expectedSizes := []uint64{100, 100, 50}
	for i, ca := range belt.chunkedAllocs {
		if ca.Size != expectedSizes[i] {
			t.Errorf("chunk %d: expected size %d, got %d", i, expectedSizes[i], ca.Size)
		}
		if ca.Buffer == nil {
			t.Errorf("chunk %d: expected non-nil buffer", i)
		}
	}

	// All chunks should be in oversized (not activeChunks).
	if len(belt.oversized) != 3 {
		t.Errorf("expected 3 oversized buffers, got %d", len(belt.oversized))
	}
	s := belt.Stats()
	if s.ActiveChunks != 0 {
		t.Errorf("expected 0 active chunks, got %d", s.ActiveChunks)
	}
}

func TestStagingBelt_ChunkedSingleChunk(t *testing.T) {
	// When oversized write fits in MaxStagingBufferSize, should NOT chunk.
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 64, 0, 200) // chunkSize=64, maxStaging=200
	defer belt.Destroy()

	data := make([]byte, 150)
	alloc, err := belt.Allocate(150, data)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	if alloc.Buffer == nil {
		t.Fatal("expected non-nil buffer")
	}

	// Should NOT have chunked — single oversized buffer.
	if len(belt.chunkedAllocs) != 0 {
		t.Errorf("expected 0 chunked allocs (not chunked), got %d", len(belt.chunkedAllocs))
	}
	if len(belt.oversized) != 1 {
		t.Errorf("expected 1 oversized buffer, got %d", len(belt.oversized))
	}
}

func TestStagingBelt_ChunkedExactMultiple(t *testing.T) {
	// Write is exact multiple of MaxStagingBufferSize.
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 64, 0, 100) // chunkSize=64, maxStaging=100
	defer belt.Destroy()

	data := make([]byte, 300)
	_, err := belt.Allocate(300, data)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	// Should have 3 chunks of 100 each.
	if len(belt.chunkedAllocs) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(belt.chunkedAllocs))
	}
	for i, ca := range belt.chunkedAllocs {
		if ca.Size != 100 {
			t.Errorf("chunk %d: expected size 100, got %d", i, ca.Size)
		}
	}
}

func TestStagingBelt_MaxStagingBufferSizeDefault(t *testing.T) {
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 0, 0, 0) // all defaults
	defer belt.Destroy()

	if belt.MaxStagingBufferSize != MaxOversizedSize {
		t.Errorf("expected default MaxStagingBufferSize=%d, got %d",
			MaxOversizedSize, belt.MaxStagingBufferSize)
	}
}

func TestStagingBelt_ChunkedDataIntegrity(t *testing.T) {
	// Verify that chunked allocation splits data correctly across multiple
	// staging buffers. This is the core of BUG-VK-001 Fix 2: writes larger
	// than MaxStagingBufferSize must be split, and each chunk must contain
	// the correct slice of the source data.
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 32, 0, 80) // chunkSize=32, maxStaging=80
	defer belt.Destroy()

	// Write 200 bytes of recognizable data (should produce 3 chunks: 80+80+40).
	data := make([]byte, 200)
	for i := range data {
		data[i] = byte(i)
	}

	alloc, err := belt.Allocate(200, data)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if alloc.Buffer == nil {
		t.Fatal("expected non-nil buffer")
	}

	// Verify chunks.
	if len(belt.chunkedAllocs) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(belt.chunkedAllocs))
	}

	wantSizes := []uint64{80, 80, 40}
	wantDstOffsets := []uint64{0, 80, 160} // cumulative offsets for CopyBufferToBuffer
	dstOffset := uint64(0)
	for i, ca := range belt.chunkedAllocs {
		if ca.Size != wantSizes[i] {
			t.Errorf("chunk %d: size=%d, want %d", i, ca.Size, wantSizes[i])
		}
		if dstOffset != wantDstOffsets[i] {
			t.Errorf("chunk %d: dst offset=%d, want %d", i, dstOffset, wantDstOffsets[i])
		}
		dstOffset += ca.Size
	}

	// Total data transferred must equal original size.
	if dstOffset != 200 {
		t.Errorf("total chunked size=%d, want 200", dstOffset)
	}
}

func TestStagingBelt_SmallMaxStagingBufferSize(t *testing.T) {
	// Simulate a device with very small maxMemoryAllocationSize (e.g., 16 bytes).
	// This exercises the extreme case where even normal-sized writes must be chunked.
	dev := newTestDevice()
	q := &testQueue{}
	belt := New(dev, q, 8, 0, 16) // chunkSize=8, maxStaging=16
	defer belt.Destroy()

	// Write 50 bytes. Should produce ceil(50/16)=4 chunks: 16+16+16+2.
	data := make([]byte, 50)
	_, err := belt.Allocate(50, data)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	if len(belt.chunkedAllocs) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(belt.chunkedAllocs))
	}

	wantSizes := []uint64{16, 16, 16, 2}
	for i, ca := range belt.chunkedAllocs {
		if ca.Size != wantSizes[i] {
			t.Errorf("chunk %d: size=%d, want %d", i, ca.Size, wantSizes[i])
		}
	}
}
