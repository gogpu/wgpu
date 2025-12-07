package core

import (
	"sync"
	"testing"

	"github.com/gogpu/wgpu/types"
)

// verifyDeviceCreation verifies that a device was created correctly
func verifyDeviceCreation(t *testing.T, deviceID DeviceID, adapterID AdapterID, desc *types.DeviceDescriptor) {
	t.Helper()

	// Verify device was created
	device, err := GetDevice(deviceID)
	if err != nil {
		t.Errorf("GetDevice() error = %v", err)
		return
	}

	// Verify adapter reference
	if device.Adapter != adapterID {
		t.Errorf("Device.Adapter = %v, want %v", device.Adapter, adapterID)
	}

	// Verify queue was created
	queueID := device.Queue
	queue, err := GetQueue(queueID)
	if err != nil {
		t.Errorf("GetQueue() error = %v", err)
		return
	}

	// Verify queue references device
	if queue.Device != deviceID {
		t.Errorf("Queue.Device = %v, want %v", queue.Device, deviceID)
	}

	// Verify label if provided
	if desc != nil && device.Label != desc.Label {
		t.Errorf("Device.Label = %v, want %v", device.Label, desc.Label)
	}
}

func TestCreateDevice(t *testing.T) {
	tests := []struct {
		name         string
		setupAdapter func() AdapterID
		desc         *types.DeviceDescriptor
		wantErr      bool
	}{
		{
			name: "create device with default descriptor",
			setupAdapter: func() AdapterID {
				return createTestAdapter(t, types.Features(0), types.DefaultLimits())
			},
			desc:    nil,
			wantErr: false,
		},
		{
			name: "create device with custom label",
			setupAdapter: func() AdapterID {
				return createTestAdapter(t, types.Features(0), types.DefaultLimits())
			},
			desc: &types.DeviceDescriptor{
				Label:            "Test Device",
				RequiredFeatures: nil,
				RequiredLimits:   types.DefaultLimits(),
			},
			wantErr: false,
		},
		{
			name: "create device with supported features",
			setupAdapter: func() AdapterID {
				features := types.Features(0)
				features.Insert(types.FeatureDepthClipControl)
				features.Insert(types.FeatureTimestampQuery)
				return createTestAdapter(t, features, types.DefaultLimits())
			},
			desc: &types.DeviceDescriptor{
				Label: "Device with features",
				RequiredFeatures: []types.Feature{
					types.FeatureDepthClipControl,
					types.FeatureTimestampQuery,
				},
				RequiredLimits: types.DefaultLimits(),
			},
			wantErr: false,
		},
		{
			name: "fail with unsupported features",
			setupAdapter: func() AdapterID {
				// Adapter with no features
				return createTestAdapter(t, types.Features(0), types.DefaultLimits())
			},
			desc: &types.DeviceDescriptor{
				Label: "Device with unsupported features",
				RequiredFeatures: []types.Feature{
					types.FeatureDepthClipControl,
				},
				RequiredLimits: types.DefaultLimits(),
			},
			wantErr: true,
		},
		{
			name: "fail with invalid adapter",
			setupAdapter: func() AdapterID {
				return AdapterID{} // Invalid ID
			},
			desc:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global state
			ResetGlobal()

			adapterID := tt.setupAdapter()
			deviceID, err := CreateDevice(adapterID, tt.desc)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateDevice() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify device was created successfully
			verifyDeviceCreation(t, deviceID, adapterID, tt.desc)
		})
	}
}

func TestGetDeviceFeatures(t *testing.T) {
	ResetGlobal()

	features := types.Features(0)
	features.Insert(types.FeatureDepthClipControl)
	features.Insert(types.FeatureTimestampQuery)

	adapterID := createTestAdapter(t, features, types.DefaultLimits())
	desc := &types.DeviceDescriptor{
		Label: "Test Device",
		RequiredFeatures: []types.Feature{
			types.FeatureDepthClipControl,
		},
		RequiredLimits: types.DefaultLimits(),
	}

	deviceID, err := CreateDevice(adapterID, desc)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	gotFeatures, err := GetDeviceFeatures(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceFeatures() error = %v", err)
	}

	if !gotFeatures.Contains(types.FeatureDepthClipControl) {
		t.Errorf("Device features should contain FeatureDepthClipControl")
	}
}

func TestGetDeviceLimits(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	limits, err := GetDeviceLimits(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceLimits() error = %v", err)
	}

	// Verify limits are set
	if limits.MaxTextureDimension2D == 0 {
		t.Errorf("Device limits should have MaxTextureDimension2D set")
	}
}

func TestGetDeviceQueue(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	queueID, err := GetDeviceQueue(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceQueue() error = %v", err)
	}

	// Verify queue exists
	queue, err := GetQueue(queueID)
	if err != nil {
		t.Errorf("GetQueue() error = %v", err)
	}

	// Verify queue references device
	if queue.Device != deviceID {
		t.Errorf("Queue.Device = %v, want %v", queue.Device, deviceID)
	}
}

