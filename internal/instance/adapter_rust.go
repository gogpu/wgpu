//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file is the rust implementation of the adapter domain (ADR-070 S12),
// moved from the root package's adapter_rust.go. The root keeps the public
// Adapter declaration layer, converts the device descriptor and wraps the
// device implementation this returns.

import (
	"fmt"

	"github.com/gogpu/gputypes"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/surface"
)

// Adapter is the rust-build implementation of a physical GPU.
type Adapter struct {
	r        *rwgpu.Adapter
	info     gputypes.AdapterInfo
	features gputypes.Features
	limits   gputypes.Limits
	instance *Instance
	released bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() gputypes.AdapterInfo {
	if a == nil {
		return gputypes.AdapterInfo{}
	}
	return a.info
}

// Features returns supported features.
func (a *Adapter) Features() gputypes.Features {
	if a == nil {
		return 0
	}
	return a.features
}

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() gputypes.Limits {
	if a == nil {
		return gputypes.Limits{}
	}
	return a.limits
}

// DownlevelCapabilities returns backend capability flags for downlevel adapters.
// Rust wgpu handles real values internally; from Go FFI these are the fully
// compliant defaults.
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	return gputypes.DefaultDownlevelCapabilities()
}

// DeviceDescriptor is the implementation's mirror of the public descriptor: the
// declaration layer converts, and a nil pointer is the public nil.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// RequestDevice creates a logical device from this adapter and returns its
// implementation and queue. The device's InstanceHost is wired to this adapter's
// instance before it is returned.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*device.Device, *queue.Queue, error) {
	if a == nil || a.released || a.instance == nil || a.instance.IsReleased() {
		return nil, nil, core.ErrReleased
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
		return nil, nil, fmt.Errorf("wgpu: failed to request device: %w", err)
	}

	deviceFeatures := convertRustFeatures(rd.Features())
	deviceLimits := convertRustLimits(rd.Limits())

	rq := rd.Queue()

	impl := device.NewRust(rd, deviceFeatures, deviceLimits)
	impl.SetInstance(a.instance)

	return impl, queue.New(rq), nil
}

// SurfaceCapabilities mirrors the public capability answer.
type SurfaceCapabilities struct {
	Formats      []gputypes.TextureFormat
	PresentModes []gputypes.PresentMode
	AlphaModes   []gputypes.CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
func (a *Adapter) GetSurfaceCapabilities(s *surface.Surface) *SurfaceCapabilities {
	if a == nil || a.released || s == nil {
		return nil
	}
	rSurface := s.R()
	if rSurface == nil {
		return nil
	}

	caps, err := rSurface.GetCapabilities(a.r)
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
	if a == nil || a.released {
		return
	}
	a.released = true
	if a.r != nil {
		a.r.Release()
	}
}

// HAL is the rust-build body of the public Adapter.HAL.
func (a *Adapter) HAL() publichal.Handles {
	// wgpu-native's WGPUAdapterImpl owns its own Arc<Context>, so the adapter
	// handle stays valid after the instance is released. The zero Handles after
	// either release is the contract the public method promises, consistent with
	// Instance.HAL(), not a lifetime requirement (ADR-070 §15.5).
	if a == nil || a.released || a.r == nil || (a.instance != nil && a.instance.IsReleased()) {
		return publichal.Handles{}
	}
	h := publichal.WebGPUHandles{Adapter: a.r.Handle()}
	if a.instance != nil && a.instance.r != nil {
		h.Instance = a.instance.r.Handle()
	}
	return publichal.Handles{Kind: publichal.KindWebGPU, WebGPU: h}
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
