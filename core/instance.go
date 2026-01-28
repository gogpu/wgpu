package core

import (
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
)

// Instance represents a WebGPU instance for GPU discovery and initialization.
// The instance is responsible for enumerating available GPU adapters and
// creating adapters based on application requirements.
//
// An instance maintains the list of available backends and their configuration.
// It is the entry point for all WebGPU operations.
//
// Thread-safe for concurrent use.
type Instance struct {
	mu       sync.RWMutex
	backends gputypes.Backends
	flags    gputypes.InstanceFlags

	// adapters contains the registered adapter IDs.
	adapters []AdapterID

	// halInstances tracks HAL instances created for each backend.
	// These are destroyed when the Instance is destroyed.
	halInstances []hal.Instance

	// useMock indicates whether to use mock adapters (for testing or when no HAL available).
	useMock bool
}

// NewInstance creates a new WebGPU instance with the given descriptor.
// If desc is nil, default settings are used.
//
// The instance will enumerate available GPU adapters based on the enabled
// backends specified in the descriptor. If HAL backends are available,
// real GPU adapters will be enumerated. Otherwise, a mock adapter is created
// for testing purposes.
func NewInstance(desc *gputypes.InstanceDescriptor) *Instance {
	if desc == nil {
		defaultDesc := gputypes.DefaultInstanceDescriptor()
		desc = &defaultDesc
	}

	i := &Instance{
		backends:     desc.Backends,
		flags:        desc.Flags,
		adapters:     []AdapterID{},
		halInstances: []hal.Instance{},
		useMock:      false,
	}

	// Try to enumerate real adapters via HAL backends
	realAdaptersFound := i.enumerateRealAdapters(desc)

	// Fall back to mock adapter if no real adapters were found
	if !realAdaptersFound {
		i.useMock = true
		i.createMockAdapter()
	}

	return i
}

// NewInstanceWithMock creates a new WebGPU instance with mock adapters.
// This is primarily for testing without requiring real GPU hardware.
func NewInstanceWithMock(desc *gputypes.InstanceDescriptor) *Instance {
	if desc == nil {
		defaultDesc := gputypes.DefaultInstanceDescriptor()
		desc = &defaultDesc
	}

	i := &Instance{
		backends:     desc.Backends,
		flags:        desc.Flags,
		adapters:     []AdapterID{},
		halInstances: []hal.Instance{},
		useMock:      true,
	}

	i.createMockAdapter()
	return i
}

// enumerateRealAdapters attempts to enumerate real GPU adapters via HAL backends.
// Returns true if at least one real adapter was found.
func (i *Instance) enumerateRealAdapters(desc *gputypes.InstanceDescriptor) bool {
	// First, ensure HAL backends are registered
	RegisterHALBackends()

	// Get backend providers filtered by the enabled backends mask
	providers := FilterBackendsByMask(desc.Backends)
	if len(providers) == 0 {
		return false
	}

	foundAdapters := false
	hub := GetGlobal().Hub()

	// Create HAL descriptor
	halDesc := &hal.InstanceDescriptor{
		Backends: desc.Backends,
		Flags:    desc.Flags,
	}

	// Try each backend provider
	for _, provider := range providers {
		// Skip noop/empty backend - we'll use that as mock fallback
		if provider.Variant() == gputypes.BackendEmpty {
			continue
		}

		// Try to create HAL instance
		halInstance, err := provider.CreateInstance(halDesc)
		if err != nil {
			// Backend not available, try next
			continue
		}

		// Track HAL instance for cleanup
		i.halInstances = append(i.halInstances, halInstance)

		// Enumerate adapters from this backend
		exposedAdapters := halInstance.EnumerateAdapters(nil)
		for idx := range exposedAdapters {
			exposed := &exposedAdapters[idx] // Use pointer to avoid copy
			// Create core.Adapter wrapping the HAL adapter
			adapter := &Adapter{
				Info:            exposed.Info,
				Features:        exposed.Features,
				Limits:          exposed.Capabilities.Limits,
				Backend:         exposed.Info.Backend,
				halAdapter:      exposed.Adapter,
				halCapabilities: &exposed.Capabilities,
			}

			// Register in the hub
			adapterID := hub.RegisterAdapter(adapter)
			i.adapters = append(i.adapters, adapterID)
			foundAdapters = true
		}
	}

	return foundAdapters
}

