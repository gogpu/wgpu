//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// TestRequestDeviceRejectsWhatTheAdapterLacks checks that RequestDevice
// refuses features the adapter does not expose and limits better than the
// adapter's, as the WebGPU requestDevice steps and Rust wgpu-core
// validate_device_descriptor (wgpu-core/src/instance.rs) do, instead of
// recording a feature the device was never created with.
func TestRequestDeviceRejectsWhatTheAdapterLacks(t *testing.T) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()
	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	defer adapter.Release()

	var missing gputypes.Feature
	for _, f := range []gputypes.Feature{
		gputypes.FeatureSubgroupOperations, gputypes.FeatureRayQuery,
		gputypes.FeatureASBindingArray, gputypes.FeatureShaderFloat64,
	} {
		if !adapter.Features().Contains(f) {
			missing = f
			break
		}
	}
	if missing == 0 {
		t.Fatalf("adapter %q exposes every candidate feature; extend the list", adapter.Info().Name)
	}

	moreBindGroups := adapter.Limits()
	moreBindGroups.MaxBindGroups++
	smallerAlignment := adapter.Limits()
	smallerAlignment.MinUniformBufferOffsetAlignment /= 2

	tests := []struct {
		name      string
		desc      *wgpu.DeviceDescriptor
		wantKind  core.RequestDeviceErrorKind
		wantLimit string
	}{
		{"unsupported feature", &wgpu.DeviceDescriptor{RequiredFeatures: gputypes.Features(missing)}, core.RequestDeviceErrorUnsupportedFeature, ""},
		{"maximum above the adapter's", &wgpu.DeviceDescriptor{RequiredLimits: moreBindGroups}, core.RequestDeviceErrorLimitsExceeded, "MaxBindGroups"},
		{"alignment below the adapter's", &wgpu.DeviceDescriptor{RequiredLimits: smallerAlignment}, core.RequestDeviceErrorLimitsExceeded, "MinUniformBufferOffsetAlignment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, err := adapter.RequestDevice(tt.desc)
			if err == nil {
				device.Release()
				t.Fatal("RequestDevice succeeded")
			}
			var rde *core.RequestDeviceError
			if !errors.As(err, &rde) || rde.Kind != tt.wantKind || rde.Limit != tt.wantLimit {
				t.Fatalf("error %v (%T), want *core.RequestDeviceError kind %d limit %q", err, err, tt.wantKind, tt.wantLimit)
			}
			if tt.wantKind == core.RequestDeviceErrorUnsupportedFeature && !rde.MissingFeatures.Contains(missing) {
				t.Fatalf("MissingFeatures %v does not list %v", rde.MissingFeatures, missing)
			}
		})
	}

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		RequiredFeatures: adapter.Features(),
		RequiredLimits:   adapter.Limits(),
	})
	if err != nil {
		t.Fatalf("RequestDevice with the adapter's own features and limits: %v", err)
	}
	device.Release()
}
