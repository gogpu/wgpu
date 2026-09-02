//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestScanSPIRVCapabilities_InvalidMagic(t *testing.T) {
	t.Parallel()
	_, err := scanSPIRVCapabilities([]uint32{0, 1, 2, 3, 4})
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}

func TestScanSPIRVCapabilities_TooShort(t *testing.T) {
	t.Parallel()
	_, err := scanSPIRVCapabilities([]uint32{spirvMagic})
	if err == nil {
		t.Fatal("expected error for short module")
	}
}

func TestValidateSPIRVShaderFeatures_F16_NoFeature(t *testing.T) {
	t.Parallel()
	spirv := buildSPIRVWithCapabilities(spirvCapFloat16, spirvCapShader)
	err := validateSPIRVShaderFeatures(spirv, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateSPIRVShaderFeatures_Subgroup_NoFeature(t *testing.T) {
	t.Parallel()
	spirv := buildSPIRVWithCapabilities(spirvCapGroupNonUniform, spirvCapShader)
	err := validateSPIRVShaderFeatures(spirv, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateSPIRVShaderFeatures_MalformedModuleIgnored(t *testing.T) {
	t.Parallel()
	// Too short — scan error is ignored; HAL compilation will reject.
	err := validateSPIRVShaderFeatures([]uint32{spirvMagic, 1}, gputypes.Features(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
