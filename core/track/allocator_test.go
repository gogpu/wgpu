package track

import (
	"sync"
	"testing"
)

func TestTrackerIndex_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		index TrackerIndex
		want  bool
	}{
		{"zero is valid", TrackerIndex(0), true},
		{"positive is valid", TrackerIndex(100), true},
		{"max-1 is valid", TrackerIndex(^uint32(0) - 1), true},
		{"invalid index", InvalidTrackerIndex, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.index.IsValid(); got != tt.want {
				t.Errorf("TrackerIndex(%d).IsValid() = %v, want %v", tt.index, got, tt.want)
			}
		})
	}
}

func TestTrackerIndexAllocator_Alloc(t *testing.T) {
	a := NewTrackerIndexAllocator()

	// First allocation should return 0
	idx0 := a.Alloc()
	if idx0 != 0 {
		t.Errorf("First alloc returned %d, want 0", idx0)
	}

	// Subsequent allocations should be sequential
	idx1 := a.Alloc()
	if idx1 != 1 {
		t.Errorf("Second alloc returned %d, want 1", idx1)
	}

	idx2 := a.Alloc()
	if idx2 != 2 {
		t.Errorf("Third alloc returned %d, want 2", idx2)
	}
}

func TestTrackerIndexAllocator_Free(t *testing.T) {
	a := NewTrackerIndexAllocator()

	idx0 := a.Alloc()
	idx1 := a.Alloc()
	idx2 := a.Alloc()

	// Free middle index
	a.Free(idx1)

	// Size should decrease
	if a.Size() != 2 {
		t.Errorf("Size after free = %d, want 2", a.Size())
	}

	// Free is idempotent for InvalidTrackerIndex
	a.Free(InvalidTrackerIndex) // Should not panic

	_ = idx0
	_ = idx2
}

func TestTrackerIndexAllocator_Reuse(t *testing.T) {
	a := NewTrackerIndexAllocator()

	// Allocate 3 indices
	idx0 := a.Alloc()
	idx1 := a.Alloc()
	idx2 := a.Alloc()

	// Free in reverse order
	a.Free(idx2)
	a.Free(idx1)
	a.Free(idx0)

	// Reallocations should reuse freed indices (LIFO)
	realloc0 := a.Alloc()
	if realloc0 != idx0 {
		t.Errorf("First realloc = %d, want %d (reuse)", realloc0, idx0)
	}

	realloc1 := a.Alloc()
	if realloc1 != idx1 {
		t.Errorf("Second realloc = %d, want %d (reuse)", realloc1, idx1)
	}

	realloc2 := a.Alloc()
	if realloc2 != idx2 {
		t.Errorf("Third realloc = %d, want %d (reuse)", realloc2, idx2)
	}

	// Next allocation should be fresh
	fresh := a.Alloc()
	if fresh != 3 {
		t.Errorf("Fresh alloc = %d, want 3", fresh)
	}
}

func TestTrackerIndexAllocator_Size(t *testing.T) {
	a := NewTrackerIndexAllocator()

	if a.Size() != 0 {
		t.Errorf("Initial size = %d, want 0", a.Size())
	}

	a.Alloc()
	if a.Size() != 1 {
		t.Errorf("Size after 1 alloc = %d, want 1", a.Size())
	}

	a.Alloc()
	a.Alloc()
	if a.Size() != 3 {
		t.Errorf("Size after 3 allocs = %d, want 3", a.Size())
	}

	a.Free(TrackerIndex(1))
	if a.Size() != 2 {
		t.Errorf("Size after 1 free = %d, want 2", a.Size())
	}
}

func TestTrackerIndexAllocator_HighWaterMark(t *testing.T) {
	a := NewTrackerIndexAllocator()

	// Empty allocator should return invalid
	if a.HighWaterMark() != InvalidTrackerIndex {
		t.Errorf("Empty HWM = %d, want InvalidTrackerIndex", a.HighWaterMark())
	}

	a.Alloc() // 0
	if a.HighWaterMark() != 0 {
		t.Errorf("HWM after 1 alloc = %d, want 0", a.HighWaterMark())
	}

	a.Alloc() // 1
	a.Alloc() // 2
	if a.HighWaterMark() != 2 {
		t.Errorf("HWM after 3 allocs = %d, want 2", a.HighWaterMark())
	}

	// HWM doesn't decrease on free
	a.Free(TrackerIndex(1))
	if a.HighWaterMark() != 2 {
		t.Errorf("HWM after free = %d, want 2 (unchanged)", a.HighWaterMark())
	}
}

func TestTrackerIndexAllocator_Reset(t *testing.T) {
	a := NewTrackerIndexAllocator()

	a.Alloc()
	a.Alloc()
	a.Alloc()
	a.Free(TrackerIndex(1))

	a.Reset()

	if a.Size() != 0 {
		t.Errorf("Size after reset = %d, want 0", a.Size())
	}

	// Next allocation should start from 0
	idx := a.Alloc()
	if idx != 0 {
		t.Errorf("First alloc after reset = %d, want 0", idx)
	}
}

func TestTrackerIndexAllocator_Concurrent(t *testing.T) {
	a := NewTrackerIndexAllocator()
	const goroutines = 100
	const allocsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent allocations
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < allocsPerGoroutine; j++ {
				idx := a.Alloc()
				// Some goroutines free immediately
				if j%3 == 0 {
					a.Free(idx)
				}
			}
		}()
	}

	wg.Wait()

	// Verify no panic and reasonable state
	size := a.Size()
	if size < 0 || size > goroutines*allocsPerGoroutine {
		t.Errorf("Final size %d is out of expected range", size)
	}
}

