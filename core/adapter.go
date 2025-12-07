package core

import (
	"fmt"

	"github.com/gogpu/wgpu/types"
)

// GetAdapterInfo returns information about the specified adapter.
// Returns an error if the adapter ID is invalid.
func GetAdapterInfo(id AdapterID) (types.AdapterInfo, error) {
	hub := GetGlobal().Hub()
	adapter, err := hub.GetAdapter(id)
	if err != nil {
		return types.AdapterInfo{}, fmt.Errorf("failed to get adapter info: %w", err)
	}
	return adapter.Info, nil
}

// GetAdapterFeatures returns the features supported by the specified adapter.
// Returns an error if the adapter ID is invalid.
func GetAdapterFeatures(id AdapterID) (types.Features, error) {
	hub := GetGlobal().Hub()
	adapter, err := hub.GetAdapter(id)
	if err != nil {
		return 0, fmt.Errorf("failed to get adapter features: %w", err)
	}
	return adapter.Features, nil
}

// GetAdapterLimits returns the resource limits of the specified adapter.
// Returns an error if the adapter ID is invalid.
func GetAdapterLimits(id AdapterID) (types.Limits, error) {
	hub := GetGlobal().Hub()
	adapter, err := hub.GetAdapter(id)
	if err != nil {
		return types.Limits{}, fmt.Errorf("failed to get adapter limits: %w", err)
	}
	return adapter.Limits, nil
}

// RequestDevice creates a logical device from the specified adapter.
// The device is configured according to the provided descriptor.
//
// The descriptor specifies:
//   - Required features the device must support
//   - Required limits the device must meet
//   - Debug label for the device
//
// Returns a DeviceID that can be used to access the device, or an error if
// device creation fails.
//
// Common failure reasons:
//   - Invalid adapter ID
//   - Requested features not supported by adapter
//   - Requested limits exceed adapter capabilities
func RequestDevice(adapterID AdapterID, desc *types.DeviceDescriptor) (DeviceID, error) {
	return CreateDevice(adapterID, desc)
}

// AdapterDrop releases the specified adapter and its associated resources.
// After calling this function, the adapter ID becomes invalid.
//
// Returns an error if the adapter ID is invalid or if the adapter
// cannot be released (e.g., devices are still using it).
//
// Note: Currently this is a simple unregister. In a full implementation,
// this would check for active devices and properly clean up resources.
func AdapterDrop(id AdapterID) error {
	hub := GetGlobal().Hub()

	// TODO: Check if any devices are still using this adapter
	// For now, just unregister it

	_, err := hub.UnregisterAdapter(id)
	if err != nil {
		return fmt.Errorf("failed to drop adapter: %w", err)
	}

	return nil
}
