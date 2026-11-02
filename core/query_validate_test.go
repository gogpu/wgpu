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

func TestValidatePassTimestampWrites(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureTimestampQuery)
	device := NewDevice(&mockHALDevice{}, &Adapter{}, features, gputypes.DefaultLimits(), "timestamps")
	otherDevice := NewDevice(&mockHALDevice{}, &Adapter{}, features, gputypes.DefaultLimits(), "other")
	active := NewQuerySet(mockQuerySet{}, device, hal.QueryTypeTimestamp, 4, "active")
	wrongType := NewQuerySet(mockQuerySet{}, device, hal.QueryTypeOcclusion, 4, "occlusion")
	destroyed := NewQuerySet(mockQuerySet{}, device, hal.QueryTypeTimestamp, 4, "destroyed")
	destroyed.Destroy()
	defer active.Destroy()
	defer wrongType.Destroy()

	zero, one, four := uint32(0), uint32(1), uint32(4)
	tests := []struct {
		name     string
		features gputypes.Features
		device   *Device
		set      *QuerySet
		begin    *uint32
		end      *uint32
		wantErr  bool
	}{
		{name: "valid begin and end", features: features, device: device, set: active, begin: &zero, end: &one},
		{name: "valid one boundary", features: features, device: device, set: active, end: &one},
		{name: "feature required", device: device, set: active, end: &one, wantErr: true},
		{name: "missing set", features: features, device: device, end: &one, wantErr: true},
		{name: "destroyed set", features: features, device: device, set: destroyed, end: &one, wantErr: true},
		{name: "wrong device", features: features, device: otherDevice, set: active, end: &one, wantErr: true},
		{name: "wrong query type", features: features, device: device, set: wrongType, end: &one, wantErr: true},
		{name: "index out of range", features: features, device: device, set: active, begin: &four, wantErr: true},
		{name: "duplicate indices", features: features, device: device, set: active, begin: &one, end: &one, wantErr: true},
		{name: "no indices", features: features, device: device, set: active, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.device
			if d == nil {
				d = device
			}
			d.Features = tt.features
			err := ValidatePassTimestampWrites(d, tt.set, tt.begin, tt.end, "pass")
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePassTimestampWrites() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
