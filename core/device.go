package core

import (
	"fmt"

	"github.com/gogpu/gputypes"
)

// CreateDevice creates a device from an adapter.
// This is called internally by RequestDevice in adapter.go.
//
// The device is created with the specified features and limits,
// and a default queue is automatically created.
//
// Returns the device ID and an error if device creation fails.
func CreateDevice(adapterID AdapterID, desc *gputypes.DeviceDescriptor) (DeviceID, error) {
	hub := GetGlobal().Hub()

	// Verify the adapter exists
	adapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		return DeviceID{}, fmt.Errorf("invalid adapter: %w", err)
	}

	// Use default descriptor if none provided
	if desc == nil {
		defaultDesc := gputypes.DefaultDeviceDescriptor()
		desc = &defaultDesc
	}

	// Validate requested features are supported
	for _, feature := range desc.RequiredFeatures {
		if !adapter.Features.Contains(feature) {
			return DeviceID{}, fmt.Errorf("adapter does not support required feature: %v", feature)
		}
	}

	// Note: Limits validation against adapter limits is deferred.
	// The HAL-based API (NewDevice) handles this at the HAL layer.

	// Determine which features to enable
	// Start with required features
	enabledFeatures := gputypes.Features(0)
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
func GetDeviceFeatures(id DeviceID) (gputypes.Features, error) {
	device, err := GetDevice(id)
	if err != nil {
		return 0, err
	}
	return device.Features, nil
}

// GetDeviceLimits returns the device's limits.
// Returns an error if the device ID is invalid.
func GetDeviceLimits(id DeviceID) (gputypes.Limits, error) {
	device, err := GetDevice(id)
	if err != nil {
		return gputypes.Limits{}, err
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

	// Note: Resource reference counting is handled by the HAL-based API.
	// The ID-based API does not track resource references.

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
//
// Deprecated: This is the legacy ID-based API. For new code, use the
// HAL-based API: Device.CreateBuffer() which provides full GPU integration.
// See resource.go for the HAL-based implementation.
//
// This function creates a placeholder buffer without actual GPU resources.
// It exists for backward compatibility with existing code.
//
// Returns a buffer ID that can be used to access the buffer, or an error if
// buffer creation fails.
func DeviceCreateBuffer(id DeviceID, desc *gputypes.BufferDescriptor) (BufferID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return BufferID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return BufferID{}, fmt.Errorf("buffer descriptor is required")
	}

	// Note: This creates a placeholder buffer without HAL integration.
	// For actual GPU buffers, use Device.CreateBuffer() instead.
	buffer := Buffer{}
	bufferID := hub.RegisterBuffer(buffer)

	return bufferID, nil
}

// DeviceCreateTexture creates a texture on this device.
//
// Deprecated: This is the legacy ID-based API. For new code, use the
// HAL-based API: Device.CreateTexture() (when implemented).
//
// This function creates a placeholder texture without actual GPU resources.
// It exists for backward compatibility with existing code.
//
// Returns a texture ID that can be used to access the texture, or an error if
// texture creation fails.
func DeviceCreateTexture(id DeviceID, desc *gputypes.TextureDescriptor) (TextureID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return TextureID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return TextureID{}, fmt.Errorf("texture descriptor is required")
	}

	// Note: This creates a placeholder texture without HAL integration.
	// HAL-based Device.CreateTexture() will be added in a future release.
	texture := Texture{}
	textureID := hub.RegisterTexture(texture)

	return textureID, nil
}

// DeviceCreateShaderModule creates a shader module.
//
// Deprecated: This is the legacy ID-based API. For new code, use the
// HAL-based API: Device.CreateShaderModule() (when implemented).
//
// This function creates a placeholder shader module without actual GPU resources.
// It exists for backward compatibility with existing code.
//
// Returns a shader module ID that can be used to access the module, or an error if
// module creation fails.
func DeviceCreateShaderModule(id DeviceID, desc *gputypes.ShaderModuleDescriptor) (ShaderModuleID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return ShaderModuleID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return ShaderModuleID{}, fmt.Errorf("shader module descriptor is required")
	}

	// Note: This creates a placeholder shader module without HAL integration.
	// HAL-based Device.CreateShaderModule() will be added in a future release.
	module := ShaderModule{}
	moduleID := hub.RegisterShaderModule(module)

	return moduleID, nil
}