func TestSharedTrackerIndexAllocator(t *testing.T) {
	s := NewSharedTrackerIndexAllocator()

	idx0 := s.Alloc()
	if idx0 != 0 {
		t.Errorf("First alloc = %d, want 0", idx0)
	}

	idx1 := s.Alloc()
	if idx1 != 1 {
		t.Errorf("Second alloc = %d, want 1", idx1)
	}

	if s.Size() != 2 {
		t.Errorf("Size = %d, want 2", s.Size())
	}

	s.Free(idx0)
	if s.Size() != 1 {
		t.Errorf("Size after free = %d, want 1", s.Size())
	}

	if s.HighWaterMark() != 1 {
		t.Errorf("HWM = %d, want 1", s.HighWaterMark())
	}

	// Reuse
	realloc := s.Alloc()
	if realloc != idx0 {
		t.Errorf("Realloc = %d, want %d", realloc, idx0)
	}
}

func TestTrackerIndexAllocators(t *testing.T) {
	allocs := NewTrackerIndexAllocators()

	// Each allocator should be independent
	bufIdx := allocs.Buffers.Alloc()
	texIdx := allocs.Textures.Alloc()
	viewIdx := allocs.TextureViews.Alloc()

	// All should return 0 (independent allocators)
	if bufIdx != 0 {
		t.Errorf("Buffer index = %d, want 0", bufIdx)
	}
	if texIdx != 0 {
		t.Errorf("Texture index = %d, want 0", texIdx)
	}
	if viewIdx != 0 {
		t.Errorf("TextureView index = %d, want 0", viewIdx)
	}

	// Allocate more in buffers
	allocs.Buffers.Alloc()
	allocs.Buffers.Alloc()

	if allocs.Buffers.Size() != 3 {
		t.Errorf("Buffers size = %d, want 3", allocs.Buffers.Size())
	}
	if allocs.Textures.Size() != 1 {
		t.Errorf("Textures size = %d, want 1", allocs.Textures.Size())
	}

	// Verify all allocators exist
	if allocs.Samplers == nil {
		t.Error("Samplers allocator is nil")
	}
	if allocs.BindGroups == nil {
		t.Error("BindGroups allocator is nil")
	}
	if allocs.BindGroupLayouts == nil {
		t.Error("BindGroupLayouts allocator is nil")
	}
	if allocs.RenderPipelines == nil {
		t.Error("RenderPipelines allocator is nil")
	}
	if allocs.ComputePipelines == nil {
		t.Error("ComputePipelines allocator is nil")
	}
}

func TestTrackingData_Lifecycle(t *testing.T) {
	alloc := NewSharedTrackerIndexAllocator()

	// Create tracking data
	td := NewTrackingData(alloc)
	if td.Index() != 0 {
		t.Errorf("First tracking data index = %d, want 0", td.Index())
	}
	if td.IsReleased() {
		t.Error("New tracking data should not be released")
	}

	// Create another
	td2 := NewTrackingData(alloc)
	if td2.Index() != 1 {
		t.Errorf("Second tracking data index = %d, want 1", td2.Index())
	}

	if alloc.Size() != 2 {
		t.Errorf("Allocator size = %d, want 2", alloc.Size())
	}

	// Release first
	td.Release()
	if !td.IsReleased() {
		t.Error("Tracking data should be released after Release()")
	}
	if alloc.Size() != 1 {
		t.Errorf("Allocator size after release = %d, want 1", alloc.Size())
	}

	// Double release should be safe
	td.Release() // Should not panic or double-free

	// New allocation should reuse the freed index
	td3 := NewTrackingData(alloc)
	if td3.Index() != 0 {
		t.Errorf("Third tracking data index = %d, want 0 (reused)", td3.Index())
	}
}

func TestTrackingData_NilAllocator(t *testing.T) {
	td := NewTrackingData(nil)

	if td.Index() != InvalidTrackerIndex {
		t.Errorf("Nil allocator index = %d, want InvalidTrackerIndex", td.Index())
	}

	// Release should be safe with nil allocator
	td.Release() // Should not panic
}

func TestTrackingData_Concurrent(t *testing.T) {
	alloc := NewSharedTrackerIndexAllocator()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	trackingDatas := make([]*TrackingData, goroutines)

	// Create tracking data concurrently
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			trackingDatas[i] = NewTrackingData(alloc)
		}()
	}
	wg.Wait()

	// Verify all got valid indices
	seen := make(map[TrackerIndex]bool)
	for i, td := range trackingDatas {
		if td == nil {
			t.Errorf("Tracking data %d is nil", i)
			continue
		}
		idx := td.Index()
		if !idx.IsValid() {
			t.Errorf("Tracking data %d has invalid index", i)
		}
		if seen[idx] {
			t.Errorf("Duplicate index %d", idx)
		}
		seen[idx] = true
	}

	if alloc.Size() != goroutines {
		t.Errorf("Allocator size = %d, want %d", alloc.Size(), goroutines)
	}

	// Release all concurrently
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			trackingDatas[i].Release()
		}()
	}
	wg.Wait()

	if alloc.Size() != 0 {
		t.Errorf("Allocator size after release = %d, want 0", alloc.Size())
	}
}

func BenchmarkTrackerIndexAllocator_Alloc(b *testing.B) {
	a := NewTrackerIndexAllocator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Alloc()
	}
}

func BenchmarkTrackerIndexAllocator_AllocFree(b *testing.B) {
	a := NewTrackerIndexAllocator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := a.Alloc()
		a.Free(idx)
	}
}

func BenchmarkTrackerIndexAllocator_Concurrent(b *testing.B) {
	a := NewTrackerIndexAllocator()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := a.Alloc()
			a.Free(idx)
		}
	})
}
