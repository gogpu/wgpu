// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestAdapterFeaturesNoTimestampQuery keeps TimestampQuery off until Metal
// query sets exist: CreateQuerySet returns hal.ErrTimestampsNotSupported, so
// offering the feature would let a device enable it and then fail every
// timestamp query set (FEAT-QUERY-METAL-001).
func TestAdapterFeaturesNoTimestampQuery(t *testing.T) {
	if adapterFeatures().Contains(gputypes.FeatureTimestampQuery) {
		t.Error("Metal advertises TimestampQuery but cannot create timestamp query sets")
	}
}
