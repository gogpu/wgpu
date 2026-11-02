//go:build !(js && wasm)

package core

import (
	"errors"
	"strings"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/naga/wgsl"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/shaderir"
)

func validateShaderModuleFeatures(desc *hal.ShaderModuleDescriptor, features gputypes.Features) error {
	if desc.Source.WGSL != "" {
		module, err := lowerWGSLShaderModule(desc.Label, desc.Source.WGSL, features)
		if err != nil {
			return err
		}
		desc.IR, desc.Info = module.IR, module.Info
		return nil
	}
	if len(desc.Source.SPIRV) > 0 {
		return validateSPIRVShaderFeatures(desc.Source.SPIRV, features)
	}
	return nil
}

// lowerWGSLShaderModule parses source and validates it once, with the naga
// capabilities of the device's features: Rust wgpu-core create_shader_module
// (wgpu-core/src/device/resource.rs), whose validator is built from
// features_to_naga_capabilities and whose result the HAL compiles without
// validating again. naga's validator refuses every optional feature outside
// Capabilities, so this is also the only capability gate.
func lowerWGSLShaderModule(label, source string, features gputypes.Features) (shaderir.Module, error) {
	opts := valid.Options{Capabilities: nagaCapabilitiesForFeatures(features)}
	module, err := shaderir.LowerOptions(source, opts)
	if err == nil {
		return module, nil
	}
	var pe *wgsl.ParseError
	if errors.As(err, &pe) {
		return shaderir.Module{}, &CreateShaderModuleError{Kind: CreateShaderModuleErrorParsing, Label: label, Cause: err}
	}
	var verrs valid.Errors
	if errors.As(err, &verrs) && len(verrs) > 0 {
		if ferr := missingCapabilityFeature(verrs[0], features); ferr != nil {
			return shaderir.Module{}, ferr
		}
	}
	return shaderir.Module{}, &CreateShaderModuleError{Kind: CreateShaderModuleErrorValidation, Label: label, Cause: err}
}

// nagaCapabilityFeature is one row of Rust's features_to_naga_capabilities
// (wgpu-naga-bridge/src/lib.rs:9-195) that has both a gputypes feature and a
// naga capability. name is Rust's flag name, which naga's "Shader requires
// capability" errors print.
type nagaCapabilityFeature struct {
	capability valid.Capabilities
	name       string
	feature    gputypes.Feature
}

