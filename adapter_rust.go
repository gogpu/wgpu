//go:build rust

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures Features
	RequiredLimits   Limits
}

// Adapter represents a physical GPU.
// On Rust backend, this wraps go-webgpu/webgpu Adapter.
type Adapter struct {
	r        *rwgpu.Adapter
	info     AdapterInfo
	features Features
	limits   Limits
	instance *Instance
	released bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() AdapterInfo { return a.info }

// Features returns supported features.
func (a *Adapter) Features() Features { return a.features }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() Limits { return a.limits }

// RequestDevice creates a logical device from this adapter.
// If desc is nil, default features and limits are used.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
	if a.released {
		return nil, ErrReleased
	}

	var rDesc *rwgpu.DeviceDescriptor
	if desc != nil {
		rDesc = &rwgpu.DeviceDescriptor{
			Label: desc.Label,
		}
		// Limits conversion: if user specified limits, convert them.
		if desc.RequiredLimits != (gputypes.Limits{}) {
			rl := convertToRustLimits(desc.RequiredLimits)
			rDesc.RequiredLimits = &rl
		}
	}

	rd, err := a.r.RequestDevice(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to request device: %w", err)
	}

	deviceFeatures := convertRustFeatures(rd.Features())
	deviceLimits := convertRustLimits(rd.Limits())

	rq := rd.Queue()

	return &Device{
		r:        rd,
		instance: a.instance,
		queue:    &Queue{r: rq},
		features: deviceFeatures,
		limits:   deviceLimits,
	}, nil
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	Formats      []TextureFormat
	PresentModes []PresentMode
	AlphaModes   []CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	if a.released || surface == nil || surface.r == nil {
		return nil
	}

	caps, err := surface.r.GetCapabilities(a.r)
	if err != nil {
		return nil
	}

	return &SurfaceCapabilities{
		Formats:      caps.Formats,
		PresentModes: caps.PresentModes,
		AlphaModes:   caps.AlphaModes,
	}
}

// Release releases the adapter.
func (a *Adapter) Release() {
	if a.released {
		return
	}
	a.released = true
	if a.r != nil {
		a.r.Release()
	}
}

// convertToRustLimits converts gputypes.Limits to go-webgpu Limits.
//
//nolint:dupl // Symmetric field-by-field mapping (rwgpu→gputypes vs gputypes→rwgpu), not real duplication.
func convertToRustLimits(gl gputypes.Limits) rwgpu.Limits {
	return rwgpu.Limits{
		MaxTextureDimension1D:                     gl.MaxTextureDimension1D,
		MaxTextureDimension2D:                     gl.MaxTextureDimension2D,
		MaxTextureDimension3D:                     gl.MaxTextureDimension3D,
		MaxTextureArrayLayers:                     gl.MaxTextureArrayLayers,
		MaxBindGroups:                             gl.MaxBindGroups,
		MaxBindingsPerBindGroup:                   gl.MaxBindingsPerBindGroup,
		MaxDynamicUniformBuffersPerPipelineLayout: gl.MaxDynamicUniformBuffersPerPipelineLayout,
		MaxDynamicStorageBuffersPerPipelineLayout: gl.MaxDynamicStorageBuffersPerPipelineLayout,
		MaxSampledTexturesPerShaderStage:          gl.MaxSampledTexturesPerShaderStage,
		MaxSamplersPerShaderStage:                 gl.MaxSamplersPerShaderStage,
		MaxStorageBuffersPerShaderStage:           gl.MaxStorageBuffersPerShaderStage,
		MaxStorageTexturesPerShaderStage:          gl.MaxStorageTexturesPerShaderStage,
		MaxUniformBuffersPerShaderStage:           gl.MaxUniformBuffersPerShaderStage,
		MaxUniformBufferBindingSize:               gl.MaxUniformBufferBindingSize,
		MaxStorageBufferBindingSize:               gl.MaxStorageBufferBindingSize,
		MinUniformBufferOffsetAlignment:           gl.MinUniformBufferOffsetAlignment,
		MinStorageBufferOffsetAlignment:           gl.MinStorageBufferOffsetAlignment,
		MaxVertexBuffers:                          gl.MaxVertexBuffers,
		MaxBufferSize:                             gl.MaxBufferSize,
		MaxVertexAttributes:                       gl.MaxVertexAttributes,
		MaxVertexBufferArrayStride:                gl.MaxVertexBufferArrayStride,
		MaxComputeWorkgroupSizeX:                  gl.MaxComputeWorkgroupSizeX,
		MaxComputeWorkgroupSizeY:                  gl.MaxComputeWorkgroupSizeY,
		MaxComputeWorkgroupSizeZ:                  gl.MaxComputeWorkgroupSizeZ,
		MaxComputeWorkgroupsPerDimension:          gl.MaxComputeWorkgroupsPerDimension,
		MaxComputeInvocationsPerWorkgroup:         gl.MaxComputeInvocationsPerWorkgroup,
	}
}
