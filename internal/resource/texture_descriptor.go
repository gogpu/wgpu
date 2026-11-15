// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

import "github.com/gogpu/gputypes"

// TextureDescriptor describes texture creation parameters.
//
// It mirrors the public root TextureDescriptor for the internal consumers
// (ADR-070 §15.1, R9): an extraction package cannot name a public root type,
// so the frontend converts on the way in. Size is gputypes.Extent3D here; the
// public type carries the root Extent3D, which converts member by member. The
// convert_gen projection keeps the two in step.
type TextureDescriptor struct {
	Label         string
	Size          gputypes.Extent3D
	MipLevelCount uint32
	SampleCount   uint32
	Dimension     gputypes.TextureDimension
	Format        gputypes.TextureFormat
	Usage         gputypes.TextureUsage
	ViewFormats   []gputypes.TextureFormat
}
