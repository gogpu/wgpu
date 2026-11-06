// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package hal

import (
	"math"
	"testing"

	"github.com/gogpu/gputypes"
)

// The conversions follow Rust's float-to-integer `as`: truncation toward
// zero, saturation at the type's bounds, NaN as 0.
func TestClearColorIntegerConversions(t *testing.T) {
	c := gputypes.Color{R: 5.9, G: -6.9, B: 1e12, A: math.NaN()}
	if got, want := ClearColorInt32(c), [4]int32{5, -6, math.MaxInt32, 0}; got != want {
		t.Errorf("ClearColorInt32 = %v, want %v", got, want)
	}
	if got, want := ClearColorUint32(c), [4]uint32{5, 0, math.MaxUint32, 0}; got != want {
		t.Errorf("ClearColorUint32 = %v, want %v", got, want)
	}
	low := gputypes.Color{R: -1e12, G: math.Inf(-1), B: math.Inf(1), A: 255}
	if got, want := ClearColorInt32(low), [4]int32{math.MinInt32, math.MinInt32, math.MaxInt32, 255}; got != want {
		t.Errorf("ClearColorInt32 = %v, want %v", got, want)
	}
}

func TestClearColorSampleType(t *testing.T) {
	for format, want := range map[gputypes.TextureFormat]gputypes.TextureSampleType{
		gputypes.TextureFormatR8Uint:        gputypes.TextureSampleTypeUint,
		gputypes.TextureFormatRGB10A2Uint:   gputypes.TextureSampleTypeUint,
		gputypes.TextureFormatRGBA32Sint:    gputypes.TextureSampleTypeSint,
		gputypes.TextureFormatRGBA8Unorm:    gputypes.TextureSampleTypeFloat,
		gputypes.TextureFormatR32Float:      gputypes.TextureSampleTypeUnfilterableFloat,
		gputypes.TextureFormatUndefined:     gputypes.TextureSampleTypeFloat,
		gputypes.TextureFormatBGRA8Unorm:    gputypes.TextureSampleTypeFloat,
		gputypes.TextureFormatRGBA16Float:   gputypes.TextureSampleTypeFloat,
		gputypes.TextureFormatRG11B10Ufloat: gputypes.TextureSampleTypeFloat,
	} {
		if got := ClearColorSampleType(format); got != want {
			t.Errorf("ClearColorSampleType(%v) = %v, want %v", format, got, want)
		}
	}
}
