// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package metal provides a Metal backend for the HAL.
//
// # Status: PLANNED
//
// Required for macOS and iOS platforms.
//
// # References
//
//   - Ebitengine - Uses purego for Metal (excellent pattern!)
//   - metal-rs   - Rust Metal bindings (architecture)
//   - Gio        - Has Metal backend
//
// # Pure Go Approach
//
// Use purego for Objective-C runtime bridge.
// Pattern proven by Ebitengine.
package metal
