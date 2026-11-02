//go:build !(js && wasm)

package core

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestValidateDeviceRequest checks RequestDevice's checks against the
// adapter, as Rust wgpu-core Adapter::validate_device_descriptor
// (wgpu-core/src/instance.rs): a feature the adapter does not expose is
// UnsupportedFeature, a limit better than the adapter's is LimitsExceeded.
// Better is larger for a maximum and smaller for an alignment.
func TestValidateDeviceRequest(t *testing.T) {
	t.Parallel()
	adapterFeatures := gputypes.Features(gputypes.FeatureShaderFloat64 | gputypes.FeatureDepthClipControl)
	adapterLimits := gputypes.DefaultLimits()

	type mutate func(*gputypes.Limits)
	tests := []struct {
		name        string
		features    gputypes.Features
		limits      mutate
		wantKind    RequestDeviceErrorKind
		wantMissing gputypes.Features
		wantLimit   string
	}{
		{name: "adapter's own features and limits"},
		{name: "subset of features", features: gputypes.Features(gputypes.FeatureShaderFloat64)},
		{name: "worse limits", limits: func(l *gputypes.Limits) {
			l.MaxBindGroups = 1
			l.MinUniformBufferOffsetAlignment *= 2
		}},
		{
			name:        "unsupported feature",
			features:    gputypes.Features(gputypes.FeatureShaderFloat64 | gputypes.FeatureSubgroupOperations | gputypes.FeatureRayQuery),
			wantKind:    RequestDeviceErrorUnsupportedFeature,
			wantMissing: gputypes.Features(gputypes.FeatureSubgroupOperations | gputypes.FeatureRayQuery),
		},
		{
			name:      "maximum above the adapter's",
			limits:    func(l *gputypes.Limits) { l.MaxBindGroups++ },
			wantKind:  RequestDeviceErrorLimitsExceeded,
			wantLimit: "MaxBindGroups",
		},
		{
			name:      "64-bit maximum above the adapter's",
			limits:    func(l *gputypes.Limits) { l.MaxBufferSize++ },
			wantKind:  RequestDeviceErrorLimitsExceeded,
			wantLimit: "MaxBufferSize",
		},
		{
			name:      "alignment below the adapter's",
			limits:    func(l *gputypes.Limits) { l.MinStorageBufferOffsetAlignment /= 2 },
			wantKind:  RequestDeviceErrorLimitsExceeded,
			wantLimit: "MinStorageBufferOffsetAlignment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			features := tt.features
			if tt.name == "adapter's own features and limits" {
				features = adapterFeatures
			}
			limits := adapterLimits
			if tt.limits != nil {
				tt.limits(&limits)
			}
			err := ValidateDeviceRequest(features, limits, adapterFeatures, adapterLimits)
			if tt.wantLimit == "" && tt.wantMissing == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var rde *RequestDeviceError
			if !errors.As(err, &rde) || rde.Kind != tt.wantKind {
				t.Fatalf("error %v (%T), want *RequestDeviceError kind %d", err, err, tt.wantKind)
			}
			if rde.MissingFeatures != tt.wantMissing || rde.Limit != tt.wantLimit {
				t.Fatalf("missing %v limit %q, want %v %q", rde.MissingFeatures, rde.Limit, tt.wantMissing, tt.wantLimit)
			}
			if rde.Error() == "" {
				t.Fatal("empty error text")
			}
		})
	}
}

// TestDeviceLimitFieldsComparable pins that every gputypes.Limits field is
// one RequestDevice compares: an unsigned integer, and an alignment exactly
// when its name starts with "Min". A new field of another kind would be
// skipped silently.
func TestDeviceLimitFieldsComparable(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(gputypes.Limits{})
	alignments := 0
	for i := range typ.NumField() {
		f := typ.Field(i)
		if k := f.Type.Kind(); k != reflect.Uint32 && k != reflect.Uint64 {
			t.Errorf("Limits.%s is %v, not compared by RequestDevice", f.Name, k)
		}
		if isAlignmentLimit(f.Name) {
			alignments++
		}
	}
	if alignments != 2 {
		t.Errorf("%d alignment limits, want 2 (MinUniformBufferOffsetAlignment, MinStorageBufferOffsetAlignment)", alignments)
	}
}
