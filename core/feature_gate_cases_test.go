//go:build !(js && wasm)

package core_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/testutil"
)

type featureGateCase struct {
	name     string
	feature  gputypes.Feature
	resource string
	act      func(env *testutil.ValidationEnv) error
}

var featureGateCases = []featureGateCase{
	{
		name:     "PushConstants/CreatePipelineLayout/noFeature",
		feature:  gputypes.FeaturePushConstants,
		resource: "CreatePipelineLayout",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreatePipelineLayoutWithPushConstants()
		},
	},
	{
		name:     "TextureCompressionBC/CreateTexture/noFeature",
		feature:  gputypes.FeatureTextureCompressionBC,
		resource: "CreateTexture",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateTextureWithFormat(
				gputypes.TextureFormatBC1RGBAUnorm,
				gputypes.TextureUsageTextureBinding,
			)
		},
	},
	{
		name:     "Depth32FloatStencil8/CreateTexture/noFeature",
		feature:  gputypes.FeatureDepth32FloatStencil8,
		resource: "CreateTexture",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateTextureWithFormat(
				gputypes.TextureFormatDepth32FloatStencil8,
				gputypes.TextureUsageTextureBinding,
			)
		},
	},
	{
		name:     "BGRA8UnormStorage/CreateTexture/noFeature",
		feature:  gputypes.FeatureBGRA8UnormStorage,
		resource: "CreateTexture",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateTextureWithFormat(
				gputypes.TextureFormatBGRA8Unorm,
				gputypes.TextureUsageStorageBinding,
			)
		},
	},
	{
		name:     "RG11B10UfloatRenderable/CreateTexture/noFeature",
		feature:  gputypes.FeatureRG11B10UfloatRenderable,
		resource: "CreateTexture",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateTextureWithFormat(
				gputypes.TextureFormatRG11B10Ufloat,
				gputypes.TextureUsageRenderAttachment,
			)
		},
	},
	{
		name:     "DepthClipControl/CreateRenderPipeline/noFeature",
		feature:  gputypes.FeatureDepthClipControl,
		resource: "CreateRenderPipeline",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateRenderPipelineUnclippedDepth()
		},
	},
	{
		name:     "ShaderF16/CreateShaderModule/noFeature",
		feature:  gputypes.FeatureShaderF16,
		resource: "CreateShaderModule",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateShaderModuleWithWGSL(`@vertex fn main() -> @builtin(position) vec4<f32> {
			 var x: f16 = 1.0;
			 return vec4<f32>(f32(x));
			}`)
		},
	},
	{
		name:     "SubgroupOperations/CreateShaderModule/noFeature",
		feature:  gputypes.FeatureSubgroupOperations,
		resource: "CreateShaderModule",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateShaderModuleWithWGSL(`@compute @workgroup_size(1) fn main() {
			 let _ = subgroupAdd(1u);
			}`)
		},
	},
	{
		name:     "TimestampQuery/CreateQuerySet/noFeature",
		feature:  gputypes.FeatureTimestampQuery,
		resource: "CreateQuerySet",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateQuerySetTimestamp()
		},
	},
	{
		name:     "Float32Filterable/CreateBindGroup/noFeature",
		feature:  gputypes.FeatureFloat32Filterable,
		resource: "CreateBindGroup",
		act: func(env *testutil.ValidationEnv) error {
			return env.CreateBindGroupWithR32FloatTexture()
		},
	},
}

// wiredFeatureGateTargets lists Phase C features validated via featureGateCases.
// Integration-only gates (MultiDrawIndirect, IndirectFirstInstance, ray tracing) live in wgpu_test.
var wiredFeatureGateTargets = []gputypes.Feature{
	gputypes.FeaturePushConstants,
	gputypes.FeatureTextureCompressionBC,
	gputypes.FeatureDepth32FloatStencil8,
	gputypes.FeatureBGRA8UnormStorage,
	gputypes.FeatureRG11B10UfloatRenderable,
	gputypes.FeatureDepthClipControl,
	gputypes.FeatureShaderF16,
	gputypes.FeatureSubgroupOperations,
	gputypes.FeatureTimestampQuery,
	gputypes.FeatureFloat32Filterable,
}

func TestFeatureGates(t *testing.T) {
	for _, tc := range featureGateCases {
		t.Run(tc.name+"/negative", func(t *testing.T) {
			t.Parallel()
			env := testutil.NewValidationEnv(t)
			err := tc.act(env)
			testutil.AssertFeatureError(t, err, tc.feature, tc.resource)
		})
		t.Run(tc.name+"/positive", func(t *testing.T) {
			t.Parallel()
			env := testutil.NewValidationEnv(t, testutil.WithFeature(tc.feature))
			if err := tc.act(env); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFeatureGateCasesCoverRegistryTargets(t *testing.T) {
	t.Parallel()
	covered := map[gputypes.Feature]struct{}{}
	for _, tc := range featureGateCases {
		if tc.feature == 0 {
			t.Fatalf("case %q has zero feature", tc.name)
		}
		if tc.resource == "" {
			t.Fatalf("case %q has empty resource", tc.name)
		}
		covered[tc.feature] = struct{}{}
	}
	for _, feature := range wiredFeatureGateTargets {
		if _, ok := covered[feature]; !ok {
			t.Errorf("wired feature %v missing from featureGateCases", feature)
		}
	}
	if len(covered) == 0 {
		t.Fatal("featureGateCases is empty")
	}
}
