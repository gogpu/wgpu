// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

// Adapter implements hal.Adapter for DirectX 12.
type Adapter struct {
	raw          *dxgi.IDXGIAdapter4
	desc         dxgi.DXGI_ADAPTER_DESC1
	instance     *Instance
	capabilities AdapterCapabilities
}

// AdapterCapabilities contains detected adapter capabilities.
type AdapterCapabilities struct {
	// FeatureLevel is the highest supported D3D feature level.
	FeatureLevel d3d12.D3D_FEATURE_LEVEL

	// ShaderModel is the highest supported shader model.
	ShaderModel d3d12.D3D_SHADER_MODEL

	// ResourceBindingTier indicates resource binding capabilities.
	ResourceBindingTier uint32

	// TiledResourcesTier indicates tiled resources support.
	TiledResourcesTier uint32

	// MaxTexture2D is the maximum 2D texture dimension.
	MaxTexture2D uint32

	// MaxTexture3D is the maximum 3D texture dimension.
	MaxTexture3D uint32

	// MaxTextureCube is the maximum cube texture dimension.
	MaxTextureCube uint32

	// TimestampFrequency is the GPU timestamp frequency in Hz.
	TimestampFrequency uint64

	// SupportsTypedUAVLoadAdditionalFormats indicates extended UAV format support.
	SupportsTypedUAVLoadAdditionalFormats bool

	// SupportsROVs indicates rasterizer ordered views support.
	SupportsROVs bool
}

// probeCapabilities probes the adapter's capabilities by creating a temporary device.
func (a *Adapter) probeCapabilities() error {
	// Feature levels to test, from highest to lowest
	featureLevels := []d3d12.D3D_FEATURE_LEVEL{
		d3d12.D3D_FEATURE_LEVEL_12_2,
		d3d12.D3D_FEATURE_LEVEL_12_1,
		d3d12.D3D_FEATURE_LEVEL_12_0,
		d3d12.D3D_FEATURE_LEVEL_11_1,
		d3d12.D3D_FEATURE_LEVEL_11_0,
	}

	var tempDevice *d3d12.ID3D12Device
	for _, level := range featureLevels {
		dev, err := a.instance.d3d12Lib.CreateDevice(
			unsafe.Pointer(a.raw),
			level,
		)
		if err == nil {
			tempDevice = dev
			a.capabilities.FeatureLevel = level
			break
		}
	}

	if tempDevice == nil {
		return fmt.Errorf("dx12: no supported feature level found for adapter")
	}
	defer tempDevice.Release()

	// Query shader model support
	a.queryShaderModel(tempDevice)

	// Query D3D12 options
	a.queryD3D12Options(tempDevice)

	// Set default texture limits based on feature level
	a.setTextureLimits()

	return nil
}

// queryShaderModel queries the highest supported shader model.
func (a *Adapter) queryShaderModel(device *d3d12.ID3D12Device) {
	// Start with the highest shader model and work down
	shaderModels := []d3d12.D3D_SHADER_MODEL{
		d3d12.D3D_SHADER_MODEL_6_7,
		d3d12.D3D_SHADER_MODEL_6_6,
		d3d12.D3D_SHADER_MODEL_6_5,
		d3d12.D3D_SHADER_MODEL_6_4,
		d3d12.D3D_SHADER_MODEL_6_3,
		d3d12.D3D_SHADER_MODEL_6_2,
		d3d12.D3D_SHADER_MODEL_6_1,
		d3d12.D3D_SHADER_MODEL_6_0,
		d3d12.D3D_SHADER_MODEL_5_1,
	}

	for _, sm := range shaderModels {
		featureData := d3d12.D3D12_FEATURE_DATA_SHADER_MODEL{
			HighestShaderModel: sm,
		}
		err := device.CheckFeatureSupport(
			d3d12.D3D12_FEATURE_SHADER_MODEL,
			unsafe.Pointer(&featureData),
			uint32(unsafe.Sizeof(featureData)),
		)
		if err == nil {
			a.capabilities.ShaderModel = featureData.HighestShaderModel
			return
		}
	}

	// Default to SM 5.1 if all checks fail
	a.capabilities.ShaderModel = d3d12.D3D_SHADER_MODEL_5_1
}

