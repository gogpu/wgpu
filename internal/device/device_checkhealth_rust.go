//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// CheckHealth returns nil: the go-webgpu binding exposes no health check.
// The method exists so the public API is identical in every build.
func (d *Device) CheckHealth() error { return nil }
