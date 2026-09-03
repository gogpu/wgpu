//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestValidateShaderModuleFeatures_EmptySource(t *testing.T) {
	t.Parallel()
	err := validateShaderModuleFeatures(&hal.ShaderModuleDescriptor{}, gputypes.Features(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateShaderModuleFeatures_SPIRVPath(t *testing.T) {
	t.Parallel()
	spirv := buildSPIRVWithCapabilities(spirvCapFloat64, spirvCapShader)
	desc := &hal.ShaderModuleDescriptor{
		Source: hal.ShaderSource{SPIRV: spirv},
	}
	err := validateShaderModuleFeatures(desc, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateShaderSubgroupFeatures_Barrier_NoFeature(t *testing.T) {
	t.Parallel()
	source := `@compute @workgroup_size(1) fn main() { subgroupBarrier(); }`
	err := validateShaderSubgroupFeatures(source, nil, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateShaderSubgroupFeatures_Operations_WithFeature(t *testing.T) {
	t.Parallel()
	source := `@compute @workgroup_size(1) fn main() { let x = subgroupAdd(1u); }`
	features := gputypes.Features(gputypes.FeatureSubgroupOperations)
	if err := validateShaderSubgroupFeatures(source, nil, features); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNagaCapabilitiesForFeatures(t *testing.T) {
	t.Parallel()
	features := gputypes.Features(gputypes.FeatureShaderF16 | gputypes.FeatureShaderFloat64)
	caps := nagaCapabilitiesForFeatures(features)
	if caps == 0 {
		t.Fatal("expected non-zero capabilities")
	}
}
