//go:build js && wasm

package browser

import (
	"math"
	"syscall/js"

	"github.com/gogpu/gputypes"
)

// PowerPreferenceToJS converts a gputypes.PowerPreference to the JS string
// expected by GPURequestAdapterOptions.powerPreference.
//
// Matches Rust wgpu's map of PowerPreference to GpuPowerPreference.
func PowerPreferenceToJS(pref gputypes.PowerPreference) string {
	switch pref {
	case gputypes.PowerPreferenceLowPower:
		return "low-power"
	case gputypes.PowerPreferenceHighPerformance:
		return "high-performance"
	default:
		// PowerPreferenceNone => omit the field (browser uses its default).
		return ""
	}
}

// BuildRequestAdapterOptions constructs a JS GPURequestAdapterOptions object.
func BuildRequestAdapterOptions(
	powerPreference gputypes.PowerPreference,
	forceFallback bool,
) js.Value {
	opts := js.Global().Get("Object").New()

	pref := PowerPreferenceToJS(powerPreference)
	if pref != "" {
		opts.Set("powerPreference", pref)
	}

	if forceFallback {
		opts.Set("forceFallbackAdapter", true)
	}

	return opts
}

// BuildDeviceDescriptor constructs a JS GPUDeviceDescriptor object.
//
// Matches Rust wgpu WebAdapter::request_device which builds the JS descriptor
// with requiredFeatures array and requiredLimits object.
func BuildDeviceDescriptor(
	label string,
	requiredFeatures gputypes.Features,
	requiredLimits gputypes.Limits,
) js.Value {
	desc := js.Global().Get("Object").New()

	if label != "" {
		desc.Set("label", label)
	}

	// Build requiredFeatures array.
	features := featuresToJSArray(requiredFeatures)
	if features.Length() > 0 {
		desc.Set("requiredFeatures", features)
	}

	// Build requiredLimits object. Only set non-zero limits to avoid
	// requesting values the adapter cannot provide.
	limits := limitsToJSObject(requiredLimits)
	desc.Set("requiredLimits", limits)

	return desc
}

// ExtractFeatures reads a GPUSupportedFeatures set and returns gputypes.Features.
//
// GPUSupportedFeatures is a Set-like object. We check each known WebGPU feature
// string using .has(). Matches Rust wgpu's map_wgt_features.
func ExtractFeatures(supported js.Value) gputypes.Features {
	if supported.IsUndefined() || supported.IsNull() {
		return 0
	}

	var features gputypes.Features
	for _, mapping := range featuresMappingTable {
		hasMethod := supported.Get("has")
		if !hasMethod.IsUndefined() && supported.Call("has", mapping.jsName).Bool() {
			features.Insert(mapping.feature)
		}
	}
	return features
}