// queryD3D12Options queries D3D12 feature options.
func (a *Adapter) queryD3D12Options(device *d3d12.ID3D12Device) {
	var options d3d12.D3D12_FEATURE_DATA_D3D12_OPTIONS

	err := device.CheckFeatureSupport(
		d3d12.D3D12_FEATURE_D3D12_OPTIONS,
		unsafe.Pointer(&options),
		uint32(unsafe.Sizeof(options)),
	)
	if err != nil {
		// Use conservative defaults
		a.capabilities.ResourceBindingTier = 1
		a.capabilities.TiledResourcesTier = 0
		return
	}

	a.capabilities.ResourceBindingTier = options.ResourceBindingTier
	a.capabilities.TiledResourcesTier = options.TiledResourcesTier
	a.capabilities.SupportsTypedUAVLoadAdditionalFormats = options.TypedUAVLoadAdditionalFormats != 0
	a.capabilities.SupportsROVs = options.ROVsSupported != 0
}

// setTextureLimits sets texture dimension limits based on feature level.
func (a *Adapter) setTextureLimits() {
	// D3D12 limits based on feature level
	// https://docs.microsoft.com/en-us/windows/win32/direct3d12/hardware-feature-levels
	switch a.capabilities.FeatureLevel {
	case d3d12.D3D_FEATURE_LEVEL_12_2,
		d3d12.D3D_FEATURE_LEVEL_12_1,
		d3d12.D3D_FEATURE_LEVEL_12_0:
		a.capabilities.MaxTexture2D = 16384
		a.capabilities.MaxTexture3D = 2048
		a.capabilities.MaxTextureCube = 16384
	case d3d12.D3D_FEATURE_LEVEL_11_1,
		d3d12.D3D_FEATURE_LEVEL_11_0:
		a.capabilities.MaxTexture2D = 16384
		a.capabilities.MaxTexture3D = 2048
		a.capabilities.MaxTextureCube = 16384
	default:
		a.capabilities.MaxTexture2D = 8192
		a.capabilities.MaxTexture3D = 2048
		a.capabilities.MaxTextureCube = 8192
	}
}

// toExposedAdapter converts the adapter to hal.ExposedAdapter.
func (a *Adapter) toExposedAdapter() hal.ExposedAdapter {
	return hal.ExposedAdapter{
		Adapter:      a,
		Info:         a.Info(),
		Features:     a.Features(),
		Capabilities: a.Capabilities(),
	}
}

// Info returns adapter information.
func (a *Adapter) Info() gputypes.AdapterInfo {
	return gputypes.AdapterInfo{
		Name:       utf16ToString(a.desc.Description[:]),
		Vendor:     vendorIDToName(a.desc.VendorID),
		VendorID:   a.desc.VendorID,
		DeviceID:   a.desc.DeviceID,
		DeviceType: a.deviceType(),
		Driver:     "DirectX 12",
		DriverInfo: featureLevelString(a.capabilities.FeatureLevel),
		Backend:    gputypes.BackendDX12,
	}
}

// Features returns supported WebGPU features.
func (a *Adapter) Features() gputypes.Features {
	var features gputypes.Features

	// Map D3D12 capabilities to WebGPU features
	// Feature level 11.0+ guarantees basic compute and texture compression
	if a.capabilities.FeatureLevel >= d3d12.D3D_FEATURE_LEVEL_11_0 {
		features |= gputypes.Features(gputypes.FeatureTextureCompressionBC)
	}

	// Feature level 12.0+ adds more advanced features
	if a.capabilities.FeatureLevel >= d3d12.D3D_FEATURE_LEVEL_12_0 {
		features |= gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	}

	// Shader model 6.0+ enables subgroups
	if a.capabilities.ShaderModel >= d3d12.D3D_SHADER_MODEL_6_0 {
		features |= gputypes.Features(gputypes.FeatureShaderF16)
	}

	return features
}

// Capabilities returns detailed adapter capabilities.
func (a *Adapter) Capabilities() hal.Capabilities {
	return hal.Capabilities{
		Limits: a.limits(),
		AlignmentsMask: hal.Alignments{
			BufferCopyOffset: 512, // D3D12_TEXTURE_DATA_PLACEMENT_ALIGNMENT
			BufferCopyPitch:  256, // D3D12_TEXTURE_DATA_PITCH_ALIGNMENT
		},
		DownlevelCapabilities: hal.DownlevelCapabilities{
			ShaderModel: uint32(a.capabilities.ShaderModel),
			Flags:       hal.DownlevelFlagsComputeShaders | hal.DownlevelFlagsAnisotropicFiltering,
		},
	}
}

