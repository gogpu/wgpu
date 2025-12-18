//go:build software

package raster

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// WorkerPool Tests
// =============================================================================

func TestWorkerPoolCreation(t *testing.T) {
	pool := NewWorkerPool(4)

	if pool.Workers() != 4 {
		t.Errorf("Workers() = %d, want 4", pool.Workers())
	}

	pool.Close()
}

func TestWorkerPoolDefaultWorkers(t *testing.T) {
	pool := NewWorkerPool(0) // Should default to NumCPU

	if pool.Workers() != runtime.NumCPU() {
		t.Errorf("Workers() = %d, want %d", pool.Workers(), runtime.NumCPU())
	}

	pool.Close()
}

func TestWorkerPoolSubmitAndWait(t *testing.T) {
	pool := NewWorkerPool(4)
	pool.Start()

	var counter int32
	const numTasks = 100

	for i := 0; i < numTasks; i++ {
		pool.Submit(func() {
			atomic.AddInt32(&counter, 1)
		})
	}

	pool.Wait()

	if counter != numTasks {
		t.Errorf("Counter = %d, want %d", counter, numTasks)
	}

	pool.Close()
}

func TestWorkerPoolMultipleWaits(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()

	var counter int32

	// First batch
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			atomic.AddInt32(&counter, 1)
		})
	}
	pool.Wait()

	if counter != 10 {
		t.Errorf("After first batch: counter = %d, want 10", counter)
	}

	// Second batch
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			atomic.AddInt32(&counter, 1)
		})
	}
	pool.Wait()

	if counter != 20 {
		t.Errorf("After second batch: counter = %d, want 20", counter)
	}

	pool.Close()
}

func TestWorkerPoolStartTwice(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()
	pool.Start() // Should be a no-op

	// Verify pool still works correctly
	var counter int32
	pool.Submit(func() {
		atomic.AddInt32(&counter, 1)
	})
	pool.Wait()

	if counter != 1 {
		t.Errorf("Counter = %d, want 1", counter)
	}

	pool.Close()
}

// =============================================================================
// ParallelConfig Tests
// =============================================================================

func TestDefaultParallelConfig(t *testing.T) {
	config := DefaultParallelConfig()

	if config.Workers != runtime.NumCPU() {
		t.Errorf("Workers = %d, want %d", config.Workers, runtime.NumCPU())
	}
	if config.TileSize != TileSize {
		t.Errorf("TileSize = %d, want %d", config.TileSize, TileSize)
	}
	if config.MinTriangles <= 0 {
		t.Error("MinTriangles should be positive")
	}
}

// =============================================================================
// ParallelRasterizer Tests
// =============================================================================

func TestParallelRasterizerCreation(t *testing.T) {
	config := ParallelConfig{
		Workers:      4,
		TileSize:     8,
		MinTriangles: 10,
	}

	pr := NewParallelRasterizer(800, 600, config)
	defer pr.Close()

	if pr.Config().Workers != 4 {
		t.Errorf("Config().Workers = %d, want 4", pr.Config().Workers)
	}
	if pr.Grid() == nil {
		t.Error("Grid() should not be nil")
	}
}

func TestParallelRasterizerResize(t *testing.T) {
	pr := NewParallelRasterizer(100, 100, DefaultParallelConfig())
	defer pr.Close()

	initialTiles := pr.Grid().TileCount()

	pr.Resize(200, 200)

	newTiles := pr.Grid().TileCount()
	if newTiles <= initialTiles {
		t.Errorf("After resize: tiles = %d, should be > %d", newTiles, initialTiles)
	}
}

func TestParallelRasterizerRasterize(t *testing.T) {
	pr := NewParallelRasterizer(100, 100, ParallelConfig{
		Workers:      4,
		TileSize:     8,
		MinTriangles: 5,
	})
	defer pr.Close()

	// Create some triangles
	triangles := make([]Triangle, 20)
	for i := range triangles {
		x := float32(i % 10 * 10)
		y := float32(i / 10 * 50)
		triangles[i] = CreateScreenTriangle(
			x, y, 0.5,
			x+8, y, 0.5,
			x+4, y+8, 0.5,
		)
	}

	var mu sync.Mutex
	tilesProcessed := make(map[int]bool)

	pr.RasterizeParallel(triangles, func(tile Tile, tileTriangles []Triangle) {
		mu.Lock()
		tilesProcessed[pr.Grid().TileIndex(tile.X, tile.Y)] = true
		mu.Unlock()
	})

	if len(tilesProcessed) == 0 {
		t.Error("No tiles were processed")
	}
}

