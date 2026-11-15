// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// PollType selects the blocking behavior of Device.Poll.
//
// The public definition stays in the root package: the API golden freezes
// `type PollType uint8` and the Device.Poll signature. internal/core declares
// the seam's own PollType (ADR-070 §15.3, core.DeviceView.Poll), and the
// device adapter converts between the two — the same split A made for
// QueryType and ErrorFilter, since internal/core cannot name a public type.
type PollType uint8

const (
	// PollPoll drains any already-completed submissions and returns
	// immediately without blocking on the GPU. This is the fast path
	// for game loops that call Poll every frame.
	PollPoll PollType = iota
	// PollWait blocks until every submission currently known to the
	// device has completed, then drains. Used by Buffer.Map and by
	// Device.Poll(PollWait) for shutdown drains.
	PollWait
)
