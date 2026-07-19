//go:build rust

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// InstanceDescriptor configures instance creation.
// On Rust backend, Backends and Flags are accepted for API compatibility
// but the Rust wgpu-native handles backend selection internally.
type InstanceDescriptor struct {
	Backends Backends
	Flags    gputypes.InstanceFlags
}

// Instance is the entry point for GPU operations.
// On Rust backend, this wraps go-webgpu/webgpu Instance.
type Instance struct {
	r        *rwgpu.Instance
	released bool
}

// CreateInstance creates a new GPU instance.
// If desc is nil, all available backends are used.
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	if err := rwgpu.Init(); err != nil {
		return nil, fmt.Errorf("wgpu: failed to init wgpu-native: %w", err)
	}

	ri, err := rwgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create instance: %w", err)
	}

	return &Instance{r: ri}, nil
}

// RequestAdapter requests a GPU adapter matching the options.
// If opts is nil, the best available adapter is returned.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	if i.released {
		return nil, ErrReleased
	}

	var rOpts *rwgpu.RequestAdapterOptions
	if opts != nil {
		rOpts = &rwgpu.RequestAdapterOptions{
			PowerPreference:      opts.PowerPreference,
			ForceFallbackAdapter: opts.ForceFallbackAdapter,
		}
		if opts.CompatibleSurface != nil {
			rOpts.CompatibleSurface = opts.CompatibleSurface.r
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
	info := AdapterInfo{
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
	if i.released {
		return
	}
	i.released = true
	if i.r != nil {
		i.r.Release()
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