func TestParallelRasterizerSmallBatch(t *testing.T) {
	pr := NewParallelRasterizer(100, 100, ParallelConfig{
		Workers:      4,
		TileSize:     8,
		MinTriangles: 100, // High threshold for testing fallback
	})
	defer pr.Close()

	// Create fewer triangles than threshold
	triangles := make([]Triangle, 5)
	for i := range triangles {
		triangles[i] = CreateScreenTriangle(
			10, 10, 0.5,
			20, 10, 0.5,
			15, 20, 0.5,
		)
	}

	var callCount int32
	pr.RasterizeParallel(triangles, func(tile Tile, tileTriangles []Triangle) {
		atomic.AddInt32(&callCount, 1)
	})

	// Should still process even with small batch
	if callCount == 0 {
		t.Error("Callback should have been called for small batch")
	}
}

// =============================================================================
// BinTrianglesToTiles Tests
// =============================================================================

func TestBinTrianglesToTiles(t *testing.T) {
	grid := NewTileGrid(32, 32) // 4x4 tiles

	// Single triangle in one tile
	triangles := []Triangle{
		CreateScreenTriangle(2, 2, 0.5, 6, 2, 0.5, 4, 6, 0.5),
	}

	bins := BinTrianglesToTiles(triangles, grid)

	if len(bins) != 1 {
		t.Errorf("Expected 1 bin, got %d", len(bins))
	}

	// Check the triangle is in tile (0, 0)
	idx := grid.TileIndex(0, 0)
	if _, ok := bins[idx]; !ok {
		t.Error("Triangle should be in tile (0, 0)")
	}
}

func TestBinTrianglesToTilesMultipleTiles(t *testing.T) {
	grid := NewTileGrid(32, 32)

	// Triangle spanning multiple tiles
	triangles := []Triangle{
		CreateScreenTriangle(4, 4, 0.5, 20, 4, 0.5, 12, 20, 0.5),
	}

	bins := BinTrianglesToTiles(triangles, grid)

	// Triangle should be in multiple bins
	if len(bins) < 2 {
		t.Errorf("Triangle spans multiple tiles, expected >1 bin, got %d", len(bins))
	}
}

func TestBinTrianglesToTilesWithTest(t *testing.T) {
	grid := NewTileGrid(32, 32)

	// Triangle in specific location
	triangles := []Triangle{
		CreateScreenTriangle(10, 10, 0.5, 14, 10, 0.5, 12, 14, 0.5),
	}

	bins := BinTrianglesToTilesWithTest(triangles, grid)

	// Should have at least one bin
	if len(bins) == 0 {
		t.Error("Expected at least one bin")
	}
}

// =============================================================================
// Fragment Pool Tests
// =============================================================================

func TestFragmentPool(t *testing.T) {
	// Get a slice from the pool
	slice := GetFragmentSlice()
	if slice == nil {
		t.Fatal("GetFragmentSlice() returned nil")
	}

	// Add some fragments
	*slice = append(*slice, Fragment{X: 1, Y: 1}, Fragment{X: 2, Y: 2})

	if len(*slice) != 2 {
		t.Errorf("Slice length = %d, want 2", len(*slice))
	}

	// Return to pool
	PutFragmentSlice(slice)

	// Get another slice - should be reset
	slice2 := GetFragmentSlice()
	if len(*slice2) != 0 {
		t.Errorf("Recycled slice should be empty, got length %d", len(*slice2))
	}

	PutFragmentSlice(slice2)
}

// =============================================================================
// ParallelForEachTile Tests
// =============================================================================

func TestParallelForEachTile(t *testing.T) {
	pr := NewParallelRasterizer(32, 32, ParallelConfig{
		Workers:      4,
		TileSize:     8,
		MinTriangles: 10,
	})
	defer pr.Close()

	var counter int32
	pr.ParallelForEachTile(func(tile Tile) {
		atomic.AddInt32(&counter, 1)
	})

	// 32/8 = 4 tiles per dimension = 16 total
	if counter != 16 {
		t.Errorf("Processed %d tiles, want 16", counter)
	}
}

// =============================================================================
// Race Condition Tests
// =============================================================================

