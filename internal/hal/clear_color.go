// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package hal

import (
	"math"

	"github.com/gogpu/gputypes"
)

// ClearColorInt32 converts a clear color to the four channels of a signed
// integer target, as Rust's `cv.r as i32` does in wgpu-hal: the value is
// truncated toward zero, saturated to the int32 range, and NaN becomes 0.
// A clear of an integer target is the integer, never the float bits.
func ClearColorInt32(c gputypes.Color) [4]int32 {
	return [4]int32{toInt32(c.R), toInt32(c.G), toInt32(c.B), toInt32(c.A)}
}

// ClearColorUint32 converts a clear color to the four channels of an
// unsigned integer target, as Rust's `cv.r as u32` does: truncated toward
// zero, saturated to the uint32 range (negative values become 0), NaN 0.
func ClearColorUint32(c gputypes.Color) [4]uint32 {
	return [4]uint32{toUint32(c.R), toUint32(c.G), toUint32(c.B), toUint32(c.A)}
}

// ClearColorSampleType is the sample type that decides how a color
// attachment of format is cleared: Uint and Sint targets take integer clear
// values, every other format the float ones (Rust wgpu-hal vulkan/conv.rs
// make_vk_clear_color matches on the view format's sample_type). A format
// without a sample type, such as a swapchain's unknown format, clears as
// float.
func ClearColorSampleType(format gputypes.TextureFormat) gputypes.TextureSampleType {
	st, ok := format.SampleType(gputypes.TextureAspectAll, 0)
	if !ok {
		return gputypes.TextureSampleTypeFloat
	}
	return st
}

func toInt32(v float64) int32 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= math.MaxInt32:
		return math.MaxInt32
	case v <= math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}

func toUint32(v float64) uint32 {
	switch {
	case math.IsNaN(v), v <= 0:
		return 0
	case v >= math.MaxUint32:
		return math.MaxUint32
	}
	return uint32(v)
}
