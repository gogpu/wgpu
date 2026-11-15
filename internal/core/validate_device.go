//go:build !(js && wasm)

package core

import (
	"reflect"
	"strings"

	"github.com/gogpu/gputypes"
)

// ValidateDeviceRequest checks the features and limits a device request
// requires against those the adapter exposes, as Rust wgpu-core
// Adapter::validate_device_descriptor (wgpu-core/src/instance.rs) and the
// WebGPU requestDevice steps: a feature the adapter does not expose is a
// RequestDeviceErrorUnsupportedFeature listing every missing feature, and a
// limit better than the adapter's is a RequestDeviceErrorLimitsExceeded
// naming the first such limit. Better is larger for a maximum and smaller
// for an alignment (the Min*Alignment limits).
func ValidateDeviceRequest(features gputypes.Features, limits gputypes.Limits, adapterFeatures gputypes.Features, adapterLimits gputypes.Limits) error {
	if missing := features &^ adapterFeatures; missing != 0 {
		return &RequestDeviceError{Kind: RequestDeviceErrorUnsupportedFeature, MissingFeatures: missing}
	}

	required := reflect.ValueOf(limits)
	allowed := reflect.ValueOf(adapterLimits)
	typ := required.Type()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		req, ok := limitValue(required.Field(i))
		if !ok {
			continue // TestDeviceLimitFieldsComparable keeps every field comparable
		}
		allow, _ := limitValue(allowed.Field(i))
		exceeded := req > allow
		if isAlignmentLimit(name) {
			exceeded = req < allow
		}
		if exceeded {
			return &RequestDeviceError{
				Kind:      RequestDeviceErrorLimitsExceeded,
				Limit:     name,
				Requested: req,
				Allowed:   allow,
			}
		}
	}
	return nil
}

// limitValue returns the value of a gputypes.Limits field.
func limitValue(v reflect.Value) (uint64, bool) {
	switch v.Kind() {
	case reflect.Uint32, reflect.Uint64:
		return v.Uint(), true
	default:
		return 0, false
	}
}

// isAlignmentLimit reports whether the gputypes.Limits field name is an
// alignment, for which a smaller value is better (Rust's with_limits!
// orders min_uniform_buffer_offset_alignment and
// min_storage_buffer_offset_alignment as Ordering::Greater).
func isAlignmentLimit(name string) bool {
	return strings.HasPrefix(name, "Min") && strings.HasSuffix(name, "Alignment")
}