// nagaCapabilityFeatures lists, in Rust's order, the capabilities a device
// feature enables. A capability Rust gates on a feature that gputypes has no
// member for yet (DUAL_SOURCE_BLENDING, the binding arrays, TEXTURE_ATOMIC,
// MULTIVIEW, 16-bit norm storage formats, mesh shaders, ...) is never
// granted, so a shader that needs it is rejected, as Rust rejects it on a
// device without the feature. The DownlevelFlags rows of Rust's function
// (SHADER_FLOAT16_IN_FLOAT32, MULTISAMPLED_SHADING, CUBE_ARRAY_TEXTURES,
// LINEAR_INTERPOLATION) have no naga capability to set.
//
// A capability Rust grants from either of two features (SUBGROUP from
// SUBGROUP or SUBGROUP_VERTEX, SHADER_INT64_ATOMIC_MIN_MAX from MIN_MAX or
// ALL_OPS) has one row per feature; the first row names the feature a
// missing-capability error asks for.
var nagaCapabilityFeatures = []nagaCapabilityFeature{
	{valid.CapFloat64, "FLOAT64", gputypes.FeatureShaderFloat64},                                                         // lib.rs:18
	{valid.CapShaderFloat16, "SHADER_FLOAT16", gputypes.FeatureShaderF16},                                                // lib.rs:19-22
	{valid.CapShaderInt16, "SHADER_INT16", gputypes.FeatureShaderI16},                                                    // lib.rs:27-30
	{valid.CapPrimitiveIndex, "PRIMITIVE_INDEX", gputypes.FeaturePrimitiveIndex},                                         // lib.rs:31-34
	{valid.CapAccelerationStructureBindingArray, "ACCELERATION_STRUCTURE_BINDING_ARRAY", gputypes.FeatureASBindingArray}, // lib.rs:71-74
	{valid.CapEarlyDepthTest, "EARLY_DEPTH_TEST", gputypes.FeatureShaderEarlyDepthTest},                                  // lib.rs:80-83
	{valid.CapShaderInt64, "SHADER_INT64", gputypes.FeatureShaderInt64},                                                  // lib.rs:84-87
	{valid.CapShaderInt64AtomicMinMax, "SHADER_INT64_ATOMIC_MIN_MAX", gputypes.FeatureShaderInt64AtomicMinMax},           // lib.rs:88-93
	{valid.CapShaderInt64AtomicMinMax, "SHADER_INT64_ATOMIC_MIN_MAX", gputypes.FeatureShaderInt64AtomicAllOps},           // lib.rs:88-93
	{valid.CapShaderInt64AtomicAllOps, "SHADER_INT64_ATOMIC_ALL_OPS", gputypes.FeatureShaderInt64AtomicAllOps},           // lib.rs:94-97
	{valid.CapShaderFloat32Atomic, "SHADER_FLOAT32_ATOMIC", gputypes.FeatureShaderFloat32Atomic},                         // lib.rs:106-109
	{valid.CapClipDistances, "CLIP_DISTANCES", gputypes.FeatureClipDistances},                                            // lib.rs:118-121
	{valid.CapSubgroup, "SUBGROUP", gputypes.FeatureSubgroupOperations},                                                  // lib.rs:126-129
	{valid.CapSubgroup, "SUBGROUP", gputypes.FeatureSubgroupVertex},                                                      // lib.rs:126-129
	{valid.CapSubgroupBarrier, "SUBGROUP_BARRIER", gputypes.FeatureSubgroupBarrier},                                      // lib.rs:130-133
	{valid.CapRayQuery, "RAY_QUERY", gputypes.FeatureRayQuery},                                                           // lib.rs:134-137
	{valid.CapSubgroupVertexStage, "SUBGROUP_VERTEX_STAGE", gputypes.FeatureSubgroupVertex},                              // lib.rs:138-141
	{valid.CapShaderBarycentrics, "SHADER_BARYCENTRICS", gputypes.FeatureShaderBarycentrics},                             // lib.rs:150-153
	{valid.CapPerVertex, "PER_VERTEX", gputypes.FeatureShaderPerVertex},                                                  // lib.rs:166-169
	{valid.CapDrawIndex, "DRAW_INDEX", gputypes.FeatureShaderDrawIndex},                                                  // lib.rs:170-173
}

func nagaCapabilitiesForFeatures(features gputypes.Features) valid.Capabilities {
	var caps valid.Capabilities
	for _, row := range nagaCapabilityFeatures {
		if features.Contains(row.feature) {
			caps |= row.capability
		}
	}
	return caps
}

// missingCapabilityFeature turns a missing-capability error into the
// missing-feature error of the device feature that grants it, or returns
// nil when ve names no capability a missing feature grants. naga names the
// capability by Rust's flag name ("Capabilities(SHADER_FLOAT16)").
func missingCapabilityFeature(ve valid.Error, features gputypes.Features) error {
	names := capabilityNames(ve.Message)
	for _, row := range nagaCapabilityFeatures {
		if _, ok := names[row.name]; !ok {
			continue
		}
		// Only the first row of a capability names its feature.
		delete(names, row.name)
		if err := RequireFeature(features, row.feature, "CreateShaderModule"); err != nil {
			return err
		}
	}
	return nil
}

// capabilityNames returns the upper-case flag-name tokens of msg. naga
// names a missing capability in two forms, "Shader requires capability
// Capabilities(A | B)" and "f16 requires the SHADER_FLOAT16 capability";
// whole tokens keep SUBGROUP apart from SUBGROUP_BARRIER.
func capabilityNames(msg string) map[string]struct{} {
	names := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(msg, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_'
	}) {
		names[token] = struct{}{}
	}
	return names
}
