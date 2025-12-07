package hal

import (
	"sync"

	"github.com/gogpu/wgpu/types"
)

var (
	// backendsMu protects the backends map.
	backendsMu sync.RWMutex

	// backends stores registered backend implementations.
	backends = make(map[types.Backend]Backend)
)

// RegisterBackend registers a backend implementation.
// This is typically called from init() functions in backend packages.
// Registering the same backend type multiple times will replace the previous registration.
func RegisterBackend(backend Backend) {
	backendsMu.Lock()
	defer backendsMu.Unlock()
	backends[backend.Variant()] = backend
}

// GetBackend returns a registered backend by type.
// Returns (nil, false) if the backend is not registered.
func GetBackend(variant types.Backend) (Backend, bool) {
	backendsMu.RLock()
	defer backendsMu.RUnlock()
	b, ok := backends[variant]
	return b, ok
}

// AvailableBackends returns all registered backend types.
// The order is non-deterministic.
func AvailableBackends() []types.Backend {
	backendsMu.RLock()
	defer backendsMu.RUnlock()
	result := make([]types.Backend, 0, len(backends))
	for v := range backends {
		result = append(result, v)
	}
	return result
}
