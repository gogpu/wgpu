// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import "errors"

// ErrReleased is returned when operating on a released resource. It lives in
// the shared leaf because every extraction child returns it (buffer, queue,
// encoder, pipeline, instance and device all report a released handle) and the
// root re-exports the same value, so errors.Is keeps working through the public
// API (ADR-070 §15.1). The message is the public one, unchanged.
//
// The declaration carries no build constraint, unlike the rest of this package's
// error set: the browser build has an internal/core too (bind_groups.go,
// extent.go), and the browser extraction children return this value exactly as
// the native and rust ones do. The root's js/wasm sentinel binds it rather than
// a second value of its own, so one value serves all three contexts and
// errors.Is crosses every boundary in each of them.
var ErrReleased = errors.New("wgpu: resource already released")
