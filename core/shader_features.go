//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/naga/wgsl"
	"github.com/gogpu/wgpu/hal"
)

func validateShaderModuleFeatures(desc *hal.ShaderModuleDescriptor, features gputypes.Features) error {
	if desc.Source.WGSL != "" {
		return validateWGSLShaderFeatures(desc.Source.WGSL, features)
	}
	if len(desc.Source.SPIRV) > 0 {
		return validateSPIRVShaderFeatures(desc.Source.SPIRV, features)
	}
	return nil
}

func validateWGSLShaderFeatures(source string, features gputypes.Features) error {
	module, _, err := wgsl.Parse(source, wgsl.ParseOptions{})
	if err != nil {
		return nil //nolint:nilerr // parse errors are reported by HAL shader compilation
	}

	// This is the only capability gate: naga's validator rejects every
	// optional feature outside Capabilities, and the HAL validates again only
	// for structure (internal/shaderir.Lower, valid.CapAll).
	opts := valid.Options{Capabilities: nagaCapabilitiesForFeatures(features)}
	if _, err := valid.Validate(module, opts); err != nil {
		var verrs valid.Errors
		if errors.As(err, &verrs) && len(verrs) > 0 {
			return mapShaderCapabilityError(verrs[0], features)
		}
		return fmt.Errorf("shader module: capability validation failed: %w", err)
	}

	return nil
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
// member for (SHADER_INT64, PRIMITIVE_INDEX, CLIP_DISTANCES,
// DUAL_SOURCE_BLENDING, the binding arrays, TEXTURE_ATOMIC, ...) is never
// granted, so a shader that needs it is rejected, as Rust rejects it on a
// device without the feature. The DownlevelFlags rows of Rust's function
// (SHADER_FLOAT16_IN_FLOAT32, MULTISAMPLED_SHADING, CUBE_ARRAY_TEXTURES,
// LINEAR_INTERPOLATION) have no naga capability to set.
var nagaCapabilityFeatures = []nagaCapabilityFeature{
	{valid.CapFloat64, "FLOAT64", gputypes.FeatureShaderFloat64},                                                         // lib.rs:18
	{valid.CapShaderFloat16, "SHADER_FLOAT16", gputypes.FeatureShaderF16},                                                // lib.rs:19-22
	{valid.CapAccelerationStructureBindingArray, "ACCELERATION_STRUCTURE_BINDING_ARRAY", gputypes.FeatureASBindingArray}, // lib.rs:71-74
	{valid.CapSubgroup, "SUBGROUP", gputypes.FeatureSubgroupOperations},                                                  // lib.rs:126-129
	{valid.CapSubgroupBarrier, "SUBGROUP_BARRIER", gputypes.FeatureSubgroupBarrier},                                      // lib.rs:130-133
	{valid.CapRayQuery, "RAY_QUERY", gputypes.FeatureRayQuery},                                                           // lib.rs:134-137
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

// mapShaderCapabilityError turns a missing-capability error into the
// missing-feature error of the device feature that grants it. naga names
// the capability by Rust's flag name ("Capabilities(SHADER_FLOAT16)").
func mapShaderCapabilityError(ve valid.Error, features gputypes.Features) error {
	names := capabilityNames(ve.Message)
	for _, row := range nagaCapabilityFeatures {
		if _, ok := names[row.name]; !ok {
			continue
		}
		if err := RequireFeature(features, row.feature, "CreateShaderModule"); err != nil {
			return err
		}
	}
	return fmt.Errorf("shader module: %s", ve.Error())
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
