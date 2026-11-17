// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package backend opens the private HAL backend instances behind the public
// wgputest constructors.
//
// It is where the wgputest tree imports internal/hal: the public facade keeps
// the exported API and the recording types and reaches a backend through the
// Opener values here, which is the layering rule ADR-070 R3 states (the facade
// imports the root, gputypes and its own internal packages only).
package backend
