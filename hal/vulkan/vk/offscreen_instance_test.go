//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import "testing"

func TestOffscreenInstanceWithoutSurfaceExtension(t *testing.T) {
	c, instance := newTestInstance(t)
	newTestDevice(t, c, instance)
}
