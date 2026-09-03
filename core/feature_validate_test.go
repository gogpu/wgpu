//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestRequireFeature_Enabled(t *testing.T) {
	t.Parallel()
	features := gputypes.Features(gputypes.FeaturePushConstants)
	err := RequireFeature(features, gputypes.FeaturePushConstants, "CreatePipelineLayout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireFeature_Disabled(t *testing.T) {
	t.Parallel()
	err := RequireFeature(gputypes.Features(0), gputypes.FeaturePushConstants, "CreatePipelineLayout")
	if err == nil {
		t.Fatal("expected feature error, got nil")
	}
	var fe *FeatureError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FeatureError, got %T", err)
	}
	if fe.Feature != gputypes.FeaturePushConstants.String() {
		t.Errorf("Feature = %q, want %q", fe.Feature, gputypes.FeaturePushConstants.String())
	}
	if fe.Resource != "CreatePipelineLayout" {
		t.Errorf("Resource = %q, want CreatePipelineLayout", fe.Resource)
	}
}

func TestFeatureGateRegistryComplete(t *testing.T) {
	t.Parallel()
	covered := make(map[gputypes.Feature]struct{}, len(AllFeatureRequirements))
	for _, req := range AllFeatureRequirements {
		if req.Resource == "" {
			t.Errorf("feature %v missing Resource in registry", req.Feature)
		}
		if req.RustRef == "" {
			t.Errorf("feature %v missing RustRef in registry", req.Feature)
		}
		if _, dup := covered[req.Feature]; dup {
			t.Errorf("duplicate registry entry for feature %v", req.Feature)
		}
		covered[req.Feature] = struct{}{}
	}
	if got := len(covered); got != 25 {
		t.Errorf("registry covers %d features, want 25", got)
	}
}

func TestValidatePipelineLayoutDescriptor_PushConstants_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validPipelineLayoutWithPushConstants()
	err := ValidatePipelineLayoutDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if err == nil {
		t.Fatal("expected feature error, got nil")
	}
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidatePipelineLayoutDescriptor_PushConstants_WithFeature(t *testing.T) {
	t.Parallel()
	desc := validPipelineLayoutWithPushConstants()
	features := gputypes.Features(gputypes.FeaturePushConstants)
	err := ValidatePipelineLayoutDescriptor(desc, gputypes.DefaultLimits(), features)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validPipelineLayoutWithPushConstants() *hal.PipelineLayoutDescriptor {
	return &hal.PipelineLayoutDescriptor{
		Label: "push-constants-test",
		PushConstantRanges: []hal.PushConstantRange{
			{
				Stages: gputypes.ShaderStageVertex,
				Range:  hal.Range{Start: 0, End: 4},
			},
		},
	}
}
