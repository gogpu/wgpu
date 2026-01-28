// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// BackendProvider bridges the Core API to HAL backend implementations.
// It provides a consistent interface for creating HAL instances and
// enumerating real GPU adapters.
//
// Backend providers are registered globally via RegisterBackendProvider()
// and are queried during Instance creation to enumerate real GPUs.
type BackendProvider interface {
	// Variant returns the backend type identifier (Vulkan, Metal, DX12, etc.).
	Variant() gputypes.Backend

	// CreateInstance creates a HAL instance with the given descriptor.
	// Returns an error if the backend is not available (e.g., drivers missing).
	CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error)

	// IsAvailable returns true if the backend can be used on this system.
	// This is a lightweight check that doesn't create resources.
	IsAvailable() bool
}

// halBackendProvider wraps a hal.Backend to implement BackendProvider.
// This is the default implementation that delegates to HAL backends.
type halBackendProvider struct {
	backend hal.Backend
}

// Variant returns the backend type identifier.
func (p *halBackendProvider) Variant() gputypes.Backend {
	return p.backend.Variant()
}

// CreateInstance creates a HAL instance using the wrapped backend.
func (p *halBackendProvider) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	return p.backend.CreateInstance(desc)
}

// IsAvailable returns true (HAL backends are considered available if registered).
func (p *halBackendProvider) IsAvailable() bool {
	return true
}

var (
	// providersMu protects the providers map.
	providersMu sync.RWMutex

	// providers stores registered backend providers by type.
	providers = make(map[gputypes.Backend]BackendProvider)

	// providerPriority defines the order in which backends are tried.
	// Higher priority backends are tried first.
	providerPriority = []gputypes.Backend{
		gputypes.BackendVulkan,
		gputypes.BackendMetal,
		gputypes.BackendDX12,
		gputypes.BackendGL,
		gputypes.BackendEmpty, // noop/software fallback
	}
)

// RegisterBackendProvider registers a backend provider for use by the Core API.
// This is typically called from init() functions in backend registration files.
//
// If a provider for the same backend type is already registered, it is replaced.
func RegisterBackendProvider(provider BackendProvider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[provider.Variant()] = provider
}

// GetBackendProvider returns a registered backend provider by type.
// Returns (nil, false) if no provider is registered for the given type.
func GetBackendProvider(variant gputypes.Backend) (BackendProvider, bool) {
	providersMu.RLock()
	defer providersMu.RUnlock()
	p, ok := providers[variant]
	return p, ok
}

// AvailableBackendProviders returns all registered backend provider gputypes.
// The order is non-deterministic.
func AvailableBackendProviders() []gputypes.Backend {
	providersMu.RLock()
	defer providersMu.RUnlock()
	result := make([]gputypes.Backend, 0, len(providers))
	for v := range providers {
		result = append(result, v)
	}
	return result
}

// GetOrderedBackendProviders returns registered providers in priority order.
// Higher priority backends (Vulkan, Metal) come first.
// Only returns providers that are registered and available.
func GetOrderedBackendProviders() []BackendProvider {
	providersMu.RLock()
	defer providersMu.RUnlock()

	result := make([]BackendProvider, 0, len(providers))
	for _, variant := range providerPriority {
		if p, ok := providers[variant]; ok && p.IsAvailable() {
			result = append(result, p)
		}
	}

	// Add any providers not in the priority list (custom backends)
	for variant, p := range providers {
		found := false
		for _, priorityVariant := range providerPriority {
			if variant == priorityVariant {
				found = true
				break
			}
		}
		if !found && p.IsAvailable() {
			result = append(result, p)
		}
	}

	return result
}

// SelectBestBackendProvider returns the highest-priority available backend provider.
// Returns nil if no backends are registered.
func SelectBestBackendProvider() BackendProvider {
	ordered := GetOrderedBackendProviders()
	if len(ordered) == 0 {
		return nil
	}
	return ordered[0]
}

// RegisterHALBackends automatically registers all HAL backends as Core backend providers.
// This is called during Core initialization to make HAL backends available.
//
// This function queries the HAL registry for all registered backends and creates
// wrapper providers for them.
func RegisterHALBackends() {
	for _, variant := range hal.AvailableBackends() {
		backend, ok := hal.GetBackend(variant)
		if ok {
			RegisterBackendProvider(&halBackendProvider{backend: backend})
		}
	}
}

// FilterBackendsByMask filters backend providers by the enabled backends mask.
// Returns only providers whose variant is enabled in the mask.
func FilterBackendsByMask(mask gputypes.Backends) []BackendProvider {
	ordered := GetOrderedBackendProviders()
	result := make([]BackendProvider, 0, len(ordered))

	for _, p := range ordered {
		// Check if this backend type is enabled in the mask
		switch p.Variant() {
		case gputypes.BackendVulkan:
			if mask&gputypes.BackendsVulkan != 0 {
				result = append(result, p)
			}
		case gputypes.BackendMetal:
			if mask&gputypes.BackendsMetal != 0 {
				result = append(result, p)
			}
		case gputypes.BackendDX12:
			if mask&gputypes.BackendsDX12 != 0 {
				result = append(result, p)
			}
		case gputypes.BackendGL:
			if mask&gputypes.BackendsGL != 0 {
				result = append(result, p)
			}
		case gputypes.BackendEmpty:
			// Empty/noop backend is always available as fallback
			result = append(result, p)
		default:
			// Unknown backend types pass through if Primary is set
			if mask&gputypes.BackendsPrimary != 0 {
				result = append(result, p)
			}
		}
	}

	return result
}
