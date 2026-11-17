//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import "github.com/gogpu/wgpu/internal/hal"

// PollType selects the blocking behavior of the device view's Poll.
//
// It lives here, in the seam package, because DeviceView.Poll names it
// (ADR-070 §15.3) and internal/core cannot import an extraction package.
// The inversion plan's older placement (in internal/resource, next to
// map_types.go) predates §15.3 and cannot hold: the seam's home decides
// where its parameter types live. The public root PollType stays a root
// definition with an explicit conversion at the seam, exactly as A treats
// QueryType and ErrorFilter: the golden's public signature is frozen and
// internal/core cannot name a public root type.
type PollType uint8

const (
	// PollPoll drains already-completed submissions and returns without
	// blocking on the GPU.
	PollPoll PollType = iota
	// PollWait blocks until every submission currently known to the device
	// has completed.
	PollWait
)

// DeviceView is the shared device view an extraction child holds when it
// must reach its owning host: the six methods the resource package calls on
// its device (ADR-070 §15.3, sized by the whole resource domain; Poll is a
// buffer need, buffer.go:131,145).
//
// A child's own seam embeds this interface and adds its domain methods, so a
// child seam value is assignable to DeviceView (that assignability is the
// N1 invariant). The root Device satisfies it through a cached adapter until
// S11 moves the device type itself; §15.6's compile check is the call-site
// assignment, never a `var _` assertion outside the child.
type DeviceView interface {
	// Core returns the core device state the child records against.
	Core() *Device
	// IsReleased reports whether the host has been released.
	IsReleased() bool
	// HALDevice returns the HAL device for resource creation.
	HALDevice() hal.Device
	// DestroyQueue returns the deferred-destroy queue, or nil.
	DestroyQueue() *DestroyQueue
	// LastSubmissionIndex returns the latest submission index known to the
	// host, used as the completion gate for a pending map.
	LastSubmissionIndex() uint64
	// Poll drives the host's pending-map triage loop.
	Poll(PollType) bool
}
