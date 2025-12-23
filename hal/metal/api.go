// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// Backend implements hal.Backend for Metal.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() types.Backend {
	return types.BackendMetal
}

// CreateInstance creates a new Metal instance.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	if err := Init(); err != nil {
		return nil, fmt.Errorf("metal: failed to initialize: %w", err)
	}
	return &Instance{}, nil
}

// Instance implements hal.Instance for Metal.
type Instance struct{}

// CreateSurface creates a rendering surface from platform handles.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (hal.Surface, error) {
	// On macOS, windowHandle is typically NSView* or CAMetalLayer*
	// We need to get or create a CAMetalLayer from the view
	layer := ID(windowHandle)
	if layer == 0 {
		return nil, fmt.Errorf("metal: window handle is nil")
	}
	Retain(layer)
	return &Surface{layer: layer}, nil
}

// EnumerateAdapters returns available Metal adapters (devices).
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	devices := CopyAllDevices()
	if len(devices) == 0 {
		return nil
	}

	adapters := make([]hal.ExposedAdapter, 0, len(devices))
	for _, device := range devices {
		deviceName := DeviceName(device)

		// Determine device type
		deviceType := types.DeviceTypeIntegratedGPU
		if DeviceIsHeadless(device) {
			deviceType = types.DeviceTypeOther
		} else if DeviceIsRemovable(device) {
			deviceType = types.DeviceTypeDiscreteGPU
		} else if !DeviceIsLowPower(device) {
			deviceType = types.DeviceTypeDiscreteGPU
		}

		// Build features
		var features types.Features
		if DeviceSupportsFamily(device, MTLGPUFamilyMetal3) {
			features.Insert(types.FeatureTimestampQuery)
		}
		features.Insert(types.FeatureDepthClipControl)
		features.Insert(types.FeatureTextureCompressionBC)

		adapter := &Adapter{
			instance: i,
			raw:      device,
		}

		maxBuf := DeviceMaxBufferLength(device)

		adapters = append(adapters, hal.ExposedAdapter{
			Adapter: adapter,
			Info: types.AdapterInfo{
				Name:       deviceName,
				Vendor:     "Apple",
				VendorID:   0x106b, // Apple Inc.
				DeviceID:   uint32(DeviceRegistryID(device) & 0xFFFFFFFF),
				DeviceType: deviceType,
				Driver:     "Metal",
				DriverInfo: "Metal API",
				Backend:    types.BackendMetal,
			},
			Features: features,
			Capabilities: hal.Capabilities{
				Limits: types.Limits{
					MaxTextureDimension1D:                     16384,
					MaxTextureDimension2D:                     16384,
					MaxTextureDimension3D:                     2048,
					MaxTextureArrayLayers:                     2048,
					MaxBindGroups:                             4,
					MaxBindGroupsPlusVertexBuffers:            24,
					MaxBindingsPerBindGroup:                   1000,
					MaxDynamicUniformBuffersPerPipelineLayout: 12,
					MaxDynamicStorageBuffersPerPipelineLayout: 4,
					MaxSampledTexturesPerShaderStage:          128,
					MaxSamplersPerShaderStage:                 16,
					MaxStorageBuffersPerShaderStage:           8,
					MaxStorageTexturesPerShaderStage:          8,
					MaxUniformBuffersPerShaderStage:           12,
					MaxUniformBufferBindingSize:               maxBuf,
					MaxStorageBufferBindingSize:               maxBuf,
					MinUniformBufferOffsetAlignment:           256,
					MinStorageBufferOffsetAlignment:           256,
					MaxVertexBuffers:                          30,
					MaxBufferSize:                             maxBuf,
					MaxVertexAttributes:                       31,
					MaxVertexBufferArrayStride:                2048,

					MaxInterStageShaderVariables:      60,
					MaxColorAttachments:               8,
					MaxColorAttachmentBytesPerSample:  128,
					MaxComputeWorkgroupStorageSize:    32768,
					MaxComputeInvocationsPerWorkgroup: 1024,
					MaxComputeWorkgroupSizeX:          1024,
					MaxComputeWorkgroupSizeY:          1024,
					MaxComputeWorkgroupSizeZ:          1024,
					MaxComputeWorkgroupsPerDimension:  65535,
				},
				AlignmentsMask: hal.Alignments{
					BufferCopyOffset: 4,
					BufferCopyPitch:  256,
				},
				DownlevelCapabilities: hal.DownlevelCapabilities{
					ShaderModel: 60,
					Flags:       0,
				},
			},
		})
	}

	return adapters
}

// Destroy releases the instance.
func (i *Instance) Destroy() {
	// Nothing to release
}