// limits returns WebGPU-style limits based on D3D12 capabilities.
func (a *Adapter) limits() gputypes.Limits {
	limits := gputypes.DefaultLimits()

	// Override with D3D12-specific limits
	limits.MaxTextureDimension2D = a.capabilities.MaxTexture2D
	limits.MaxTextureDimension3D = a.capabilities.MaxTexture3D

	// D3D12 specific limits
	limits.MaxBindGroups = 4 // D3D12 has 4 root signature slots for descriptor tables
	limits.MaxSampledTexturesPerShaderStage = 128
	limits.MaxSamplersPerShaderStage = 16
	limits.MaxStorageBuffersPerShaderStage = 64
	limits.MaxStorageTexturesPerShaderStage = 64
	limits.MaxUniformBuffersPerShaderStage = 14 // D3D12_COMMONSHADER_CONSTANT_BUFFER_API_SLOT_COUNT

	// Buffer limits
	limits.MaxBufferSize = 128 * 1024 * 1024 * 1024 // 128 GB (virtual address space)
	limits.MaxUniformBufferBindingSize = 65536      // 64 KB per CBV

	// Compute limits
	limits.MaxComputeWorkgroupStorageSize = 32768 // 32 KB shared memory
	limits.MaxComputeInvocationsPerWorkgroup = 1024
	limits.MaxComputeWorkgroupSizeX = 1024
	limits.MaxComputeWorkgroupSizeY = 1024
	limits.MaxComputeWorkgroupSizeZ = 64
	limits.MaxComputeWorkgroupsPerDimension = 65535

	return limits
}

// deviceType determines the device type from the adapter flags and dedicated memory.
func (a *Adapter) deviceType() gputypes.DeviceType {
	// Check for software adapter (WARP)
	if a.desc.Flags&dxgi.DXGI_ADAPTER_FLAG_SOFTWARE != 0 {
		return gputypes.DeviceTypeCPU
	}

	// Check for dedicated video memory to distinguish discrete from integrated
	if a.desc.DedicatedVideoMemory > 0 {
		// Heuristic: >512MB dedicated VRAM is likely discrete
		if a.desc.DedicatedVideoMemory > 512*1024*1024 {
			return gputypes.DeviceTypeDiscreteGPU
		}
	}

	// If there's no dedicated video memory, it's likely integrated
	if a.desc.DedicatedVideoMemory == 0 && a.desc.SharedSystemMemory > 0 {
		return gputypes.DeviceTypeIntegratedGPU
	}

	// Assume discrete if there's any dedicated memory
	if a.desc.DedicatedVideoMemory > 0 {
		return gputypes.DeviceTypeDiscreteGPU
	}

	return gputypes.DeviceTypeOther
}

// Open opens a logical device with the requested features and limits.
func (a *Adapter) Open(features gputypes.Features, limits gputypes.Limits) (hal.OpenDevice, error) {
	// Validate that the adapter supports the requested features
	supported := a.Features()
	if features&^supported != 0 {
		return hal.OpenDevice{}, fmt.Errorf("dx12: adapter does not support requested features")
	}

	// Create device using the adapter
	device, err := newDevice(a.instance, unsafe.Pointer(a.raw), a.capabilities.FeatureLevel)
	if err != nil {
		return hal.OpenDevice{}, err
	}

	// Create queue wrapper
	queue := newQueue(device)

	return hal.OpenDevice{
		Device: device,
		Queue:  queue,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a specific texture format.
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	// Note: CheckFormatSupport can query exact format capabilities per resource type.
	// For now, return common capabilities for well-supported formats
	flags := hal.TextureFormatCapabilitySampled

	switch format {
	case gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGBA32Float:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve

	case gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityMultisample
	}

	return hal.TextureFormatCapabilities{
		Flags: flags,
	}
}

// SurfaceCapabilities returns surface capabilities.
func (a *Adapter) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	// D3D12 supports these formats for swap chains
	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
			gputypes.TextureFormatBGRA8UnormSrgb,
			gputypes.TextureFormatRGBA8UnormSrgb,
			gputypes.TextureFormatRGBA16Float,
		},
		PresentModes: a.presentModes(),
		AlphaModes: []hal.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
		},
	}
}

// presentModes returns supported present modes.
func (a *Adapter) presentModes() []hal.PresentMode {
	modes := []hal.PresentMode{
		hal.PresentModeFifo, // Always supported (vsync)
	}

	// Check if tearing is supported for immediate mode
	if a.instance.AllowsTearing() {
		modes = append(modes, hal.PresentModeImmediate)
	}

	// Mailbox is always available in DX12 with flip model
	modes = append(modes, hal.PresentModeMailbox)

	return modes
}

// Destroy releases the adapter.
func (a *Adapter) Destroy() {
	if a.raw != nil {
		a.raw.Release()
		a.raw = nil
	}
}

