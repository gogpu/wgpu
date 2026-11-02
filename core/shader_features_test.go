//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/valid"
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

// TestValidateWGSLShaderFeatures_SubgroupWordNotGated checks that only
// subgroup operations need the subgroup features: the word in a comment or
// an identifier is not a use.
func TestValidateWGSLShaderFeatures_SubgroupWordNotGated(t *testing.T) {
	t.Parallel()
	source := `// no subgroupBarrier or subgroup operations here
@group(0) @binding(0) var<storage, read_write> subgroup_sums: array<u32>;
@compute @workgroup_size(1) fn main() { subgroup_sums[0] = 1u; }`
	if err := validateWGSLShaderFeatures(source, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestNagaCapabilitiesForFeatures pins each device feature to the naga
// capabilities Rust's features_to_naga_capabilities gives it
// (wgpu-naga-bridge/src/lib.rs:9-195). FLOAT64 alone for ShaderFloat64:
// Rust gates SHADER_INT64 on its own SHADER_INT64 feature, which gputypes
// does not have.
func TestNagaCapabilitiesForFeatures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		feature gputypes.Feature
		want    valid.Capabilities
	}{
		{gputypes.FeatureShaderFloat64, valid.CapFloat64},                            // lib.rs:18
		{gputypes.FeatureShaderF16, valid.CapShaderFloat16},                          // lib.rs:19-22
		{gputypes.FeatureASBindingArray, valid.CapAccelerationStructureBindingArray}, // lib.rs:71-74
		{gputypes.FeatureSubgroupOperations, valid.CapSubgroup},                      // lib.rs:126-129
		{gputypes.FeatureSubgroupBarrier, valid.CapSubgroupBarrier},                  // lib.rs:130-133
		{gputypes.FeatureRayQuery, valid.CapRayQuery},                                // lib.rs:134-137
		// Features with no naga capability.
		{gputypes.FeatureDepthClipControl, 0},
		{gputypes.FeatureTimestampQuery, 0},
		{gputypes.FeaturePushConstants, 0},
		{gputypes.FeatureRayHitVertexReturn, 0},
		{gputypes.FeatureRayTracingPipelines, 0},
	}
	var all gputypes.Features
	var allCaps valid.Capabilities
	for _, tt := range tests {
		if got := nagaCapabilitiesForFeatures(gputypes.Features(tt.feature)); got != tt.want {
			t.Errorf("%v: capabilities %#x, want %#x", tt.feature, uint64(got), uint64(tt.want))
		}
		all |= gputypes.Features(tt.feature)
		allCaps |= tt.want
	}
	if got := nagaCapabilitiesForFeatures(all); got != allCaps {
		t.Errorf("all features: capabilities %#x, want %#x", uint64(got), uint64(allCaps))
	}
	if got := nagaCapabilitiesForFeatures(0); got != 0 {
		t.Errorf("no features: capabilities %#x, want 0", uint64(got))
	}
}

// TestValidateWGSLShaderCapabilityGates checks that a shader needing a
// capability is refused with the missing feature's FeatureError and passes
// once the device has the feature. naga names the capability in two message
// forms, both covered.
func TestValidateWGSLShaderCapabilityGates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		base    gputypes.Features // granted in both runs
		feature gputypes.Feature
		source  string
	}{
		// The WGSL spec requires `enable subgroups;` for subgroup built-ins
		// (Rust naga does not implement the directive yet).
		{"subgroup operation", 0, gputypes.FeatureSubgroupOperations, `enable subgroups;
@compute @workgroup_size(1) fn main() { let x = subgroupAdd(1u); }`},
		// A subgroup barrier needs SUBGROUP | SUBGROUP_BARRIER, as Rust.
		{"subgroup barrier", gputypes.Features(gputypes.FeatureSubgroupOperations), gputypes.FeatureSubgroupBarrier, `enable subgroups;
@compute @workgroup_size(1) fn main() { subgroupBarrier(); }`},
		{"f16 type", 0, gputypes.FeatureShaderF16, `enable f16;
@group(0) @binding(0) var<storage, read_write> out: array<f16>;
@compute @workgroup_size(1) fn main() { out[0] = 1.0h; }`},
		{"f64 type", 0, gputypes.FeatureShaderFloat64, `@group(0) @binding(0) var<storage, read_write> out: array<u32>;
var<private> x: f64;
@compute @workgroup_size(1) fn main() { out[0] = u32(f32(x)); }`},
		{"ray query", 0, gputypes.FeatureRayQuery, `enable wgpu_ray_query;
@group(0) @binding(0) var acc: acceleration_structure;
@group(0) @binding(1) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1) fn main() {
	var rq: ray_query;
	rayQueryInitialize(&rq, acc, RayDesc(0u, 0xFFu, 0.0, 1.0, vec3f(0.0), vec3f(0.0, 0.0, 1.0)));
	rayQueryProceed(&rq);
	out[0] = rayQueryGetCommittedIntersection(&rq).kind;
}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateWGSLShaderFeatures(tt.source, tt.base)
			var fe *FeatureError
			if !errors.As(err, &fe) || fe.Feature != tt.feature.String() {
				t.Fatalf("without %v: error %v (%T), want *FeatureError for it", tt.feature, err, err)
			}
			if err := validateWGSLShaderFeatures(tt.source, tt.base|gputypes.Features(tt.feature)); err != nil {
				t.Fatalf("with %v: %v", tt.feature, err)
			}
		})
	}
}
