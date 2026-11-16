//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file is the rust implementation of the instance domain (ADR-070 S12),
// moved from the root package's instance_rust.go. The root keeps the public
// Instance declaration layer. The instance is the device's InstanceHost on this
// build: its handle is what the device's error scopes read (ADR-070 §15.9).

import (
	"fmt"

	"github.com/gogpu/gputypes"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/surface"
)

// Instance is the rust-build implementation of the GPU instance.
type Instance struct {
	r        *rwgpu.Instance
	released bool
}

// Create initializes wgpu-native and creates the instance.
func Create() (*Instance, error) {
	if err := rwgpu.Init(); err != nil {
		return nil, fmt.Errorf("wgpu: failed to init wgpu-native: %w", err)
	}
	ri, err := rwgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create instance: %w", err)
	}
	return &Instance{r: ri}, nil
}

// InstanceR implements internal/device's InstanceHost: the wgpu-native instance
// the device's error scopes read, or nil when there is none. A nil receiver is
// the declaration layer's zero value.
func (i *Instance) InstanceR() *rwgpu.Instance {
	if i == nil {
		return nil
	}
	return i.r
}

// IsReleased reports whether the instance has been released. A nil receiver
// answers true.
func (i *Instance) IsReleased() bool {
	return i == nil || i.released
}

// RequestAdapter requests an adapter for the options the declaration layer
// converted (a nil pointer is the public nil) and the compatible surface
// implementation, or nil.
func (i *Instance) RequestAdapter(opts *gputypes.RequestAdapterOptions, compatible *surface.Surface) (*Adapter, error) {
	if i.released {
		return nil, core.ErrReleased
	}

	var rOpts *rwgpu.RequestAdapterOptions
	if opts != nil {
		rOpts = &rwgpu.RequestAdapterOptions{
			PowerPreference:      opts.PowerPreference,
			ForceFallbackAdapter: opts.ForceFallbackAdapter,
		}
		if compatible != nil {
			rOpts.CompatibleSurface = compatible.R()
		}
	}

	ra, err := i.r.RequestAdapter(rOpts)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to request adapter: %w", err)
	}

	// Convert adapter info.
	rInfo, err := ra.Info()
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter info: %w", err)
	}
	info := gputypes.AdapterInfo{
		Name:       rInfo.Description,
		Vendor:     rInfo.Vendor,
		VendorID:   rInfo.VendorID,
		DeviceID:   rInfo.DeviceID,
		Driver:     rInfo.Device,
		DriverInfo: rInfo.Architecture,
		Backend:    convertRustBackendType(rInfo.BackendType),
		DeviceType: convertRustAdapterType(rInfo.AdapterType),
	}

	// Convert limits from rwgpu.Limits to gputypes.Limits.
	rLimits := ra.Limits()
	limits := convertRustLimits(rLimits)

	// Convert features from []FeatureName to Features bitmask.
	rFeatures := ra.Features()
	features := convertRustFeatures(rFeatures)

	return &Adapter{
		r:        ra,
		info:     info,
		features: features,
		limits:   limits,
		instance: i,
	}, nil
}

// Release releases the instance. Surfaces must be released explicitly.
func (i *Instance) Release() {
	if i == nil || i.released {
		return
	}
	i.released = true
	if i.r != nil {
		i.r.Release()
	}
}

// HAL is the rust-build body of the public Instance.HAL.
func (i *Instance) HAL() publichal.Handles {
	if i == nil || i.released || i.r == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind:   publichal.KindWebGPU,
		WebGPU: publichal.WebGPUHandles{Instance: i.r.Handle()},
	}
}

