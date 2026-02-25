package raster

import (
	"runtime"
	"sync"
)

// ParallelConfig configures parallel rasterization.
type ParallelConfig struct {
	// Workers is the number of worker goroutines.
	// If 0, defaults to runtime.NumCPU().
	Workers int

	// TileSize is the tile size for work distribution.
	// If 0, uses the default TileSize constant.
	TileSize int

	// MinTriangles is the minimum number of triangles to parallelize.
	// Below this threshold, single-threaded execution is used to avoid overhead.
	// If 0, defaults to 10.
	MinTriangles int
}

// DefaultParallelConfig returns sensible defaults for parallel rasterization.
func DefaultParallelConfig() ParallelConfig {
	return ParallelConfig{
		Workers:      runtime.NumCPU(),
		TileSize:     TileSize,
		MinTriangles: 10,
	}
}

// WorkerPool manages a pool of worker goroutines for parallel execution.
// Tasks are submitted via channels and executed concurrently.
type WorkerPool struct {
	workers int
	wg      sync.WaitGroup
	tasks   chan func()
	quit    chan struct{}
	started bool
	mu      sync.Mutex
}

// NewWorkerPool creates a new worker pool with the specified number of workers.
// If workers <= 0, it defaults to runtime.NumCPU().
func NewWorkerPool(workers int) *WorkerPool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return &WorkerPool{
		workers: workers,
		tasks:   make(chan func(), workers*4), // Buffered channel
		quit:    make(chan struct{}),
		started: false,
	}
}

// Start launches the worker goroutines.
// It is safe to call Start multiple times; subsequent calls are no-ops.
func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return
	}

	p.started = true
	for i := 0; i < p.workers; i++ {
		go p.worker()
	}
}

// worker is the main loop for a worker goroutine.
func (p *WorkerPool) worker() {
	for {
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			task()
			p.wg.Done()
		case <-p.quit:
			return
		}
	}
}

// Submit adds a task to the worker pool.
// The task will be executed by one of the workers.
// This method blocks if the task queue is full.
func (p *WorkerPool) Submit(task func()) {
	p.wg.Add(1)
	p.tasks <- task
}

// Wait blocks until all submitted tasks have completed.
func (p *WorkerPool) Wait() {
	p.wg.Wait()
}

// Close shuts down the worker pool.
// It signals all workers to stop and waits for pending tasks.
func (p *WorkerPool) Close() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	close(p.quit)
	// Drain remaining tasks
	close(p.tasks)
}

// Workers returns the number of workers in the pool.
func (p *WorkerPool) Workers() int {
	return p.workers
}

// ParallelRasterizer handles parallel rasterization of triangles.
// It divides the framebuffer into tiles and distributes work across workers.
type ParallelRasterizer struct {
	config ParallelConfig
	grid   *TileGrid
	pool   *WorkerPool
}

// NewParallelRasterizer creates a new parallel rasterizer for the given dimensions.
func NewParallelRasterizer(width, height int, config ParallelConfig) *ParallelRasterizer {
	if config.Workers <= 0 {
		config.Workers = runtime.NumCPU()
	}
	if config.TileSize <= 0 {
		config.TileSize = TileSize
	}
	if config.MinTriangles <= 0 {
		config.MinTriangles = 10
	}

	pool := NewWorkerPool(config.Workers)
	pool.Start()

	return &ParallelRasterizer{
		config: config,
		grid:   NewTileGrid(width, height),
		pool:   pool,
	}
}

// Resize updates the tile grid for new dimensions.
func (r *ParallelRasterizer) Resize(width, height int) {
	r.grid = NewTileGrid(width, height)
}

// Close shuts down the parallel rasterizer and releases resources.
func (r *ParallelRasterizer) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

// Config returns the current parallel configuration.
func (r *ParallelRasterizer) Config() ParallelConfig {
	return r.config
}

// Grid returns the tile grid.
func (r *ParallelRasterizer) Grid() *TileGrid {
	return r.grid
}

