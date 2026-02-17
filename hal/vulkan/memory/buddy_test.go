package memory

import (
	"errors"
	"testing"
)

func TestNewBuddyAllocator(t *testing.T) {
	tests := []struct {
		name         string
		totalSize    uint64
		minBlockSize uint64
		wantErr      bool
	}{
		{
			name:         "valid 1MB with 256B min",
			totalSize:    1 << 20, // 1 MB
			minBlockSize: 256,
			wantErr:      false,
		},
		{
			name:         "valid 256MB with 4KB min",
			totalSize:    256 << 20, // 256 MB
			minBlockSize: 4096,
			wantErr:      false,
		},
		{
			name:         "valid equal sizes",
			totalSize:    4096,
			minBlockSize: 4096,
			wantErr:      false,
		},
		{
			name:         "invalid zero total",
			totalSize:    0,
			minBlockSize: 256,
			wantErr:      true,
		},
		{
			name:         "invalid zero min",
			totalSize:    1 << 20,
			minBlockSize: 0,
			wantErr:      true,
		},
		{
			name:         "invalid non-power-of-2 total",
			totalSize:    1000,
			minBlockSize: 256,
			wantErr:      true,
		},
		{
			name:         "invalid non-power-of-2 min",
			totalSize:    1 << 20,
			minBlockSize: 300,
			wantErr:      true,
		},
		{
			name:         "invalid min > total",
			totalSize:    256,
			minBlockSize: 4096,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewBuddyAllocator(tt.totalSize, tt.minBlockSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBuddyAllocator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && b == nil {
				t.Error("NewBuddyAllocator() returned nil allocator without error")
			}
		})
	}
}

func TestBuddyAlloc(t *testing.T) {
	b, err := NewBuddyAllocator(1<<20, 256) // 1MB, 256B min
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	tests := []struct {
		name     string
		size     uint64
		wantSize uint64 // Expected allocated size (rounded up)
		wantErr  error
	}{
		{"min size", 1, 256, nil},
		{"exact min", 256, 256, nil},
		{"between powers", 300, 512, nil},
		{"exact power", 512, 512, nil},
		{"1KB", 1024, 1024, nil},
		{"zero size", 0, 0, ErrInvalidSize},
		{"too large", 2 << 20, 0, ErrInvalidSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := b.Alloc(tt.size)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Alloc(%d) error = %v, wantErr %v", tt.size, err, tt.wantErr)
				return
			}
			if err == nil {
				if block.Size != tt.wantSize {
					t.Errorf("Alloc(%d) size = %d, want %d", tt.size, block.Size, tt.wantSize)
				}
				// Clean up
				if err := b.Free(block); err != nil {
					t.Errorf("Free failed: %v", err)
				}
			}
		})
	}
}

func TestBuddyAllocMultiple(t *testing.T) {
	b, err := NewBuddyAllocator(1<<20, 256) // 1MB total
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	// Allocate multiple blocks
	blocks := make([]BuddyBlock, 0)
	for i := 0; i < 100; i++ {
		block, err := b.Alloc(1024) // 1KB each
		if err != nil {
			t.Fatalf("Alloc %d failed: %v", i, err)
		}
		blocks = append(blocks, block)
	}

	// Verify stats
	stats := b.Stats()
	if stats.AllocationCount != 100 {
		t.Errorf("AllocationCount = %d, want 100", stats.AllocationCount)
	}
	if stats.AllocatedSize != 100*1024 {
		t.Errorf("AllocatedSize = %d, want %d", stats.AllocatedSize, 100*1024)
	}

	// Free all blocks
	for _, block := range blocks {
		if err := b.Free(block); err != nil {
			t.Errorf("Free failed: %v", err)
		}
	}

	// Verify all freed
	stats = b.Stats()
	if stats.AllocationCount != 0 {
		t.Errorf("AllocationCount after free = %d, want 0", stats.AllocationCount)
	}
	if stats.AllocatedSize != 0 {
		t.Errorf("AllocatedSize after free = %d, want 0", stats.AllocatedSize)
	}
}