// ExtractLimits reads a GPUSupportedLimits object and returns gputypes.Limits.
//
// GPUSupportedLimits has getter properties for each limit. We read them
// as float64 (JS numbers) and convert to the appropriate Go integer type.
// Matches Rust wgpu's map_wgt_limits.
func ExtractLimits(jsLimits js.Value) gputypes.Limits {
	if jsLimits.IsUndefined() || jsLimits.IsNull() {
		return gputypes.DefaultLimits()
	}

	return gputypes.Limits{
		MaxTextureDimension1D:                     getUint32(jsLimits, "maxTextureDimension1D"),
		MaxTextureDimension2D:                     getUint32(jsLimits, "maxTextureDimension2D"),
		MaxTextureDimension3D:                     getUint32(jsLimits, "maxTextureDimension3D"),
		MaxTextureArrayLayers:                     getUint32(jsLimits, "maxTextureArrayLayers"),
		MaxBindGroups:                             getUint32(jsLimits, "maxBindGroups"),
		MaxBindingsPerBindGroup:                   getUint32(jsLimits, "maxBindingsPerBindGroup"),
		MaxDynamicUniformBuffersPerPipelineLayout: getUint32(jsLimits, "maxDynamicUniformBuffersPerPipelineLayout"),
		MaxDynamicStorageBuffersPerPipelineLayout: getUint32(jsLimits, "maxDynamicStorageBuffersPerPipelineLayout"),
		MaxSampledTexturesPerShaderStage:          getUint32(jsLimits, "maxSampledTexturesPerShaderStage"),
		MaxSamplersPerShaderStage:                 getUint32(jsLimits, "maxSamplersPerShaderStage"),
		MaxStorageBuffersPerShaderStage:           getUint32(jsLimits, "maxStorageBuffersPerShaderStage"),
		MaxStorageTexturesPerShaderStage:          getUint32(jsLimits, "maxStorageTexturesPerShaderStage"),
		MaxUniformBuffersPerShaderStage:           getUint32(jsLimits, "maxUniformBuffersPerShaderStage"),
		MaxUniformBufferBindingSize:               getUint64(jsLimits, "maxUniformBufferBindingSize"),
		MaxStorageBufferBindingSize:               getUint64(jsLimits, "maxStorageBufferBindingSize"),
		MinUniformBufferOffsetAlignment:           getUint32(jsLimits, "minUniformBufferOffsetAlignment"),
		MinStorageBufferOffsetAlignment:           getUint32(jsLimits, "minStorageBufferOffsetAlignment"),
		MaxVertexBuffers:                          getUint32(jsLimits, "maxVertexBuffers"),
		MaxBufferSize:                             getUint64(jsLimits, "maxBufferSize"),
		MaxVertexAttributes:                       getUint32(jsLimits, "maxVertexAttributes"),
		MaxVertexBufferArrayStride:                getUint32(jsLimits, "maxVertexBufferArrayStride"),
		MaxColorAttachments:                       getUint32(jsLimits, "maxColorAttachments"),
		MaxColorAttachmentBytesPerSample:          getUint32(jsLimits, "maxColorAttachmentBytesPerSample"),
		MaxComputeWorkgroupStorageSize:            getUint32(jsLimits, "maxComputeWorkgroupStorageSize"),
		MaxComputeInvocationsPerWorkgroup:         getUint32(jsLimits, "maxComputeInvocationsPerWorkgroup"),
		MaxComputeWorkgroupSizeX:                  getUint32(jsLimits, "maxComputeWorkgroupSizeX"),
		MaxComputeWorkgroupSizeY:                  getUint32(jsLimits, "maxComputeWorkgroupSizeY"),
		MaxComputeWorkgroupSizeZ:                  getUint32(jsLimits, "maxComputeWorkgroupSizeZ"),
		MaxComputeWorkgroupsPerDimension:          getUint32(jsLimits, "maxComputeWorkgroupsPerDimension"),
	}
}

// featureMapping maps a gputypes.Feature to its WebGPU JS string name.
type featureMapping struct {
	feature gputypes.Feature
	jsName  string
}

// featuresMappingTable maps Go feature flags to WebGPU JS feature name strings.
// Matches Rust wgpu's FEATURES_MAPPING constant.
var featuresMappingTable = []featureMapping{
	{gputypes.FeatureDepthClipControl, "depth-clip-control"},
	{gputypes.FeatureDepth32FloatStencil8, "depth32float-stencil8"},
	{gputypes.FeatureTextureCompressionBC, "texture-compression-bc"},
	{gputypes.FeatureTextureCompressionETC2, "texture-compression-etc2"},
	{gputypes.FeatureTextureCompressionASTC, "texture-compression-astc"},
	{gputypes.FeatureTimestampQuery, "timestamp-query"},
	{gputypes.FeatureIndirectFirstInstance, "indirect-first-instance"},
	{gputypes.FeatureShaderF16, "shader-f16"},
	{gputypes.FeatureRG11B10UfloatRenderable, "rg11b10ufloat-renderable"},
	{gputypes.FeatureBGRA8UnormStorage, "bgra8unorm-storage"},
	{gputypes.FeatureFloat32Filterable, "float32-filterable"},
}

// featuresToJSArray builds a JS Array of feature name strings from Go flags.
func featuresToJSArray(features gputypes.Features) js.Value {
	array := js.Global().Get("Array").New()
	for _, mapping := range featuresMappingTable {
		if features.Contains(mapping.feature) {
			array.Call("push", mapping.jsName)
		}
	}
	return array
}

