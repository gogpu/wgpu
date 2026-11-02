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
		{gputypes.FeatureShaderI16, valid.CapShaderInt16},                            // lib.rs:27-30
		{gputypes.FeaturePrimitiveIndex, valid.CapPrimitiveIndex},                    // lib.rs:31-34
		{gputypes.FeatureASBindingArray, valid.CapAccelerationStructureBindingArray}, // lib.rs:71-74
		{gputypes.FeatureShaderEarlyDepthTest, valid.CapEarlyDepthTest},              // lib.rs:80-83
		{gputypes.FeatureShaderInt64, valid.CapShaderInt64},                          // lib.rs:84-87
		{gputypes.FeatureShaderInt64AtomicMinMax, valid.CapShaderInt64AtomicMinMax},  // lib.rs:88-93
		// ALL_OPS grants MIN_MAX too (lib.rs:88-97).
		{gputypes.FeatureShaderInt64AtomicAllOps, valid.CapShaderInt64AtomicMinMax | valid.CapShaderInt64AtomicAllOps},
		{gputypes.FeatureShaderFloat32Atomic, valid.CapShaderFloat32Atomic}, // lib.rs:106-109
		{gputypes.FeatureClipDistances, valid.CapClipDistances},             // lib.rs:118-121
		{gputypes.FeatureSubgroupOperations, valid.CapSubgroup},             // lib.rs:126-129
		{gputypes.FeatureSubgroupBarrier, valid.CapSubgroupBarrier},         // lib.rs:130-133
		{gputypes.FeatureRayQuery, valid.CapRayQuery},                       // lib.rs:134-137
		// SUBGROUP_VERTEX grants SUBGROUP too (lib.rs:126-129, 138-141).
		{gputypes.FeatureSubgroupVertex, valid.CapSubgroup | valid.CapSubgroupVertexStage},
		{gputypes.FeatureShaderBarycentrics, valid.CapShaderBarycentrics}, // lib.rs:150-153
		{gputypes.FeatureShaderPerVertex, valid.CapPerVertex},             // lib.rs:166-169
		{gputypes.FeatureShaderDrawIndex, valid.CapDrawIndex},             // lib.rs:170-173
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
		{"primitive index", 0, gputypes.FeaturePrimitiveIndex, `enable primitive_index;
@fragment fn main(@builtin(primitive_index) p: u32) -> @location(0) vec4f { return vec4f(f32(p)); }`},
		{"clip distances", 0, gputypes.FeatureClipDistances, `enable clip_distances;
struct VOut { @builtin(position) pos: vec4f, @builtin(clip_distances) clip: array<f32, 1> }
@vertex fn main() -> VOut { return VOut(vec4f(0.0), array<f32, 1>(1.0)); }`},
		{"draw index", 0, gputypes.FeatureShaderDrawIndex, `enable draw_index;
@vertex fn main(@builtin(draw_index) d: u32) -> @builtin(position) vec4f { return vec4f(f32(d)); }`},
		{"i64 type", 0, gputypes.FeatureShaderInt64, `@group(0) @binding(0) var<storage, read_write> out: array<u32>;
var<private> x: i64;
@compute @workgroup_size(1) fn main() { out[0] = u32(x); }`},
		// No WGSL row for FeatureShaderI16: naga v0.21.0's WGSL front end has
		// no i16/u16 type names yet, so only SPIR-V input reaches the gate
		// (the mapping is covered by TestNagaCapabilitiesForFeatures).
		{"early depth test", 0, gputypes.FeatureShaderEarlyDepthTest, `@fragment @early_depth_test(force) fn main() -> @location(0) vec4f { return vec4f(1.0); }`},
		{"barycentrics", 0, gputypes.FeatureShaderBarycentrics, `@fragment fn main(@builtin(barycentric) b: vec3f) -> @location(0) vec4f { return vec4f(b, 1.0); }`},
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

// TestValidateShaderModuleDescriptor_LowersWGSLOnce checks that core parses
// and validates a WGSL module once, with the device's capabilities, and hands
// the IR and its ModuleInfo to the HAL in the descriptor, as Rust wgpu-core
// create_shader_module hands the HAL hal::ShaderInput::Naga
// (wgpu-core/src/device/resource.rs). A module using f64 on a device with
// ShaderFloat64 is accepted with its IR.
func TestValidateShaderModuleDescriptor_LowersWGSLOnce(t *testing.T) {
	t.Parallel()
	desc := &hal.ShaderModuleDescriptor{Source: hal.ShaderSource{WGSL: `@group(0) @binding(0) var<storage, read_write> out: array<u32>;
var<private> x: f64;
@compute @workgroup_size(1) fn main() { out[0] = u32(f32(x)); }`}}
	if err := ValidateShaderModuleDescriptor(desc, gputypes.Features(gputypes.FeatureShaderFloat64)); err != nil {
		t.Fatalf("ValidateShaderModuleDescriptor: %v", err)
	}
	if desc.IR == nil {
		t.Fatal("desc.IR is nil: core did not hand the validated module to the HAL")
	}
	if len(desc.Info.EntryPoints) != len(desc.IR.EntryPoints) || len(desc.Info.EntryPoints) != 1 {
		t.Fatalf("desc.Info has %d entry points, IR has %d, want 1", len(desc.Info.EntryPoints), len(desc.IR.EntryPoints))
	}
}

// TestValidateShaderModuleDescriptor_WGSLErrors checks that a module core
// cannot parse or validate is refused there with a typed error, as Rust's
// CreateShaderModuleError::Parsing and ::Validation, and that a SPIR-V
// module carries no IR.
func TestValidateShaderModuleDescriptor_WGSLErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   CreateShaderModuleErrorKind
	}{
		{"parse", `@compute @workgroup_size(1) fn main( {`, CreateShaderModuleErrorParsing},
		{"validation", `@compute @workgroup_size(1) fn main() { let x: u32 = 1.5f; }`, CreateShaderModuleErrorParsing},
		{"undeclared", `@compute @workgroup_size(1) fn main() { _ = undefined_name; }`, CreateShaderModuleErrorParsing},
		{"binding collision", `@group(0) @binding(0) var<storage, read_write> a: array<u32>;
@group(0) @binding(0) var<storage, read_write> b: array<u32>;
@compute @workgroup_size(1) fn main() { a[0] = b[0]; }`, CreateShaderModuleErrorValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			desc := &hal.ShaderModuleDescriptor{Label: tt.name, Source: hal.ShaderSource{WGSL: tt.source}}
			err := ValidateShaderModuleDescriptor(desc, 0)
			var csme *CreateShaderModuleError
			if !errors.As(err, &csme) || csme.Kind != tt.want {
				t.Fatalf("error %v (%T), want *CreateShaderModuleError kind %d", err, err, tt.want)
			}
			if csme.Unwrap() == nil {
				t.Fatal("error does not wrap naga's error")
			}
			if desc.IR != nil {
				t.Fatal("desc.IR set for a rejected module")
			}
		})
	}

	spirv := &hal.ShaderModuleDescriptor{Source: hal.ShaderSource{SPIRV: buildSPIRVWithCapabilities(spirvCapShader)}}
	if err := ValidateShaderModuleDescriptor(spirv, 0); err != nil {
		t.Fatalf("SPIR-V: %v", err)
	}
	if spirv.IR != nil {
		t.Fatal("SPIR-V module got IR")
	}
}

// validateWGSLShaderFeatures runs the core WGSL lowering for its error only.
func validateWGSLShaderFeatures(source string, features gputypes.Features) error {
	_, err := lowerWGSLShaderModule("", source, features)
	return err
}
