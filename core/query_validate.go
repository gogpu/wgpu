//go:build !(js && wasm)

package core

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ValidateQuerySetDescriptor validates query set creation against enabled features.
// VAL-C24/C25: Timestamp and pipeline statistics queries require feature gates.
func ValidateQuerySetDescriptor(desc *hal.QuerySetDescriptor, features gputypes.Features) error {
	if desc == nil {
		return fmt.Errorf("query set descriptor is nil")
	}
	if desc.Count == 0 {
		return fmt.Errorf("query set count must be greater than 0")
	}

	switch desc.Type {
	case hal.QueryTypeTimestamp:
		return RequireFeature(features, gputypes.FeatureTimestampQuery, "CreateQuerySet")
	case hal.QueryTypeOcclusion:
		return nil
	default:
		// Pipeline statistics and future query types map here when HAL exposes them.
		if desc.Type > hal.QueryTypeTimestamp {
			return RequireFeature(features, gputypes.FeaturePipelineStatisticsQuery, "CreateQuerySet")
		}
		return fmt.Errorf("query set: unknown query type %d", desc.Type)
	}
}

// ValidatePassTimestampWrites validates the timestampWrites of a render or
// compute pass on device, as Rust wgpu-core does
// (command/mod.rs validate_pass_timestamp_writes) after the WebGPU spec's
// GPUComputePassTimestampWrites / GPURenderPassTimestampWrites rules: the
// device has TimestampQuery, the query set is a live timestamp set of the
// same device, each given index is below its count, the two indices differ,
// and at least one is given. pass names the operation in errors.
func ValidatePassTimestampWrites(device *Device, querySet *QuerySet, begin, end *uint32, pass string) error {
	if err := RequireFeature(device.Features, gputypes.FeatureTimestampQuery, pass); err != nil {
		return err
	}
	if querySet == nil || querySet.IsDestroyed() {
		return fmt.Errorf("%s: timestampWrites query set is missing or destroyed", pass)
	}
	if querySet.device != device {
		return fmt.Errorf("%s: timestampWrites query set %q belongs to another device", pass, querySet.label)
	}
	if querySet.queryType != hal.QueryTypeTimestamp {
		return fmt.Errorf("%s: timestampWrites query set %q is not a timestamp query set", pass, querySet.label)
	}
	for _, idx := range [2]*uint32{begin, end} {
		if idx != nil && *idx >= querySet.count {
			return fmt.Errorf("%s: timestampWrites index %d is out of range for query set %q of %d queries",
				pass, *idx, querySet.label, querySet.count)
		}
	}
	if begin != nil && end != nil && *begin == *end {
		return fmt.Errorf("%s: timestampWrites beginning and end of pass write the same index %d", pass, *begin)
	}
	if begin == nil && end == nil {
		return fmt.Errorf("%s: timestampWrites names neither a beginning nor an end of pass index", pass)
	}
	return nil
}
