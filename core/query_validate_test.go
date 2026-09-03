//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestValidateQuerySetDescriptor_Nil(t *testing.T) {
	t.Parallel()
	if err := ValidateQuerySetDescriptor(nil, gputypes.Features(0)); err == nil {
		t.Fatal("expected error for nil descriptor")
	}
}

func TestValidateQuerySetDescriptor_ZeroCount(t *testing.T) {
	t.Parallel()
	err := ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryTypeOcclusion,
		Count: 0,
	}, gputypes.Features(0))
	if err == nil {
		t.Fatal("expected error for zero count")
	}
}

func TestValidateQuerySetDescriptor_Occlusion(t *testing.T) {
	t.Parallel()
	err := ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryTypeOcclusion,
		Count: 4,
	}, gputypes.Features(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateQuerySetDescriptor_PipelineStatistics_NoFeature(t *testing.T) {
	t.Parallel()
	err := ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryType(2),
		Count: 1,
	}, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateQuerySetDescriptor_UnknownType(t *testing.T) {
	t.Parallel()
	err := ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryType(99),
		Count: 1,
	}, gputypes.Features(0))
	if err == nil {
		t.Fatal("expected error for unknown query type")
	}
}
