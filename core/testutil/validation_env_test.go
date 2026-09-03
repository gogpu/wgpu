//go:build !(js && wasm)

package testutil_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/testutil"
)

func TestValidationEnv_AllHelpers(t *testing.T) {
	t.Parallel()
	env := testutil.NewValidationEnv(t,
		testutil.WithFeature(gputypes.FeaturePushConstants),
		testutil.WithFeature(gputypes.FeatureTextureCompressionBC),
		testutil.WithFeature(gputypes.FeatureDepthClipControl),
		testutil.WithFeature(gputypes.FeatureTimestampQuery),
		testutil.WithFeature(gputypes.FeatureShaderF16),
		testutil.WithFeature(gputypes.FeatureFloat32Filterable),
		testutil.WithLimits(gputypes.DefaultLimits()),
	)

	if err := env.CreatePipelineLayoutWithPushConstants(); err != nil {
		t.Fatalf("CreatePipelineLayoutWithPushConstants: %v", err)
	}
	if err := env.CreateTextureWithFormat(
		gputypes.TextureFormatBC1RGBAUnorm,
		gputypes.TextureUsageTextureBinding,
	); err != nil {
		t.Fatalf("CreateTextureWithFormat: %v", err)
	}
	if err := env.CreateRenderPipelineUnclippedDepth(); err != nil {
		t.Fatalf("CreateRenderPipelineUnclippedDepth: %v", err)
	}
	if err := env.CreateQuerySetTimestamp(); err != nil {
		t.Fatalf("CreateQuerySetTimestamp: %v", err)
	}
	if err := env.CreateShaderModuleWithWGSL(`@vertex fn main() -> @builtin(position) vec4<f32> {
		return vec4<f32>(0.0);
	}`); err != nil {
		t.Fatalf("CreateShaderModuleWithWGSL: %v", err)
	}
	if err := env.CreateBindGroupWithR32FloatTexture(); err != nil {
		t.Fatalf("CreateBindGroupWithR32FloatTexture: %v", err)
	}
}

func TestValidationEnv_WithFeatures(t *testing.T) {
	t.Parallel()
	fs := gputypes.Features(gputypes.FeatureTimestampQuery)
	env := testutil.NewValidationEnv(t, testutil.WithFeatures(fs))
	if err := env.CreateQuerySetTimestamp(); err != nil {
		t.Fatalf("CreateQuerySetTimestamp: %v", err)
	}
}