// limitsToJSObject converts gputypes.Limits to a JS object for GPUDeviceDescriptor.
// Only non-zero fields are set, matching Rust wgpu's map_js_sys_limits.
// JS numbers are f64, so uint64 values are sent as f64 per the WebGPU convention.
func limitsToJSObject(limits gputypes.Limits) js.Value {
	obj := js.Global().Get("Object").New()

	setNonZeroU32 := func(name string, val uint32) {
		if val != 0 {
			obj.Set(name, val)
		}
	}
	setNonZeroU64 := func(name string, val uint64) {
		if val != 0 {
			obj.Set(name, float64(val))
		}
	}

	setNonZeroU32("maxTextureDimension1D", limits.MaxTextureDimension1D)
	setNonZeroU32("maxTextureDimension2D", limits.MaxTextureDimension2D)
	setNonZeroU32("maxTextureDimension3D", limits.MaxTextureDimension3D)
	setNonZeroU32("maxTextureArrayLayers", limits.MaxTextureArrayLayers)
	setNonZeroU32("maxBindGroups", limits.MaxBindGroups)
	setNonZeroU32("maxBindingsPerBindGroup", limits.MaxBindingsPerBindGroup)
	setNonZeroU32("maxDynamicUniformBuffersPerPipelineLayout", limits.MaxDynamicUniformBuffersPerPipelineLayout)
	setNonZeroU32("maxDynamicStorageBuffersPerPipelineLayout", limits.MaxDynamicStorageBuffersPerPipelineLayout)
	setNonZeroU32("maxSampledTexturesPerShaderStage", limits.MaxSampledTexturesPerShaderStage)
	setNonZeroU32("maxSamplersPerShaderStage", limits.MaxSamplersPerShaderStage)
	setNonZeroU32("maxStorageBuffersPerShaderStage", limits.MaxStorageBuffersPerShaderStage)
	setNonZeroU32("maxStorageTexturesPerShaderStage", limits.MaxStorageTexturesPerShaderStage)
	setNonZeroU32("maxUniformBuffersPerShaderStage", limits.MaxUniformBuffersPerShaderStage)
	setNonZeroU64("maxUniformBufferBindingSize", limits.MaxUniformBufferBindingSize)
	setNonZeroU64("maxStorageBufferBindingSize", limits.MaxStorageBufferBindingSize)
	setNonZeroU32("minUniformBufferOffsetAlignment", limits.MinUniformBufferOffsetAlignment)
	setNonZeroU32("minStorageBufferOffsetAlignment", limits.MinStorageBufferOffsetAlignment)
	setNonZeroU32("maxVertexBuffers", limits.MaxVertexBuffers)
	setNonZeroU64("maxBufferSize", limits.MaxBufferSize)
	setNonZeroU32("maxVertexAttributes", limits.MaxVertexAttributes)
	setNonZeroU32("maxVertexBufferArrayStride", limits.MaxVertexBufferArrayStride)
	setNonZeroU32("maxColorAttachments", limits.MaxColorAttachments)
	setNonZeroU32("maxColorAttachmentBytesPerSample", limits.MaxColorAttachmentBytesPerSample)
	setNonZeroU32("maxComputeWorkgroupStorageSize", limits.MaxComputeWorkgroupStorageSize)
	setNonZeroU32("maxComputeInvocationsPerWorkgroup", limits.MaxComputeInvocationsPerWorkgroup)
	setNonZeroU32("maxComputeWorkgroupSizeX", limits.MaxComputeWorkgroupSizeX)
	setNonZeroU32("maxComputeWorkgroupSizeY", limits.MaxComputeWorkgroupSizeY)
	setNonZeroU32("maxComputeWorkgroupSizeZ", limits.MaxComputeWorkgroupSizeZ)
	setNonZeroU32("maxComputeWorkgroupsPerDimension", limits.MaxComputeWorkgroupsPerDimension)

	return obj
}

// getUint32 reads a JS number property as uint32. Returns 0 if missing.
func getUint32(obj js.Value, name string) uint32 {
	v := obj.Get(name)
	if v.IsUndefined() || v.IsNull() {
		return 0
	}
	return uint32(v.Int()) //nolint:gosec // JS API returns safe integers
}

// getUint64 reads a JS number property as uint64.
// JS numbers are f64, so values up to 2^53 are exact. Returns 0 if missing.
func getUint64(obj js.Value, name string) uint64 {
	v := obj.Get(name)
	if v.IsUndefined() || v.IsNull() {
		return 0
	}
	f := v.Float()
	if f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return uint64(f) //nolint:gosec // JS API returns safe integers within f64 precision
}
