package core

import (
	"fmt"

	"github.com/gogpu/wgpu/types"
)

// CreateDevice creates a device from an adapter.
// This is called internally by RequestDevice in adapter.go.
//
// The device is created with the specified features and limits,
// and a default queue is automatically created.
//
// Returns the device ID and an error if device creation fails.
func CreateDevice(adapterID AdapterID, desc *types.DeviceDescriptor) (DeviceID, error) {
	hub := GetGlobal().Hub()

	// Verify the adapter exists
	adapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		return DeviceID{}, fmt.Errorf("invalid adapter: %w", err)
	}

	// Use default descriptor if none provided
	if desc == nil {
		defaultDesc := types.DefaultDeviceDescriptor()
		desc = &defaultDesc
	}

	// Validate requested features are supported
	for _, feature := range desc.RequiredFeatures {
		if !adapter.Features.Contains(feature) {
			return DeviceID{}, fmt.Errorf("adapter does not support required feature: %v", feature)
		}
	}

	// TODO: Validate requested limits against adapter limits

	// Determine which features to enable
	// Start with required features
	enabledFeatures := types.Features(0)
	for _, feature := range desc.RequiredFeatures {
		enabledFeatures.Insert(feature)
	}

	// Use the descriptor's limits or default limits
	deviceLimits := desc.RequiredLimits

	// Create the queue first
	queue := Queue{
		// Device ID will be set after device is registered
		Label: desc.Label + " Queue",
	}
	queueID := hub.RegisterQueue(queue)

	// Create the device
	device := Device{
		Adapter:  adapterID,
		Label:    desc.Label,
		Features: enabledFeatures,
		Limits:   deviceLimits,
		Queue:    queueID,
	}

	// Register the device
	deviceID := hub.RegisterDevice(device)

	// Update the queue with the device ID
	queue.Device = deviceID
	err = hub.UpdateQueue(queueID, queue)
	if err != nil {
		// Rollback device registration if queue update fails
		_, _ = hub.UnregisterDevice(deviceID)
		_, _ = hub.UnregisterQueue(queueID)
		return DeviceID{}, fmt.Errorf("failed to update queue: %w", err)
	}

	return deviceID, nil
}

// GetDevice retrieves device data.
// Returns an error if the device ID is invalid.
func GetDevice(id DeviceID) (*Device, error) {
	hub := GetGlobal().Hub()
	device, err := hub.GetDevice(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	return &device, nil
}

// GetDeviceFeatures returns the device's enabled features.
// Returns an error if the device ID is invalid.
func GetDeviceFeatures(id DeviceID) (types.Features, error) {
	device, err := GetDevice(id)
	if err != nil {
		return 0, err
	}
	return device.Features, nil
}

// GetDeviceLimits returns the device's limits.
// Returns an error if the device ID is invalid.
func GetDeviceLimits(id DeviceID) (types.Limits, error) {
	device, err := GetDevice(id)
	if err != nil {
		return types.Limits{}, err
	}
	return device.Limits, nil
}

// GetDeviceQueue returns the device's queue.
// Returns an error if the device ID is invalid.
func GetDeviceQueue(id DeviceID) (QueueID, error) {
	device, err := GetDevice(id)
	if err != nil {
		return QueueID{}, err
	}
	return device.Queue, nil
}

// DeviceDrop destroys the device and its queue.
// After calling this function, the device ID and its queue ID become invalid.
//
// Returns an error if the device ID is invalid or if the device
// cannot be released (e.g., resources are still using it).
//
// Note: Currently this is a simple unregister. In a full implementation,
// this would check for active resources and properly clean up.
func DeviceDrop(id DeviceID) error {
	hub := GetGlobal().Hub()

	// Get the device to find its queue
	device, err := hub.GetDevice(id)
	if err != nil {
		return fmt.Errorf("failed to drop device: %w", err)
	}

	// TODO: Check if any resources are still using this device

	// Unregister the queue first
	_, err = hub.UnregisterQueue(device.Queue)
	if err != nil {
		return fmt.Errorf("failed to drop device queue: %w", err)
	}

	// Unregister the device
	_, err = hub.UnregisterDevice(id)
	if err != nil {
		return fmt.Errorf("failed to drop device: %w", err)
	}

	return nil
}

// DeviceCreateBuffer creates a buffer on this device.
// This is a placeholder implementation that will be expanded later.
//
// Returns a buffer ID that can be used to access the buffer, or an error if
// buffer creation fails.
func DeviceCreateBuffer(id DeviceID, desc *types.BufferDescriptor) (BufferID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return BufferID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return BufferID{}, fmt.Errorf("buffer descriptor is required")
	}

	// TODO: Validate buffer descriptor
	// TODO: Create actual buffer

	// Create a placeholder buffer
	buffer := Buffer{}
	bufferID := hub.RegisterBuffer(buffer)

	return bufferID, nil
}

// DeviceCreateTexture creates a texture on this device.
// This is a placeholder implementation that will be expanded later.
//
// Returns a texture ID that can be used to access the texture, or an error if
// texture creation fails.
func DeviceCreateTexture(id DeviceID, desc *types.TextureDescriptor) (TextureID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return TextureID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return TextureID{}, fmt.Errorf("texture descriptor is required")
	}

	// TODO: Validate texture descriptor
	// TODO: Create actual texture

	// Create a placeholder texture
	texture := Texture{}
	textureID := hub.RegisterTexture(texture)

	return textureID, nil
}

// DeviceCreateShaderModule creates a shader module.
// This is a placeholder implementation that will be expanded later.
//
// Returns a shader module ID that can be used to access the module, or an error if
// module creation fails.
func DeviceCreateShaderModule(id DeviceID, desc *types.ShaderModuleDescriptor) (ShaderModuleID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return ShaderModuleID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return ShaderModuleID{}, fmt.Errorf("shader module descriptor is required")
	}

	// TODO: Validate shader module descriptor
	// TODO: Create actual shader module

	// Create a placeholder shader module
	module := ShaderModule{}
	moduleID := hub.RegisterShaderModule(module)

	return moduleID, nil
}
