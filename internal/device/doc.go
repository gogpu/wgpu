// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package device holds the device domain's implementation (ADR-070 S11).
//
// A device orchestrates every other domain: it creates resources against the
// core state, encodes through the command encoder, submits through the queue,
// and tears down the surfaces configured against it. At S11 the orchestration
// leaves the root package, which keeps the public Device declaration layer
// over it.
//
// Today the package declares seams only. The instance-side view the device
// reads for its teardown scan and its unregister is declared per context in
// seams_native.go and seams_rust.go; the implementation moves in the next
// commit of the step, and the seams are unchanged by that move.
package device