// Helper functions

// vendorIDToName converts a PCI vendor ID to a human-readable name.
func vendorIDToName(id uint32) string {
	switch id {
	case 0x1002:
		return "AMD"
	case 0x10DE:
		return "NVIDIA"
	case 0x8086:
		return "Intel"
	case 0x1414:
		return "Microsoft" // WARP
	case 0x1022:
		return "AMD" // Alternative AMD ID
	case 0x5143:
		return "Qualcomm"
	default:
		return fmt.Sprintf("0x%04X", id)
	}
}

// utf16ToString converts a UTF-16 encoded string (null-terminated) to Go string.
func utf16ToString(s []uint16) string {
	// Find null terminator
	n := 0
	for i, c := range s {
		if c == 0 {
			n = i
			break
		}
		n = i + 1
	}

	if n == 0 {
		return ""
	}

	// Convert UTF-16 to UTF-8
	runes := make([]rune, n)
	for i := 0; i < n; i++ {
		runes[i] = rune(s[i])
	}
	return string(runes)
}

// featureLevelString returns a human-readable feature level string.
func featureLevelString(level d3d12.D3D_FEATURE_LEVEL) string {
	switch level {
	case d3d12.D3D_FEATURE_LEVEL_12_2:
		return "Feature Level 12_2"
	case d3d12.D3D_FEATURE_LEVEL_12_1:
		return "Feature Level 12_1"
	case d3d12.D3D_FEATURE_LEVEL_12_0:
		return "Feature Level 12_0"
	case d3d12.D3D_FEATURE_LEVEL_11_1:
		return "Feature Level 11_1"
	case d3d12.D3D_FEATURE_LEVEL_11_0:
		return "Feature Level 11_0"
	default:
		return fmt.Sprintf("Feature Level 0x%X", level)
	}
}

// AdapterLegacy is used for adapters enumerated via the legacy API.
// It wraps IDXGIAdapter1 instead of IDXGIAdapter4.
type AdapterLegacy struct {
	raw          *dxgi.IDXGIAdapter1
	desc         dxgi.DXGI_ADAPTER_DESC1
	instance     *Instance
	capabilities AdapterCapabilities
}

// probeCapabilities probes the adapter's capabilities by creating a temporary device.
func (a *AdapterLegacy) probeCapabilities() error {
	featureLevels := []d3d12.D3D_FEATURE_LEVEL{
		d3d12.D3D_FEATURE_LEVEL_12_2,
		d3d12.D3D_FEATURE_LEVEL_12_1,
		d3d12.D3D_FEATURE_LEVEL_12_0,
		d3d12.D3D_FEATURE_LEVEL_11_1,
		d3d12.D3D_FEATURE_LEVEL_11_0,
	}

	var tempDevice *d3d12.ID3D12Device
	for _, level := range featureLevels {
		dev, err := a.instance.d3d12Lib.CreateDevice(
			unsafe.Pointer(a.raw),
			level,
		)
		if err == nil {
			tempDevice = dev
			a.capabilities.FeatureLevel = level
			break
		}
	}

	if tempDevice == nil {
		return fmt.Errorf("dx12: no supported feature level found for adapter")
	}
	defer tempDevice.Release()

	// Set default texture limits based on feature level
	a.setTextureLimits()

	return nil
}

// setTextureLimits sets texture dimension limits based on feature level.
func (a *AdapterLegacy) setTextureLimits() {
	switch a.capabilities.FeatureLevel {
	case d3d12.D3D_FEATURE_LEVEL_12_2,
		d3d12.D3D_FEATURE_LEVEL_12_1,
		d3d12.D3D_FEATURE_LEVEL_12_0:
		a.capabilities.MaxTexture2D = 16384
		a.capabilities.MaxTexture3D = 2048
		a.capabilities.MaxTextureCube = 16384
	case d3d12.D3D_FEATURE_LEVEL_11_1,
		d3d12.D3D_FEATURE_LEVEL_11_0:
		a.capabilities.MaxTexture2D = 16384
		a.capabilities.MaxTexture3D = 2048
		a.capabilities.MaxTextureCube = 16384
	default:
		a.capabilities.MaxTexture2D = 8192
		a.capabilities.MaxTexture3D = 2048
		a.capabilities.MaxTextureCube = 8192
	}
}