// createMockAdapter creates a mock adapter for testing purposes.
// Mock adapters provide a functional Core API without requiring real GPU hardware.
func (i *Instance) createMockAdapter() {
	// Create a mock adapter with reasonable default values
	adapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "Mock Adapter",
			Vendor:     "MockVendor",
			VendorID:   0x1234,
			DeviceID:   0x5678,
			DeviceType: gputypes.DeviceTypeDiscreteGPU,
			Driver:     "1.0.0",
			DriverInfo: "Mock Driver (no real GPU)",
			Backend:    gputypes.BackendVulkan,
		},
		Features: gputypes.Features(0), // No special features for mock
		Limits:   gputypes.DefaultLimits(),
		Backend:  gputypes.BackendVulkan,
		// HAL fields are nil for mock adapters
		halAdapter:      nil,
		halCapabilities: nil,
	}

	// Register the adapter in the global hub
	hub := GetGlobal().Hub()
	adapterID := hub.RegisterAdapter(adapter)
	i.adapters = append(i.adapters, adapterID)
}

// EnumerateAdapters returns a list of all available GPU adapters.
// The adapters are filtered based on the backends enabled in the instance.
//
// This method returns a snapshot of available adapters at the time of the call.
// The adapter list may change if GPUs are added or removed from the system.
func (i *Instance) EnumerateAdapters() []AdapterID {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]AdapterID, len(i.adapters))
	copy(result, i.adapters)
	return result
}

// RequestAdapter requests an adapter matching the given options.
// Returns the first adapter that meets the requirements, or an error if none found.
//
// Options control adapter selection:
//   - PowerPreference: prefer low-power or high-performance adapters
//   - ForceFallbackAdapter: use software rendering
//   - CompatibleSurface: adapter must support the given surface
//
// If options is nil, the first available adapter is returned.
func (i *Instance) RequestAdapter(options *gputypes.RequestAdapterOptions) (AdapterID, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if len(i.adapters) == 0 {
		return AdapterID{}, fmt.Errorf("no adapters available")
	}

	// If no options specified, return the first adapter
	if options == nil {
		return i.adapters[0], nil
	}

	hub := GetGlobal().Hub()

	// Filter adapters based on power preference
	for _, adapterID := range i.adapters {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue // Skip invalid adapters
		}

		// Check power preference
		if options.PowerPreference != gputypes.PowerPreferenceNone {
			if !matchesPowerPreference(adapter.Info.DeviceType, options.PowerPreference) {
				continue
			}
		}

		// Check fallback adapter requirement
		if options.ForceFallbackAdapter && adapter.Info.DeviceType != gputypes.DeviceTypeCPU {
			continue
		}

		// If we get here, the adapter matches requirements
		return adapterID, nil
	}

	return AdapterID{}, fmt.Errorf("no adapter matches the requested options")
}

// matchesPowerPreference checks if a device type matches the power preference.
func matchesPowerPreference(deviceType gputypes.DeviceType, preference gputypes.PowerPreference) bool {
	switch preference {
	case gputypes.PowerPreferenceLowPower:
		// Prefer integrated GPUs for low power
		return deviceType == gputypes.DeviceTypeIntegratedGPU
	case gputypes.PowerPreferenceHighPerformance:
		// Prefer discrete GPUs for high performance
		return deviceType == gputypes.DeviceTypeDiscreteGPU
	default:
		return true
	}
}

// Backends returns the enabled backends for this instance.
func (i *Instance) Backends() gputypes.Backends {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.backends
}

// Flags returns the instance flags.
func (i *Instance) Flags() gputypes.InstanceFlags {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.flags
}

// IsMock returns true if the instance is using mock adapters.
// Mock adapters are used when no HAL backends are available or
// when the instance was explicitly created with NewInstanceWithMock.
func (i *Instance) IsMock() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.useMock
}

// HasHALAdapters returns true if any real HAL adapters are available.
func (i *Instance) HasHALAdapters() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.halInstances) > 0 && !i.useMock
}

// Destroy releases all resources associated with this instance.
// This includes unregistering all adapters and destroying HAL instances.
// After calling Destroy, the instance should not be used.
func (i *Instance) Destroy() {
	i.mu.Lock()
	defer i.mu.Unlock()

	hub := GetGlobal().Hub()

	// Unregister all adapters from the hub
	for _, adapterID := range i.adapters {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}

		// Destroy the HAL adapter if present
		if adapter.halAdapter != nil {
			adapter.halAdapter.Destroy()
		}

		// Unregister from hub
		_, _ = hub.UnregisterAdapter(adapterID)
	}
	i.adapters = nil

	// Destroy all HAL instances
	for _, halInstance := range i.halInstances {
		halInstance.Destroy()
	}
	i.halInstances = nil
}