func TestWorkerPoolRaceConditions(t *testing.T) {
	pool := NewWorkerPool(4)
	pool.Start()

	var counter int64
	const numTasks = 1000

	for i := 0; i < numTasks; i++ {
		pool.Submit(func() {
			atomic.AddInt64(&counter, 1)
		})
	}

	pool.Wait()

	if counter != numTasks {
		t.Errorf("Counter = %d, want %d (race condition?)", counter, numTasks)
	}

	pool.Close()
}

func TestParallelRasterizerRaceConditions(t *testing.T) {
	pr := NewParallelRasterizer(100, 100, ParallelConfig{
		Workers:      8,
		TileSize:     8,
		MinTriangles: 5,
	})
	defer pr.Close()

	// Create triangles that span multiple tiles
	triangles := make([]Triangle, 50)
	for i := range triangles {
		x := float32(i % 10 * 10)
		y := float32(i / 10 * 20)
		triangles[i] = CreateScreenTriangle(
			x, y, 0.5,
			x+15, y, 0.5,
			x+7, y+15, 0.5,
		)
	}

	var counter int64
	pr.RasterizeParallel(triangles, func(tile Tile, tileTriangles []Triangle) {
		atomic.AddInt64(&counter, int64(len(tileTriangles)))
	})

	// Counter should be at least the number of triangles
	// (might be more if triangles span multiple tiles)
	if counter < int64(len(triangles)) {
		t.Errorf("Processed fewer triangle-tile pairs than triangles: %d < %d",
			counter, len(triangles))
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkWorkerPoolSubmit(b *testing.B) {
	pool := NewWorkerPool(runtime.NumCPU())
	pool.Start()
	defer pool.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Submit(func() {
			// Empty task
		})
	}
	pool.Wait()
}

func BenchmarkBinTrianglesToTiles(b *testing.B) {
	grid := NewTileGrid(800, 600)
	triangles := make([]Triangle, 1000)
	for i := range triangles {
		x := float32(i % 100 * 8)
		y := float32(i / 100 * 60)
		triangles[i] = CreateScreenTriangle(
			x, y, 0.5,
			x+50, y, 0.5,
			x+25, y+40, 0.5,
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BinTrianglesToTiles(triangles, grid)
	}
}

func BenchmarkBinTrianglesToTilesWithTest(b *testing.B) {
	grid := NewTileGrid(800, 600)
	triangles := make([]Triangle, 1000)
	for i := range triangles {
		x := float32(i % 100 * 8)
		y := float32(i / 100 * 60)
		triangles[i] = CreateScreenTriangle(
			x, y, 0.5,
			x+50, y, 0.5,
			x+25, y+40, 0.5,
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BinTrianglesToTilesWithTest(triangles, grid)
	}
}

func BenchmarkParallelRasterize(b *testing.B) {
	pr := NewParallelRasterizer(800, 600, ParallelConfig{
		Workers:      runtime.NumCPU(),
		TileSize:     8,
		MinTriangles: 10,
	})
	defer pr.Close()

	triangles := make([]Triangle, 1000)
	for i := range triangles {
		x := float32(i % 100 * 8)
		y := float32(i / 100 * 60)
		triangles[i] = CreateScreenTriangle(
			x, y, 0.5,
			x+50, y, 0.5,
			x+25, y+40, 0.5,
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr.RasterizeParallel(triangles, func(tile Tile, tileTriangles []Triangle) {
			// Simulate some work
			for range tileTriangles {
				_ = tile.Width() * tile.Height()
			}
		})
	}
}

func BenchmarkFragmentPool(b *testing.B) {
	b.Run("WithPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			slice := GetFragmentSlice()
			*slice = append(*slice, Fragment{X: 1, Y: 1})
			PutFragmentSlice(slice)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			slice := make([]Fragment, 0, 64)
			slice = append(slice, Fragment{X: 1, Y: 1})
			_ = slice
		}
	})
}

// =============================================================================
// Timeout Tests
// =============================================================================

func TestWorkerPoolTaskTimeout(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()
	defer pool.Close()

	done := make(chan bool, 1)

	pool.Submit(func() {
		time.Sleep(10 * time.Millisecond)
	})
	pool.Submit(func() {
		done <- true
	})

	select {
	case <-done:
		// Task completed
	case <-time.After(1 * time.Second):
		t.Error("Tasks should complete within timeout")
	}

	pool.Wait()
}
