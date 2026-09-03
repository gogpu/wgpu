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
