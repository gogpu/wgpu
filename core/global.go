package core

import (
	"sync"
)

// Global is the singleton managing global WebGPU state.
// It holds the Hub (for GPU resources) and the surface registry.
//
// The Global provides the top-level API for resource management,
// coordinating between surfaces and the GPU resource hub.
//
// Thread-safe for concurrent use via singleton pattern.
type Global struct {
	mu       sync.RWMutex
	surfaces *Registry[Surface, surfaceMarker]
	hub      *Hub
}

var (
	globalOnce     sync.Once
	globalInstance *Global
)

// GetGlobal returns the singleton Global instance.
// The instance is created lazily on first call.
func GetGlobal() *Global {
	globalOnce.Do(func() {
		globalInstance = &Global{
			surfaces: NewRegistry[Surface, surfaceMarker](),
			hub:      NewHub(),
		}
	})
	return globalInstance
}

// Hub returns the GPU resource hub.
// The hub manages all GPU resources (adapters, devices, buffers, etc.).
func (g *Global) Hub() *Hub {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.hub
}

// Surface methods

// RegisterSurface allocates a new ID and stores the surface.
// Surfaces are managed separately from other GPU resources because
// they're tied to windowing systems and created before adapters.
func (g *Global) RegisterSurface(surface Surface) SurfaceID {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.surfaces.Register(surface)
}

// GetSurface retrieves a surface by ID.
func (g *Global) GetSurface(id SurfaceID) (Surface, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.surfaces.Get(id)
}

// UnregisterSurface removes a surface by ID.
func (g *Global) UnregisterSurface(id SurfaceID) (Surface, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.surfaces.Unregister(id)
}

// SurfaceCount returns the number of registered surfaces.
func (g *Global) SurfaceCount() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.surfaces.Count()
}

// Stats returns statistics about global resource usage.
// Returns surface count and all hub resource counts.
func (g *Global) Stats() map[string]uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := g.hub.ResourceCounts()
	stats["surfaces"] = g.surfaces.Count()
	return stats
}

// Clear removes all resources from the global state.
// Note: This does not release IDs properly - use only for cleanup/testing.
func (g *Global) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.surfaces.Clear()
	g.hub.Clear()
}

// ResetGlobal resets the global instance for testing.
// This allows tests to start with a clean state.
// Should only be used in tests.
func ResetGlobal() {
	globalInstance = &Global{
		surfaces: NewRegistry[Surface, surfaceMarker](),
		hub:      NewHub(),
	}
}
