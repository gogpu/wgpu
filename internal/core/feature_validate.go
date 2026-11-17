//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
)

// FeatureRequirement documents where a WebGPU feature must be validated at usage time.
// Used by validation logic and registry completeness tests (Phase C).
type FeatureRequirement struct {
	Feature  gputypes.Feature
	Resource string // API entry point, e.g. "MultiDrawIndirect", "CreatePipelineLayout"
	RustRef  string // Rust wgpu-core source reference
}

// Rust wgpu-core reference paths for the feature registry.
const (
	rustRefDeviceResource = "wgpu-core/device/resource.rs"
	rustRefCommandRender  = "wgpu-core/command/render.rs"
	rustRefRaytracing     = "internal/raytracing/validate.go"
)

// Resource entry points referenced by multiple feature requirements.
const (
	resourceCreateTexture        = "CreateTexture"
	resourceCreateShaderModule   = "CreateShaderModule"
	resourceCreateRenderPipeline = "CreateRenderPipeline"
	resourceCreateQuerySet       = "CreateQuerySet"
)

// AllFeatureRequirements is the canonical registry of feature-gated operations.
// Phase C wires checks incrementally; the registry documents the full target set.
var AllFeatureRequirements = []FeatureRequirement{
	{Feature: gputypes.FeatureDepthClipControl, Resource: resourceCreateRenderPipeline, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureDepth32FloatStencil8, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureTextureCompressionBC, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureTextureCompressionETC2, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureTextureCompressionASTC, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureIndirectFirstInstance, Resource: "DrawIndirect", RustRef: rustRefCommandRender},
	{Feature: gputypes.FeatureShaderF16, Resource: resourceCreateShaderModule, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureRG11B10UfloatRenderable, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureBGRA8UnormStorage, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureFloat32Filterable, Resource: "CreateBindGroup", RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureTimestampQuery, Resource: resourceCreateQuerySet, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeaturePipelineStatisticsQuery, Resource: resourceCreateQuerySet, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureMultiDrawIndirect, Resource: "MultiDrawIndirect", RustRef: rustRefCommandRender},
	{Feature: gputypes.FeatureMultiDrawIndirectCount, Resource: "MultiDrawIndirectCount", RustRef: rustRefCommandRender},
	{Feature: gputypes.FeaturePushConstants, Resource: "CreatePipelineLayout", RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureTextureAdapterSpecificFormatFeatures, Resource: resourceCreateTexture, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureShaderFloat64, Resource: resourceCreateShaderModule, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureVertexAttribute64bit, Resource: resourceCreateRenderPipeline, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureSubgroupOperations, Resource: resourceCreateShaderModule, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureSubgroupBarrier, Resource: resourceCreateShaderModule, RustRef: rustRefDeviceResource},
	{Feature: gputypes.FeatureRayQuery, Resource: "RayTracing", RustRef: rustRefRaytracing},
	{Feature: gputypes.FeatureRayHitVertexReturn, Resource: "RayTracing", RustRef: rustRefRaytracing},
	{Feature: gputypes.FeatureExtendedASVertexFormats, Resource: "CreateBlas", RustRef: rustRefRaytracing},
	{Feature: gputypes.FeatureASBindingArray, Resource: "CreateBindGroupLayout", RustRef: rustRefRaytracing},
	{Feature: gputypes.FeatureRayTracingPipelines, Resource: "CreateRayTracingPipeline", RustRef: rustRefRaytracing},
}

// RequireFeature returns a *FeatureError when feature is not enabled on the device.
//
// VAL-C0: base helper for all usage-time feature gate checks.
func RequireFeature(features gputypes.Features, feature gputypes.Feature, resource string) error {
	if features.Contains(feature) {
		return nil
	}
	return NewFeatureError(resource, feature.String())
}