// convertRustLimits converts go-webgpu Limits to gputypes.Limits.
//
//nolint:dupl // Symmetric field-by-field mapping (rwgpu→gputypes vs gputypes→rwgpu), not real duplication.
func convertRustLimits(rl rwgpu.Limits) gputypes.Limits {
	return gputypes.Limits{
		MaxTextureDimension1D:                     rl.MaxTextureDimension1D,
		MaxTextureDimension2D:                     rl.MaxTextureDimension2D,
		MaxTextureDimension3D:                     rl.MaxTextureDimension3D,
		MaxTextureArrayLayers:                     rl.MaxTextureArrayLayers,
		MaxBindGroups:                             rl.MaxBindGroups,
		MaxBindingsPerBindGroup:                   rl.MaxBindingsPerBindGroup,
		MaxDynamicUniformBuffersPerPipelineLayout: rl.MaxDynamicUniformBuffersPerPipelineLayout,
		MaxDynamicStorageBuffersPerPipelineLayout: rl.MaxDynamicStorageBuffersPerPipelineLayout,
		MaxSampledTexturesPerShaderStage:          rl.MaxSampledTexturesPerShaderStage,
		MaxSamplersPerShaderStage:                 rl.MaxSamplersPerShaderStage,
		MaxStorageBuffersPerShaderStage:           rl.MaxStorageBuffersPerShaderStage,
		MaxStorageTexturesPerShaderStage:          rl.MaxStorageTexturesPerShaderStage,
		MaxUniformBuffersPerShaderStage:           rl.MaxUniformBuffersPerShaderStage,
		MaxUniformBufferBindingSize:               rl.MaxUniformBufferBindingSize,
		MaxStorageBufferBindingSize:               rl.MaxStorageBufferBindingSize,
		MinUniformBufferOffsetAlignment:           rl.MinUniformBufferOffsetAlignment,
		MinStorageBufferOffsetAlignment:           rl.MinStorageBufferOffsetAlignment,
		MaxVertexBuffers:                          rl.MaxVertexBuffers,
		MaxBufferSize:                             rl.MaxBufferSize,
		MaxVertexAttributes:                       rl.MaxVertexAttributes,
		MaxVertexBufferArrayStride:                rl.MaxVertexBufferArrayStride,
		MaxComputeWorkgroupSizeX:                  rl.MaxComputeWorkgroupSizeX,
		MaxComputeWorkgroupSizeY:                  rl.MaxComputeWorkgroupSizeY,
		MaxComputeWorkgroupSizeZ:                  rl.MaxComputeWorkgroupSizeZ,
		MaxComputeWorkgroupsPerDimension:          rl.MaxComputeWorkgroupsPerDimension,
		MaxComputeInvocationsPerWorkgroup:         rl.MaxComputeInvocationsPerWorkgroup,
	}
}

// convertRustFeatures converts go-webgpu []FeatureName to gputypes.Features bitmask.
// Maps each go-webgpu FeatureName constant to the corresponding gputypes.Feature bit.
// Unrecognized features are silently ignored (wgpu-native extensions not in our bitmask).
func convertRustFeatures(names []rwgpu.FeatureName) gputypes.Features {
	var features gputypes.Features
	for _, name := range names {
		if f, ok := rustFeatureMap[name]; ok {
			features |= gputypes.Features(f)
		}
	}
	return features
}

// rustFeatureMap maps go-webgpu FeatureName constants to gputypes.Feature bitmask bits.
// Only features that exist in both go-webgpu and gputypes are included.
var rustFeatureMap = map[rwgpu.FeatureName]gputypes.Feature{
	rwgpu.FeatureNameDepthClipControl:        gputypes.FeatureDepthClipControl,
	rwgpu.FeatureNameDepth32FloatStencil8:    gputypes.FeatureDepth32FloatStencil8,
	rwgpu.FeatureNameTextureCompressionBC:    gputypes.FeatureTextureCompressionBC,
	rwgpu.FeatureNameTextureCompressionETC2:  gputypes.FeatureTextureCompressionETC2,
	rwgpu.FeatureNameTextureCompressionASTC:  gputypes.FeatureTextureCompressionASTC,
	rwgpu.FeatureNameIndirectFirstInstance:   gputypes.FeatureIndirectFirstInstance,
	rwgpu.FeatureNameShaderF16:               gputypes.FeatureShaderF16,
	rwgpu.FeatureNameRG11B10UfloatRenderable: gputypes.FeatureRG11B10UfloatRenderable,
	rwgpu.FeatureNameBGRA8UnormStorage:       gputypes.FeatureBGRA8UnormStorage,
	rwgpu.FeatureNameFloat32Filterable:       gputypes.FeatureFloat32Filterable,
	rwgpu.FeatureNameTimestampQuery:          gputypes.FeatureTimestampQuery,
	rwgpu.FeatureNameSubgroups:               gputypes.FeatureSubgroupOperations,
}

// convertRustBackendType maps go-webgpu BackendType to gputypes.Backend.
func convertRustBackendType(bt rwgpu.BackendType) gputypes.Backend {
	switch bt {
	case rwgpu.BackendTypeVulkan:
		return gputypes.BackendVulkan
	case rwgpu.BackendTypeMetal:
		return gputypes.BackendMetal
	case rwgpu.BackendTypeD3D12:
		return gputypes.BackendDX12
	case rwgpu.BackendTypeOpenGL, rwgpu.BackendTypeOpenGLES:
		return gputypes.BackendGL
	default:
		return gputypes.BackendEmpty
	}
}

// convertRustAdapterType maps go-webgpu AdapterType to gputypes.DeviceType.
func convertRustAdapterType(at rwgpu.AdapterType) gputypes.DeviceType {
	switch at {
	case rwgpu.AdapterTypeDiscreteGPU:
		return gputypes.DeviceTypeDiscreteGPU
	case rwgpu.AdapterTypeIntegratedGPU:
		return gputypes.DeviceTypeIntegratedGPU
	case rwgpu.AdapterTypeCPU:
		return gputypes.DeviceTypeCPU
	default:
		return gputypes.DeviceTypeOther
	}
}
