//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogpu/gputypes"
	nagair "github.com/gogpu/naga/ir"
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
	// Subgroup gates scan raw source and run even when naga parse fails.
	if err := validateShaderSubgroupFeatures(source, nil, features); err != nil {
		return err
	}

	module, _, err := wgsl.Parse(source, wgsl.ParseOptions{})
	if err != nil {
		return nil //nolint:nilerr // parse errors are reported by HAL shader compilation
	}

	caps := nagaCapabilitiesForFeatures(features)
	if _, err := nagair.Validate(module, nagair.ValidateOptions{Capabilities: caps}); err != nil {
		var verrs nagair.ValidationErrors
		if errors.As(err, &verrs) && len(verrs) > 0 {
			return mapShaderCapabilityError(verrs[0], features)
		}
		return fmt.Errorf("shader module: capability validation failed: %w", err)
	}

	return nil
}

func nagaCapabilitiesForFeatures(features gputypes.Features) nagair.Capabilities {
	var caps nagair.Capabilities
	if features.Contains(gputypes.FeatureShaderF16) {
		caps |= nagair.CapShaderFloat16
	}
	if features.Contains(gputypes.FeatureShaderFloat64) {
		caps |= nagair.CapFloat64 | nagair.CapShaderInt64
	}
	return caps
}

func mapShaderCapabilityError(ve nagair.ValidationError, features gputypes.Features) error {
	msg := ve.Message
	switch {
	case strings.Contains(msg, "SHADER_FLOAT16") || strings.Contains(msg, "f16"):
		return RequireFeature(features, gputypes.FeatureShaderF16, "CreateShaderModule")
	case strings.Contains(msg, "FLOAT64") || strings.Contains(msg, "f64"):
		return RequireFeature(features, gputypes.FeatureShaderFloat64, "CreateShaderModule")
	case strings.Contains(msg, "SHADER_INT64") || strings.Contains(msg, "i64") || strings.Contains(msg, "u64"):
		return RequireFeature(features, gputypes.FeatureShaderFloat64, "CreateShaderModule")
	default:
		return fmt.Errorf("shader module: %s", msg)
	}
}

func validateShaderSubgroupFeatures(source string, _ *nagair.Module, features gputypes.Features) error {
	usesBarrier := strings.Contains(source, "subgroupBarrier")
	usesSubgroup := strings.Contains(source, "subgroup")

	if usesBarrier {
		if err := RequireFeature(features, gputypes.FeatureSubgroupBarrier, "CreateShaderModule"); err != nil {
			return err
		}
	}
	if usesSubgroup {
		if err := RequireFeature(features, gputypes.FeatureSubgroupOperations, "CreateShaderModule"); err != nil {
			return err
		}
	}
	return nil
}
