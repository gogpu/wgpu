// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package device builds a wgpu device and queue around a private HAL backend
// instance for the public wgputest constructors, and builds the detached
// texture handle that carries no backend object.
//
// It is the wgputest tree's one consumer of internal/wgputestbridge: the
// facade cannot import that seam itself (ADR-070 R3), so the device factory
// call and the arguments it takes live here.
package device
