// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Backend implements hal.Backend for Metal.
type Backend struct{}

// NewBackend returns a Metal backend instance.
func NewBackend() Backend { return Backend{} }

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendMetal
}

// CreateInstance creates a new Metal instance.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	if err := Init(); err != nil {
		return nil, fmt.Errorf("metal: failed to initialize: %w", err)
	}
	hal.Logger().Info("metal: instance created")
	return &Instance{}, nil
}

// Instance implements hal.Instance for Metal.
type Instance struct{}

// CreateSurface creates a rendering surface from a CAMetalLayer target.
func (i *Instance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	if err := target.RequireKind(hal.SurfaceTargetMetalLayer); err != nil {
		return nil, fmt.Errorf("metal: %w", err)
	}
	layer := ID(target.WindowHandle)
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

	hal.Logger().Debug("metal: enumerating adapters", "count", len(devices))

	adapters := make([]hal.ExposedAdapter, 0, len(devices))
	for _, device := range devices {
		deviceName := DeviceName(device)

		// Determine device type
		var deviceType gputypes.DeviceType
		switch {
		case DeviceIsHeadless(device):
			deviceType = gputypes.DeviceTypeOther
		case DeviceIsRemovable(device), !DeviceIsLowPower(device):
			deviceType = gputypes.DeviceTypeDiscreteGPU
		default:
			deviceType = gputypes.DeviceTypeIntegratedGPU
		}

		// Build features
		var features gputypes.Features
		if DeviceSupportsFamily(device, MTLGPUFamilyMetal3) {
			features.Insert(gputypes.FeatureTimestampQuery)
		}
		features.Insert(gputypes.FeatureDepthClipControl)
		features.Insert(gputypes.FeatureTextureCompressionBC)

		adapter := &Adapter{
			instance:              i,
			raw:                   device,
			formatDepth24Stencil8: MsgSendBool(device, Sel("isDepth24Stencil8PixelFormatSupported")),
		}

		maxBuf := DeviceMaxBufferLength(device)

		hal.Logger().Info("metal: adapter found",
			"name", deviceName,
			"type", deviceType,
			"lowPower", DeviceIsLowPower(device),
			"removable", DeviceIsRemovable(device),
			"headless", DeviceIsHeadless(device),
			"maxBuffer", maxBuf,
			"depth24Stencil8", adapter.formatDepth24Stencil8,
		)

		adapters = append(adapters, hal.ExposedAdapter{
			Adapter: adapter,
			Info: gputypes.AdapterInfo{
				Name:       deviceName,
				Vendor:     "Apple",
				VendorID:   0x106b, // Apple Inc.
				DeviceID:   uint32(DeviceRegistryID(device) & 0xFFFFFFFF),
				DeviceType: deviceType,
				Driver:     "Metal",
				DriverInfo: "Metal API",
				Backend:    gputypes.BackendMetal,
			},
			Features: features,
			Capabilities: hal.Capabilities{
				Limits: gputypes.Limits{
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
					MaxVertexBuffers:                          maxVertexBuffers,
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
				// DefaultDownlevelCapabilities (all 27 flags) is correct for Metal.
				// Rust wgpu-hal Metal conditionally sets 8 flags from feature sets
				// (adapter.rs:1337-1371), but ALL of those checks pass on our minimum
				// targets (macOS 15.0+ / iOS 18.0+):
				//   FRAGMENT_WRITABLE_STORAGE — macOS 10.12+ (available!(macos=10.12))
				//   CUBE_ARRAY_TEXTURES       — macOS_GPUFamily1_v1 (macOS 10.11+)
				//   COMPARISON_SAMPLERS        — macOS_GPUFamily1_v1 (macOS 10.11+)
				//   INDIRECT_EXECUTION         — macOS_GPUFamily1_v1 (macOS 10.11+)
				//   BASE_VERTEX               — same as INDIRECT_EXECUTION
				//   ANISOTROPIC_FILTERING      — always true in Rust
				//   MSL2_1                    — MSL 2.1 requires macOS 10.14+
				//   TEXTURE_COMPRESSION        — macOS always has BC; iOS has EAC+ASTC
				DownlevelCapabilities: gputypes.DefaultDownlevelCapabilities(),
			},
		})
	}

	return adapters
}

// Destroy releases the instance.
func (i *Instance) Destroy() {
	// Nothing to release
}
