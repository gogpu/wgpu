//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// CheckHealth returns nil: the browser exposes no health check. The method
// exists so the public API is identical in every build.
func (d *Device) CheckHealth() error { return nil }