func TestDeviceDrop(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() DeviceID
		wantErr bool
	}{
		{
			name: "drop valid device",
			setup: func() DeviceID {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				return deviceID
			},
			wantErr: false,
		},
		{
			name: "drop invalid device",
			setup: func() DeviceID {
				ResetGlobal()
				return DeviceID{} // Invalid ID
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceID := tt.setup()
			err := DeviceDrop(deviceID)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeviceDrop() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify device no longer exists
				_, err := GetDevice(deviceID)
				if err == nil {
					t.Errorf("GetDevice() should fail after drop")
				}
			}
		})
	}
}

func TestDeviceCreateBuffer(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	tests := []struct {
		name    string
		desc    *types.BufferDescriptor
		wantErr bool
	}{
		{
			name: "create buffer with valid descriptor",
			desc: &types.BufferDescriptor{
				Label: "Test Buffer",
				Size:  256,
				Usage: types.BufferUsageVertex | types.BufferUsageCopyDst,
			},
			wantErr: false,
		},
		{
			name:    "fail with nil descriptor",
			desc:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bufferID, err := DeviceCreateBuffer(deviceID, tt.desc)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeviceCreateBuffer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify buffer was created
				hub := GetGlobal().Hub()
				_, err := hub.GetBuffer(bufferID)
				if err != nil {
					t.Errorf("Buffer should exist after creation")
				}
			}
		})
	}
}

func TestDeviceCreateTexture(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	tests := []struct {
		name    string
		desc    *types.TextureDescriptor
		wantErr bool
	}{
		{
			name: "create texture with valid descriptor",
			desc: &types.TextureDescriptor{
				Label: "Test Texture",
				Size: types.Extent3D{
					Width:              256,
					Height:             256,
					DepthOrArrayLayers: 1,
				},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     types.TextureDimension2D,
				Format:        types.TextureFormatRGBA8Unorm,
				Usage:         types.TextureUsageTextureBinding | types.TextureUsageCopyDst,
			},
			wantErr: false,
		},
		{
			name:    "fail with nil descriptor",
			desc:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			textureID, err := DeviceCreateTexture(deviceID, tt.desc)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeviceCreateTexture() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify texture was created
				hub := GetGlobal().Hub()
				_, err := hub.GetTexture(textureID)
				if err != nil {
					t.Errorf("Texture should exist after creation")
				}
			}
		})
	}
}

func TestDeviceCreateShaderModule(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	tests := []struct {
		name    string
		desc    *types.ShaderModuleDescriptor
		wantErr bool
	}{
		{
			name: "create shader module with WGSL",
			desc: &types.ShaderModuleDescriptor{
				Label: "Test Shader",
				Source: types.ShaderSourceWGSL{
					Code: "@vertex fn main() -> @builtin(position) vec4<f32> { return vec4<f32>(0.0); }",
				},
			},
			wantErr: false,
		},
		{
			name:    "fail with nil descriptor",
			desc:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moduleID, err := DeviceCreateShaderModule(deviceID, tt.desc)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeviceCreateShaderModule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify shader module was created
				hub := GetGlobal().Hub()
				_, err := hub.GetShaderModule(moduleID)
				if err != nil {
					t.Errorf("ShaderModule should exist after creation")
				}
			}
		})
	}
}

func TestDeviceConcurrentAccess(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())

	// Create multiple devices concurrently
	const numDevices = 10
	var wg sync.WaitGroup
	deviceIDs := make([]DeviceID, numDevices)
	errors := make([]error, numDevices)

	for i := 0; i < numDevices; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			desc := &types.DeviceDescriptor{
				Label:          "Concurrent Device",
				RequiredLimits: types.DefaultLimits(),
			}
			deviceIDs[idx], errors[idx] = CreateDevice(adapterID, desc)
		}(i)
	}

	wg.Wait()

	// Verify all devices were created
	for i, err := range errors {
		if err != nil {
			t.Errorf("Device %d creation failed: %v", i, err)
		}
	}

	// Verify all devices can be accessed
	for i, deviceID := range deviceIDs {
		_, err := GetDevice(deviceID)
		if err != nil {
			t.Errorf("Device %d access failed: %v", i, err)
		}
	}
}

// Helper function to create a test adapter
func createTestAdapter(t *testing.T, features types.Features, limits types.Limits) AdapterID {
	t.Helper()
	hub := GetGlobal().Hub()
	adapter := &Adapter{
		Info: types.AdapterInfo{
			Name:       "Test Adapter",
			Vendor:     "Test",
			DeviceType: types.DeviceTypeDiscreteGPU,
			Backend:    types.BackendVulkan,
		},
		Features: features,
		Limits:   limits,
		Backend:  types.BackendVulkan,
	}
	return hub.RegisterAdapter(adapter)
}
