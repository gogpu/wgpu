package core

import (
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/types"
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
	backends types.Backends
	flags    types.InstanceFlags
	// Mock adapter list for testing - will be replaced with real enumeration
	adapters []AdapterID
}

// NewInstance creates a new WebGPU instance with the given descriptor.
// If desc is nil, default settings are used.
//
// The instance will enumerate available GPU adapters based on the enabled
// backends specified in the descriptor.
func NewInstance(desc *types.InstanceDescriptor) *Instance {
	if desc == nil {
		defaultDesc := types.DefaultInstanceDescriptor()
		desc = &defaultDesc
	}

	i := &Instance{
		backends: desc.Backends,
		flags:    desc.Flags,
		adapters: []AdapterID{}, // Will be populated by enumeration
	}

	// TODO: Implement real adapter enumeration
	// For now, create a mock adapter for testing
	i.createMockAdapter()

	return i
}

// createMockAdapter creates a mock adapter for testing purposes.
// This will be replaced with real adapter enumeration.
func (i *Instance) createMockAdapter() {
	// Create a mock adapter with reasonable default values
	adapter := &Adapter{
		Info: types.AdapterInfo{
			Name:       "Mock Adapter",
			Vendor:     "MockVendor",
			VendorID:   0x1234,
			DeviceID:   0x5678,
			DeviceType: types.DeviceTypeDiscreteGPU,
			Driver:     "1.0.0",
			DriverInfo: "Mock Driver",
			Backend:    types.BackendVulkan,
		},
		Features: types.Features(0), // No special features for now
		Limits:   types.DefaultLimits(),
		Backend:  types.BackendVulkan,
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
func (i *Instance) RequestAdapter(options *types.RequestAdapterOptions) (AdapterID, error) {
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
		if options.PowerPreference != types.PowerPreferenceNone {
			if !matchesPowerPreference(adapter.Info.DeviceType, options.PowerPreference) {
				continue
			}
		}

		// Check fallback adapter requirement
		if options.ForceFallbackAdapter && adapter.Info.DeviceType != types.DeviceTypeCPU {
			continue
		}

		// If we get here, the adapter matches requirements
		return adapterID, nil
	}

	return AdapterID{}, fmt.Errorf("no adapter matches the requested options")
}

// matchesPowerPreference checks if a device type matches the power preference.
func matchesPowerPreference(deviceType types.DeviceType, preference types.PowerPreference) bool {
	switch preference {
	case types.PowerPreferenceLowPower:
		// Prefer integrated GPUs for low power
		return deviceType == types.DeviceTypeIntegratedGPU
	case types.PowerPreferenceHighPerformance:
		// Prefer discrete GPUs for high performance
		return deviceType == types.DeviceTypeDiscreteGPU
	default:
		return true
	}
}

// Backends returns the enabled backends for this instance.
func (i *Instance) Backends() types.Backends {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.backends
}

// Flags returns the instance flags.
func (i *Instance) Flags() types.InstanceFlags {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.flags
}
