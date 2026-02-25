package raster

import (
	"sync"
)

// DepthBuffer stores depth values for the software rasterizer.
// Depth values are float32 in the range [0, 1] where 0 is near and 1 is far.
// The buffer is thread-safe for concurrent access.
type DepthBuffer struct {
	data   []float32
	width  int
	height int
	mu     sync.RWMutex
}

// NewDepthBuffer creates a new depth buffer with the given dimensions.
// All values are initialized to 1.0 (far plane).
func NewDepthBuffer(width, height int) *DepthBuffer {
	size := width * height
	data := make([]float32, size)

	// Initialize to far plane (1.0)
	for i := range data {
		data[i] = 1.0
	}

	return &DepthBuffer{
		data:   data,
		width:  width,
		height: height,
	}
}

// Width returns the width of the depth buffer.
func (d *DepthBuffer) Width() int {
	return d.width
}

// Height returns the height of the depth buffer.
func (d *DepthBuffer) Height() int {
	return d.height
}

// Clear fills the entire depth buffer with the given value.
// Typically called with 1.0 to reset to the far plane.
func (d *DepthBuffer) Clear(value float32) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.data {
		d.data[i] = value
	}
}

// Get returns the depth value at pixel (x, y).
// Returns 1.0 if coordinates are out of bounds.
func (d *DepthBuffer) Get(x, y int) float32 {
	if x < 0 || x >= d.width || y < 0 || y >= d.height {
		return 1.0
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.data[y*d.width+x]
}

// Set writes a depth value at pixel (x, y).
// Out-of-bounds coordinates are silently ignored.
func (d *DepthBuffer) Set(x, y int, depth float32) {
	if x < 0 || x >= d.width || y < 0 || y >= d.height {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.data[y*d.width+x] = depth
}

// Test performs a depth test at pixel (x, y).
// Returns true if the test passes (fragment should be drawn).
// This does NOT modify the depth buffer.
func (d *DepthBuffer) Test(x, y int, depth float32, compare CompareFunc) bool {
	if x < 0 || x >= d.width || y < 0 || y >= d.height {
		return false
	}

	d.mu.RLock()
	storedDepth := d.data[y*d.width+x]
	d.mu.RUnlock()

	return compareDepth(depth, storedDepth, compare)
}

// TestAndSet performs a depth test and updates the buffer if the test passes.
// Returns true if the test passed and the buffer was updated.
// This is an atomic test-and-write operation.
func (d *DepthBuffer) TestAndSet(x, y int, depth float32, compare CompareFunc, write bool) bool {
	if x < 0 || x >= d.width || y < 0 || y >= d.height {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	idx := y*d.width + x
	storedDepth := d.data[idx]

	if !compareDepth(depth, storedDepth, compare) {
		return false
	}

	if write {
		d.data[idx] = depth
	}

	return true
}

// GetData returns a copy of the raw depth buffer data.
// The data is in row-major order (y * width + x).
func (d *DepthBuffer) GetData() []float32 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]float32, len(d.data))
	copy(result, d.data)
	return result
}

// compareDepth compares source depth against destination using the specified function.
func compareDepth(src, dst float32, compare CompareFunc) bool {
	switch compare {
	case CompareNever:
		return false
	case CompareLess:
		return src < dst
	case CompareEqual:
		return src == dst
	case CompareLessEqual:
		return src <= dst
	case CompareGreater:
		return src > dst
	case CompareNotEqual:
		return src != dst
	case CompareGreaterEqual:
		return src >= dst
	case CompareAlways:
		return true
	default:
		return false
	}
}