func TestBuddyAllocUntilFull(t *testing.T) {
	b, err := NewBuddyAllocator(4096, 256) // 4KB total, 256B min = 16 blocks max
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	blocks := make([]BuddyBlock, 0)

	// Allocate until full
	for {
		block, err := b.Alloc(256)
		if errors.Is(err, ErrOutOfMemory) {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		blocks = append(blocks, block)
	}

	// Should have allocated 16 blocks
	if len(blocks) != 16 {
		t.Errorf("Allocated %d blocks, want 16", len(blocks))
	}

	// Free one block
	if err := b.Free(blocks[0]); err != nil {
		t.Fatalf("Free failed: %v", err)
	}
	blocks = blocks[1:]

	// Should be able to allocate again
	block, err := b.Alloc(256)
	if err != nil {
		t.Errorf("Alloc after free failed: %v", err)
	} else {
		blocks = append(blocks, block)
	}

	// Clean up
	for _, blk := range blocks {
		_ = b.Free(blk)
	}
}

func TestBuddyFree(t *testing.T) {
	b, err := NewBuddyAllocator(1<<20, 256)
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	block, err := b.Alloc(1024)
	if err != nil {
		t.Fatalf("Alloc failed: %v", err)
	}

	// Free the block
	if err := b.Free(block); err != nil {
		t.Errorf("Free() error = %v", err)
	}

	// Double free should fail
	if err := b.Free(block); !errors.Is(err, ErrDoubleFree) {
		t.Errorf("Double Free() error = %v, want ErrDoubleFree", err)
	}
}

func TestBuddyMerging(t *testing.T) {
	b, err := NewBuddyAllocator(4096, 256) // 4KB total
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	// Allocate two adjacent 2KB blocks (fills the entire space)
	block1, err := b.Alloc(2048)
	if err != nil {
		t.Fatalf("Alloc 1 failed: %v", err)
	}
	block2, err := b.Alloc(2048)
	if err != nil {
		t.Fatalf("Alloc 2 failed: %v", err)
	}

	// Should be full now
	_, err = b.Alloc(256)
	if !errors.Is(err, ErrOutOfMemory) {
		t.Errorf("Expected ErrOutOfMemory, got %v", err)
	}

	// Free both blocks
	if err := b.Free(block1); err != nil {
		t.Fatalf("Free 1 failed: %v", err)
	}
	if err := b.Free(block2); err != nil {
		t.Fatalf("Free 2 failed: %v", err)
	}

	// Now should be able to allocate full 4KB
	bigBlock, err := b.Alloc(4096)
	if err != nil {
		t.Errorf("Alloc full block failed: %v", err)
	}
	if bigBlock.Size != 4096 {
		t.Errorf("Big block size = %d, want 4096", bigBlock.Size)
	}

	// Verify merge happened
	stats := b.Stats()
	if stats.MergeCount == 0 {
		t.Error("Expected merges to occur")
	}
}

func TestBuddyReset(t *testing.T) {
	b, err := NewBuddyAllocator(1<<20, 256)
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	// Allocate some blocks
	for i := 0; i < 10; i++ {
		_, err := b.Alloc(1024)
		if err != nil {
			t.Fatalf("Alloc failed: %v", err)
		}
	}

	stats := b.Stats()
	if stats.AllocationCount != 10 {
		t.Errorf("AllocationCount = %d, want 10", stats.AllocationCount)
	}

	// Reset
	b.Reset()

	// Stats should be cleared
	stats = b.Stats()
	if stats.AllocationCount != 0 {
		t.Errorf("AllocationCount after reset = %d, want 0", stats.AllocationCount)
	}
	if stats.AllocatedSize != 0 {
		t.Errorf("AllocatedSize after reset = %d, want 0", stats.AllocatedSize)
	}

	// Should be able to allocate full size again
	block, err := b.Alloc(1 << 20)
	if err != nil {
		t.Errorf("Alloc full size after reset failed: %v", err)
	}
	if block.Size != 1<<20 {
		t.Errorf("Block size = %d, want %d", block.Size, 1<<20)
	}
}

func TestBuddyStats(t *testing.T) {
	b, err := NewBuddyAllocator(1<<20, 256)
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	// Initial stats
	stats := b.Stats()
	if stats.TotalSize != 1<<20 {
		t.Errorf("TotalSize = %d, want %d", stats.TotalSize, 1<<20)
	}
	if stats.AllocatedSize != 0 {
		t.Errorf("Initial AllocatedSize = %d, want 0", stats.AllocatedSize)
	}

	// Allocate and check
	block1, _ := b.Alloc(4096)
	block2, _ := b.Alloc(8192)

	stats = b.Stats()
	if stats.AllocatedSize != 4096+8192 {
		t.Errorf("AllocatedSize = %d, want %d", stats.AllocatedSize, 4096+8192)
	}
	if stats.AllocationCount != 2 {
		t.Errorf("AllocationCount = %d, want 2", stats.AllocationCount)
	}
	if stats.TotalAllocated != 4096+8192 {
		t.Errorf("TotalAllocated = %d, want %d", stats.TotalAllocated, 4096+8192)
	}

	// Free and check
	_ = b.Free(block1)
	stats = b.Stats()
	if stats.AllocatedSize != 8192 {
		t.Errorf("AllocatedSize after free = %d, want 8192", stats.AllocatedSize)
	}
	if stats.TotalFreed != 4096 {
		t.Errorf("TotalFreed = %d, want 4096", stats.TotalFreed)
	}

	_ = b.Free(block2)
}

func TestBuddyAllocAlignment(t *testing.T) {
	b, err := NewBuddyAllocator(1<<20, 256)
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	// All allocations should be aligned to their size
	sizes := []uint64{256, 512, 1024, 2048, 4096, 8192}
	for _, size := range sizes {
		block, err := b.Alloc(size)
		if err != nil {
			t.Fatalf("Alloc(%d) failed: %v", size, err)
		}

		// Offset should be aligned to block size
		if block.Offset%block.Size != 0 {
			t.Errorf("Block offset %d not aligned to size %d", block.Offset, block.Size)
		}

		_ = b.Free(block)
	}
}

func TestBuddyNoOverlap(t *testing.T) {
	b, err := NewBuddyAllocator(1<<16, 256) // 64KB
	if err != nil {
		t.Fatalf("NewBuddyAllocator failed: %v", err)
	}

	// Allocate many blocks
	blocks := make([]BuddyBlock, 0)
	for i := 0; i < 50; i++ {
		block, err := b.Alloc(1024)
		if errors.Is(err, ErrOutOfMemory) {
			break
		}
		if err != nil {
			t.Fatalf("Alloc failed: %v", err)
		}
		blocks = append(blocks, block)
	}

	// Check no overlaps
	for i := 0; i < len(blocks); i++ {
		for j := i + 1; j < len(blocks); j++ {
			a := blocks[i]
			bb := blocks[j]

			aEnd := a.Offset + a.Size
			bEnd := bb.Offset + bb.Size

			// Check if ranges overlap
			if a.Offset < bEnd && bb.Offset < aEnd {
				t.Errorf("Blocks overlap: [%d-%d) and [%d-%d)",
					a.Offset, aEnd, bb.Offset, bEnd)
			}
		}
	}

	// Clean up
	for _, blk := range blocks {
		_ = b.Free(blk)
	}
}

// Benchmarks

func BenchmarkBuddyAlloc(b *testing.B) {
	allocator, err := NewBuddyAllocator(256<<20, 256) // 256MB
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block, err := allocator.Alloc(4096)
		if err != nil {
			allocator.Reset()
			block, _ = allocator.Alloc(4096)
		}
		_ = allocator.Free(block)
	}
}