// RasterizeParallel rasterizes triangles in parallel by tile.
// The callback is invoked for each tile with its assigned triangles.
// Each tile is processed by exactly one goroutine, so no synchronization
// is needed for tile-local writes.
func (r *ParallelRasterizer) RasterizeParallel(
	triangles []Triangle,
	callback func(tile Tile, triangles []Triangle),
) {
	// Use single-threaded path for small workloads
	if len(triangles) < r.config.MinTriangles {
		r.rasterizeSingleThreaded(triangles, callback)
		return
	}

	// Bin triangles to tiles
	tileBins := BinTrianglesToTiles(triangles, r.grid)

	// Process each tile in parallel
	for tileIdx, tileTriangles := range tileBins {
		if len(tileTriangles) == 0 {
			continue
		}

		tile := r.grid.tiles[tileIdx]
		tris := tileTriangles // Capture for closure

		r.pool.Submit(func() {
			callback(tile, tris)
		})
	}

	r.pool.Wait()
}

// rasterizeSingleThreaded processes triangles without parallelization.
func (r *ParallelRasterizer) rasterizeSingleThreaded(
	triangles []Triangle,
	callback func(tile Tile, triangles []Triangle),
) {
	tileBins := BinTrianglesToTiles(triangles, r.grid)

	for tileIdx, tileTriangles := range tileBins {
		if len(tileTriangles) == 0 {
			continue
		}
		callback(r.grid.tiles[tileIdx], tileTriangles)
	}
}

// BinTrianglesToTiles assigns each triangle to the tiles it overlaps.
// Returns a map from tile index to the list of triangles in that tile.
// A triangle may appear in multiple tiles if it spans tile boundaries.
func BinTrianglesToTiles(triangles []Triangle, grid *TileGrid) map[int][]Triangle {
	result := make(map[int][]Triangle)

	for i := range triangles {
		tri := &triangles[i]
		tiles := grid.GetTilesForTriangle(*tri)

		for _, tile := range tiles {
			idx := grid.TileIndex(tile.X, tile.Y)
			result[idx] = append(result[idx], *tri)
		}
	}

	return result
}

// BinTrianglesToTilesWithTest is like BinTrianglesToTiles but uses
// hierarchical tile-triangle testing to reject tiles that don't actually
// overlap the triangle.
func BinTrianglesToTilesWithTest(triangles []Triangle, grid *TileGrid) map[int][]Triangle {
	result := make(map[int][]Triangle)

	for i := range triangles {
		tri := &triangles[i]

		// Compute edge functions once per triangle
		e01 := NewEdgeFunction(tri.V0.X, tri.V0.Y, tri.V1.X, tri.V1.Y)
		e12 := NewEdgeFunction(tri.V1.X, tri.V1.Y, tri.V2.X, tri.V2.Y)
		e20 := NewEdgeFunction(tri.V2.X, tri.V2.Y, tri.V0.X, tri.V0.Y)

		tiles := grid.GetTilesForTriangle(*tri)

		for _, tile := range tiles {
			// Use hierarchical test to skip tiles outside triangle
			if TileTriangleTest(tile, e01, e12, e20) != -1 {
				idx := grid.TileIndex(tile.X, tile.Y)
				result[idx] = append(result[idx], *tri)
			}
		}
	}

	return result
}

// fragmentPool is used to reduce allocations for fragment slices.
var fragmentPool = sync.Pool{
	New: func() interface{} {
		s := make([]Fragment, 0, 64)
		return &s
	},
}

// GetFragmentSlice obtains a fragment slice from the pool.
func GetFragmentSlice() *[]Fragment {
	return fragmentPool.Get().(*[]Fragment)
}

// PutFragmentSlice returns a fragment slice to the pool.
func PutFragmentSlice(s *[]Fragment) {
	*s = (*s)[:0] // Reset length but keep capacity
	fragmentPool.Put(s)
}

// ParallelForEachTile executes a function for each tile in parallel.
// This is useful for operations that need to process all tiles.
func (r *ParallelRasterizer) ParallelForEachTile(fn func(tile Tile)) {
	for _, tile := range r.grid.tiles {
		t := tile // Capture for closure
		r.pool.Submit(func() {
			fn(t)
		})
	}
	r.pool.Wait()
}