// toExposedAdapter converts the legacy adapter to hal.ExposedAdapter.
func (a *AdapterLegacy) toExposedAdapter() hal.ExposedAdapter {
	info := gputypes.AdapterInfo{
		Name:       utf16ToString(a.desc.Description[:]),
		Vendor:     vendorIDToName(a.desc.VendorID),
		VendorID:   a.desc.VendorID,
		DeviceID:   a.desc.DeviceID,
		DeviceType: a.deviceType(),
		Driver:     "DirectX 12",
		DriverInfo: featureLevelString(a.capabilities.FeatureLevel),
		Backend:    gputypes.BackendDX12,
	}

	return hal.ExposedAdapter{
		Adapter:      a,
		Info:         info,
		Features:     a.Features(),
		Capabilities: a.Capabilities(),
	}
}

// Features returns supported WebGPU features for legacy adapter.
func (a *AdapterLegacy) Features() gputypes.Features {
	var features gputypes.Features
	if a.capabilities.FeatureLevel >= d3d12.D3D_FEATURE_LEVEL_11_0 {
		features |= gputypes.Features(gputypes.FeatureTextureCompressionBC)
	}
	if a.capabilities.FeatureLevel >= d3d12.D3D_FEATURE_LEVEL_12_0 {
		features |= gputypes.Features(gputypes.FeatureDepth32FloatStencil8)
	}
	return features
}

// Capabilities returns detailed adapter capabilities.
func (a *AdapterLegacy) Capabilities() hal.Capabilities {
	limits := gputypes.DefaultLimits()
	limits.MaxTextureDimension2D = a.capabilities.MaxTexture2D
	limits.MaxTextureDimension3D = a.capabilities.MaxTexture3D

	return hal.Capabilities{
		Limits: limits,
		AlignmentsMask: hal.Alignments{
			BufferCopyOffset: 512,
			BufferCopyPitch:  256,
		},
		DownlevelCapabilities: hal.DownlevelCapabilities{
			ShaderModel: uint32(a.capabilities.ShaderModel),
			Flags:       hal.DownlevelFlagsComputeShaders | hal.DownlevelFlagsAnisotropicFiltering,
		},
	}
}

// deviceType determines the device type from the adapter flags and dedicated memory.
func (a *AdapterLegacy) deviceType() gputypes.DeviceType {
	if a.desc.Flags&dxgi.DXGI_ADAPTER_FLAG_SOFTWARE != 0 {
		return gputypes.DeviceTypeCPU
	}
	if a.desc.DedicatedVideoMemory > 512*1024*1024 {
		return gputypes.DeviceTypeDiscreteGPU
	}
	if a.desc.DedicatedVideoMemory == 0 && a.desc.SharedSystemMemory > 0 {
		return gputypes.DeviceTypeIntegratedGPU
	}
	if a.desc.DedicatedVideoMemory > 0 {
		return gputypes.DeviceTypeDiscreteGPU
	}
	return gputypes.DeviceTypeOther
}

// Open opens a logical device with the requested features and limits.
func (a *AdapterLegacy) Open(features gputypes.Features, limits gputypes.Limits) (hal.OpenDevice, error) {
	// Validate that the adapter supports the requested features
	supported := a.Features()
	if features&^supported != 0 {
		return hal.OpenDevice{}, fmt.Errorf("dx12: adapter does not support requested features")
	}

	// Create device using the legacy adapter
	device, err := newDevice(a.instance, unsafe.Pointer(a.raw), a.capabilities.FeatureLevel)
	if err != nil {
		return hal.OpenDevice{}, err
	}

	// Create queue wrapper
	queue := newQueue(device)

	return hal.OpenDevice{
		Device: device,
		Queue:  queue,
	}, nil
}

// TextureFormatCapabilities returns capabilities for a specific texture format.
func (a *AdapterLegacy) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	flags := hal.TextureFormatCapabilitySampled
	switch format {
	case gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatBGRA8UnormSrgb:
		flags |= hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample
	}
	return hal.TextureFormatCapabilities{Flags: flags}
}

// SurfaceCapabilities returns surface capabilities.
func (a *AdapterLegacy) SurfaceCapabilities(surface hal.Surface) *hal.SurfaceCapabilities {
	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
		},
		PresentModes: []hal.PresentMode{hal.PresentModeFifo},
		AlphaModes:   []hal.CompositeAlphaMode{hal.CompositeAlphaModeOpaque},
	}
}

// Destroy releases the adapter.
func (a *AdapterLegacy) Destroy() {
	if a.raw != nil {
		a.raw.Release()
		a.raw = nil
	}
}

// Compile-time interface assertions.
var (
	_ hal.Adapter = (*Adapter)(nil)
	_ hal.Adapter = (*AdapterLegacy)(nil)
)