func BenchmarkBuddyAllocFree(b *testing.B) {
	allocator, err := NewBuddyAllocator(256<<20, 256) // 256MB
	if err != nil {
		b.Fatal(err)
	}

	blocks := make([]BuddyBlock, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Allocate batch
		for j := 0; j < 1000; j++ {
			blocks[j], _ = allocator.Alloc(4096)
		}
		// Free batch
		for j := 0; j < 1000; j++ {
			_ = allocator.Free(blocks[j])
		}
	}
}

// BenchmarkBuddyAllocParallel measures concurrent allocation throughput.
// GPU memory allocation can be called from multiple goroutines in real workloads
// (e.g., texture loading, buffer creation during scene setup).
func BenchmarkBuddyAllocParallel(b *testing.B) {
	b.ReportAllocs()
	allocator, err := NewBuddyAllocator(256<<20, 256) // 256MB
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			block, err := allocator.Alloc(4096)
			if err != nil {
				// Allocator is full, reset and retry.
				// In parallel benchmarks, contention may cause OOM.
				continue
			}
			_ = allocator.Free(block)
		}
	})
}

// BenchmarkBuddyAllocVariedSizes measures allocation with varied sizes,
// simulating real GPU workloads that mix uniform buffers (256B),
// vertex buffers (4KB-64KB), and textures (1MB+).
func BenchmarkBuddyAllocVariedSizes(b *testing.B) {
	b.ReportAllocs()
	allocator, err := NewBuddyAllocator(256<<20, 256) // 256MB
	if err != nil {
		b.Fatal(err)
	}

	sizes := []uint64{256, 1024, 4096, 16384, 65536, 262144}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		size := sizes[i%len(sizes)]
		block, err := allocator.Alloc(size)
		if err != nil {
			allocator.Reset()
			block, _ = allocator.Alloc(size)
		}
		_ = allocator.Free(block)
	}
}

// BenchmarkBuddyFragmentation measures allocation under fragmentation pressure.
// Allocates many blocks, frees every other one (creating fragmentation),
// then tries to allocate into the gaps. This simulates real GPU resource churn
// where buffers/textures are created and destroyed in non-sequential order.
func BenchmarkBuddyFragmentation(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		allocator, _ := NewBuddyAllocator(1<<20, 256) // 1MB

		// Phase 1: Fill with small blocks
		blocks := make([]BuddyBlock, 0, 256)
		for {
			block, err := allocator.Alloc(4096)
			if err != nil {
				break
			}
			blocks = append(blocks, block)
		}

		// Phase 2: Free every other block (create fragmentation)
		for j := 0; j < len(blocks); j += 2 {
			_ = allocator.Free(blocks[j])
		}

		b.StartTimer()

		// Phase 3: Re-allocate into gaps (measure fragmented allocation cost)
		for j := 0; j < len(blocks)/2; j++ {
			block, err := allocator.Alloc(4096)
			if err != nil {
				break
			}
			_ = allocator.Free(block)
		}
	}
}

// BenchmarkBuddyAllocSizes measures allocation speed across different block sizes.
func BenchmarkBuddyAllocSizes(b *testing.B) {
	sizes := []struct {
		name string
		size uint64
	}{
		{"256B", 256},
		{"1KB", 1024},
		{"4KB", 4096},
		{"64KB", 65536},
		{"1MB", 1 << 20},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			allocator, err := NewBuddyAllocator(256<<20, 256)
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				block, err := allocator.Alloc(s.size)
				if err != nil {
					allocator.Reset()
					block, _ = allocator.Alloc(s.size)
				}
				_ = allocator.Free(block)
			}
		})
	}
}

// Helper tests

func TestIsPowerOfTwo(t *testing.T) {
	tests := []struct {
		n    uint64
		want bool
	}{
		{0, false},
		{1, true},
		{2, true},
		{3, false},
		{4, true},
		{5, false},
		{256, true},
		{1000, false},
		{1 << 20, true},
	}

	for _, tt := range tests {
		if got := isPowerOfTwo(tt.n); got != tt.want {
			t.Errorf("isPowerOfTwo(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		n    uint64
		want uint64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{100, 128},
		{256, 256},
		{257, 512},
	}

	for _, tt := range tests {
		if got := nextPowerOfTwo(tt.n); got != tt.want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestLog2(t *testing.T) {
	tests := []struct {
		n    uint64
		want int
	}{
		{1, 0},
		{2, 1},
		{4, 2},
		{8, 3},
		{16, 4},
		{256, 8},
		{1024, 10},
	}

	for _, tt := range tests {
		if got := log2(tt.n); got != tt.want {
			t.Errorf("log2(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}
